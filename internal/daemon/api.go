package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/cron"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/orchestrator"
	"github.com/gorilla/websocket"
)

var daemonStartTime = time.Now()

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Permite conexoes de qualquer origem
	},
}

type daemonAPIEventHandler struct {
	workspaceName string
	router        *AgentEventsRouter
	permRespChan  chan permissionResult
	onFinished    func()
	lastStatus    string
	autoApprove   bool
}

func (h *daemonAPIEventHandler) OnStatusChange(status string) {
	h.lastStatus = status
	payload, _ := json.Marshal(map[string]string{
		"type":   "status",
		"status": status,
	})

	isFinished := status == "finished" || status == "idle" || strings.HasPrefix(status, "error:")
	errStr := ""
	if strings.HasPrefix(status, "error:") {
		errStr = strings.TrimPrefix(status, "error: ")
	}

	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: !strings.HasPrefix(status, "error:"),
		Stream:  !isFinished,
		Error:   errStr,
		Data:    payload,
	})

	if isFinished && h.onFinished != nil {
		h.onFinished()
	}
}

func (h *daemonAPIEventHandler) OnMessage(role string, content string) {
	payload, _ := json.Marshal(map[string]string{
		"type":    "message",
		"role":    role,
		"content": content,
	})

	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})
}

func (h *daemonAPIEventHandler) AskPermission(action, target string) (bool, bool) {
	if h.autoApprove {
		log.Printf("[daemonAPIEventHandler] Auto-approving permission check for action: %s - %s", action, target)
		return true, false
	}

	payload, _ := json.Marshal(map[string]string{
		"type":   "ask_permission",
		"action": action,
		"target": target,
	})

	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})

	res := <-h.permRespChan
	return res.approved, res.remember
}

func (h *daemonAPIEventHandler) OnEvent(event loop.AgentEvent) {
	payload, _ := json.Marshal(event)
	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})
}

type terminalListener struct {
	ch   chan []byte
	once sync.Once
}

// TerminalSession holds interactive PTY details
type TerminalSession struct {
	ID        string
	Cmd       *exec.Cmd
	PTY       *os.File
	buffer    []byte
	mu        sync.Mutex
	listeners map[*terminalListener]bool
	closed    bool
}

func NewTerminalSession(id string) (*TerminalSession, error) {
	cmd := exec.Command("bash")
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		cmd.Dir = homeDir
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.Start(cmd)
	if err != nil {
		cmd = exec.Command("sh")
		if homeDir != "" {
			cmd.Dir = homeDir
		}
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		f, err = pty.Start(cmd)
		if err != nil {
			return nil, err
		}
	}

	session := &TerminalSession{
		ID:        id,
		Cmd:       cmd,
		PTY:       f,
		listeners: make(map[*terminalListener]bool),
	}

	go func() {
		defer session.Close()
		buf := make([]byte, 2048)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				session.mu.Lock()
				session.buffer = append(session.buffer, chunk...)
				if len(session.buffer) > 50000 {
					session.buffer = session.buffer[len(session.buffer)-50000:]
				}
				for listener := range session.listeners {
					select {
					case listener.ch <- chunk:
					default:
					}
				}
				session.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}()

	return session, nil
}

func (s *TerminalSession) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("terminal session closed")
	}
	_, err := s.PTY.Write(data)
	return err
}

func (s *TerminalSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.PTY.Close()
	if s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
	}

	s.mu.Lock()
	for listener := range s.listeners {
		listener.once.Do(func() {
			close(listener.ch)
		})
	}
	s.listeners = make(map[*terminalListener]bool)
	s.mu.Unlock()
}

// APIServer expoes uma API HTTP/WebSocket para controle remoto
type APIServer struct {
	manager            *orchestrator.MultiAgentManager
	router             *AgentEventsRouter
	server             *http.Server
	activeHandlers     map[string]*daemonAPIEventHandler
	mu                 sync.Mutex
	quit               chan struct{}
	SessionToken       string
	cronScheduler      *cron.CronScheduler
	terminalSessions   map[string]*TerminalSession
	terminalSessionsMu sync.Mutex
	activeAudioCmd     *exec.Cmd
	activeScreenCmd    *exec.Cmd
	audioMutex         sync.Mutex
	screenMutex        sync.Mutex
}

func NewAPIServer(manager *orchestrator.MultiAgentManager, router *AgentEventsRouter) *APIServer {
	sched := cron.NewCronScheduler()
	sched.Start()
	return &APIServer{
		manager:          manager,
		router:           router,
		activeHandlers:   make(map[string]*daemonAPIEventHandler),
		quit:             make(chan struct{}),
		cronScheduler:    sched,
		terminalSessions: make(map[string]*TerminalSession),
	}
}

func (s *APIServer) Start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/stop", s.handleStop)
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/files", s.handleFiles)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/schedule", s.handleScheduleRoute)
	mux.HandleFunc("/api/schedule/run", s.handleRunSchedule)
	mux.HandleFunc("/api/network", s.handleNetwork)
	mux.HandleFunc("/api/terminal/ws", s.handleTerminalWS)
	mux.HandleFunc("/api/terminal/list", s.handleTerminalList)
	mux.HandleFunc("/api/terminal/close", s.handleTerminalClose)
	mux.HandleFunc("/api/system/info", s.handleSystemInfo)
	mux.HandleFunc("/api/transcribe", s.handleTranscribe)
	mux.HandleFunc("/api/record/start", s.handleRecordStart)
	mux.HandleFunc("/api/record/stop", s.handleRecordStop)
	mux.HandleFunc("/api/devices/audio", s.handleDevicesAudio)
	mux.HandleFunc("/api/devices/screens", s.handleDevicesScreens)

	// Load and register existing jobs on start
	tasks, err := s.loadTasks()
	if err == nil {
		for _, t := range tasks {
			taskCopy := t // copy for closure
			_ = s.cronScheduler.AddJob(t.ID, t.Cron, func() {
				s.triggerScheduledTask(taskCopy)
			})
		}
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
	s.server = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("falha ao ligar API Server na porta %d: %w", port, err)
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[APIServer] Erro no servidor HTTP: %v", err)
		}
	}()

	log.Printf("[APIServer] Servidor HTTP/WS iniciado em http://127.0.0.1:%d (escutando em todas as interfaces)", port)
	return nil
}

func (s *APIServer) Stop() {
	close(s.quit)
	if s.cronScheduler != nil {
		s.cronScheduler.Stop()
	}
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	}

	s.terminalSessionsMu.Lock()
	for _, sess := range s.terminalSessions {
		sess.Close()
	}
	s.terminalSessions = make(map[string]*TerminalSession)
	s.terminalSessionsMu.Unlock()
}

func (s *APIServer) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.SessionToken == "" {
		return true
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = authHeader[7:]
			} else {
				token = authHeader
			}
		}
	}
	if token == "" {
		token = r.Header.Get("X-Session-Token")
	}
	if token != s.SessionToken {
		http.Error(w, "Nao autorizado: token de sessao invalido", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *APIServer) handleNetwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	ip := "127.0.0.1"
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			ip = localAddr.IP.String()
		}
	} else {
		addrs, err := net.InterfaceAddrs()
		if err == nil {
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						ip = ipnet.IP.String()
						break
					}
				}
			}
		}
	}

	response := map[string]interface{}{
		"ip": ip,
	}
	data, _ := json.Marshal(response)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	agents := s.manager.ListRunningAgents()
	wsList, _ := orchestrator.LoadWorkspaces()
	type WsStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Task   string `json:"task"`
	}
	list := []WsStatus{}
	for _, ws := range wsList {
		status := "idle"
		task := ""
		for _, a := range agents {
			if a.WorkspaceName == ws.Name {
				status = "running"
				task = a.Task
			}
		}
		list = append(list, WsStatus{Name: ws.Name, Status: status, Task: task})
	}
	data, _ := json.Marshal(list)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *APIServer) handleRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo nao permitido", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Workspace string `json:"workspace"`
		Task      string `json:"task"`
		Session   string `json:"session,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	handler := &daemonAPIEventHandler{
		workspaceName: req.Workspace,
		router:         s.router,
		permRespChan:  make(chan permissionResult, 1),
	}
	handler.onFinished = func() {
		s.mu.Lock()
		delete(s.activeHandlers, req.Workspace)
		s.mu.Unlock()
	}
	s.activeHandlers[req.Workspace] = handler
	s.mu.Unlock()

	err := s.manager.StartAgent(context.Background(), req.Workspace, req.Session, req.Task, handler)
	if err != nil {
		s.mu.Lock()
		delete(s.activeHandlers, req.Workspace)
		s.mu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (s *APIServer) handleStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo nao permitido", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.manager.StopAgent(req.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (s *APIServer) handleWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	closeChan := make(chan struct{})
	eventCh := make(chan IPCResponse, 100)
	var currentWorkspace string

	defer func() {
		if currentWorkspace != "" {
			s.router.Unregister(currentWorkspace, eventCh)
		}
	}()

	// Read loop do WebSocket
	go func() {
		defer close(closeChan)
		for {
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var msg IPCMessage
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "subscribe":
				if currentWorkspace != "" {
					s.router.Unregister(currentWorkspace, eventCh)
				}
				currentWorkspace = msg.Workspace
				s.router.Register(currentWorkspace, eventCh)

				s.mu.Lock()
				h, exists := s.activeHandlers[msg.Workspace]
				s.mu.Unlock()
				if exists && h.lastStatus != "" {
					statusPayload, _ := json.Marshal(map[string]string{
						"type":   "status",
						"status": h.lastStatus,
					})
					_ = conn.WriteJSON(IPCResponse{
						Success: !strings.HasPrefix(h.lastStatus, "error:"),
						Stream:  h.lastStatus != "finished" && h.lastStatus != "idle" && !strings.HasPrefix(h.lastStatus, "error:"),
						Data:    statusPayload,
					})
				} else {
					// Se não houver agente rodando ativo, sincroniza o cliente para 'idle'
					statusPayload, _ := json.Marshal(map[string]string{
						"type":   "status",
						"status": "idle",
					})
					_ = conn.WriteJSON(IPCResponse{
						Success: true,
						Stream:  false,
						Data:    statusPayload,
					})
				}

			case "permission_response":
				var permPayload struct {
					Approved bool `json:"approved"`
					Remember bool `json:"remember"`
				}
				_ = json.Unmarshal(msg.Payload, &permPayload)

				s.mu.Lock()
				h, exists := s.activeHandlers[msg.Workspace]
				s.mu.Unlock()

				if exists {
					select {
					case h.permRespChan <- permissionResult{approved: permPayload.Approved, remember: permPayload.Remember}:
					default:
					}
				}

			case "run":
				log.Printf("[daemon WS run] msg Workspace: %s, Task: %s, Provider: %s, Model: %s\n", msg.Workspace, msg.Task, msg.Provider, msg.Model)
				if currentWorkspace != "" {
					s.router.Unregister(currentWorkspace, eventCh)
				}
				currentWorkspace = msg.Workspace
				s.router.Register(currentWorkspace, eventCh)

				s.mu.Lock()
				handler := &daemonAPIEventHandler{
					workspaceName: msg.Workspace,
					router:         s.router,
					permRespChan:  make(chan permissionResult, 1),
				}
				handler.onFinished = func() {
					s.mu.Lock()
					delete(s.activeHandlers, msg.Workspace)
					s.mu.Unlock()
				}
				s.activeHandlers[msg.Workspace] = handler
				s.mu.Unlock()

				ctx := context.Background()
				if msg.Provider != "" {
					ctx = context.WithValue(ctx, "provider_override", msg.Provider)
				}
				if msg.Model != "" {
					ctx = context.WithValue(ctx, "model_override", msg.Model)
				}

				// Notifica inicio
				startedPayload, _ := json.Marshal(map[string]string{"type": "started"})
				eventCh <- IPCResponse{Success: true, Stream: true, Data: startedPayload}

				go func() {
					err := s.manager.StartAgent(ctx, msg.Workspace, msg.Session, msg.Task, handler)
					if err != nil {
						s.mu.Lock()
						delete(s.activeHandlers, msg.Workspace)
						s.mu.Unlock()
						eventCh <- IPCResponse{Success: false, Error: err.Error()}
						return
					}
				}()
			}
		}
	}()

	// Write loop do WebSocket
	for {
		select {
		case resp, ok := <-eventCh:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(resp); err != nil {
				return
			}
		case <-closeChan:
			return
		}
	}
}

func (s *APIServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if !s.authorize(w, r) {
		return
	}

	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		http.Error(w, "caminho obrigatorio", http.StatusBadRequest)
		return
	}

	// Garante que o diretorio exista para evitar 500 no fallback do browser
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type FileItem struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	result := []FileItem{}
	for _, entry := range entries {
		name := entry.Name()
		if name == "node_modules" || name == ".git" || name == "target" || name == "dist" || name == ".results" || name == ".home" || name == ".cargo" {
			continue
		}
		fullPath := filepath.Join(dirPath, name)
		result = append(result, FileItem{
			Name:  name,
			Path:  fullPath,
			IsDir: entry.IsDir(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *APIServer) handleFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if !s.authorize(w, r) {
		return
	}

	if r.Method == http.MethodGet {
		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "caminho obrigatorio", http.StatusBadRequest)
			return
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(content)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Path == "" {
			http.Error(w, "caminho obrigatorio", http.StatusBadRequest)
			return
		}

		parentDir := filepath.Dir(req.Path)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(req.Path, []byte(req.Content), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
		return
	}

	http.Error(w, "Metodo nao permitido", http.StatusMethodNotAllowed)
}

type ScheduledTask struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cron      string `json:"cron"`
	Workspace string `json:"workspace"`
	Task      string `json:"task"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

func (s *APIServer) tasksPath() (string, error) {
	gDir, err := config.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gDir, "scheduled_tasks.json"), nil
}

func (s *APIServer) loadTasks() ([]ScheduledTask, error) {
	path, err := s.tasksPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ScheduledTask{}, nil
		}
		return nil, err
	}
	var list []ScheduledTask
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *APIServer) saveTasks(list []ScheduledTask) error {
	path, err := s.tasksPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *APIServer) triggerScheduledTask(t ScheduledTask) {
	log.Printf("[CronScheduler] Triggered task: %s in workspace: %s", t.Name, t.Workspace)

	s.mu.Lock()
	handler := &daemonAPIEventHandler{
		workspaceName: t.Workspace,
		router:         s.router,
		permRespChan:  make(chan permissionResult, 1),
		autoApprove:   true,
	}
	handler.onFinished = func() {
		s.mu.Lock()
		delete(s.activeHandlers, t.Workspace)
		s.mu.Unlock()
	}
	s.activeHandlers[t.Workspace] = handler
	s.mu.Unlock()

	err := s.manager.StartAgent(context.Background(), t.Workspace, "", t.Task, handler)
	if err != nil {
		log.Printf("[CronScheduler] Error starting agent for task %s: %v", t.Name, err)
	}
}

func (s *APIServer) handleScheduleRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSchedule(w, r)
	case http.MethodPost:
		s.handlePostSchedule(w, r)
	case http.MethodDelete:
		s.handleDeleteSchedule(w, r)
	default:
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Metodo nao permitido", http.StatusMethodNotAllowed)
	}
}

func (s *APIServer) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

func (s *APIServer) handlePostSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	var req struct {
		Name      string `json:"name"`
		Cron      string `json:"cron"`
		Workspace string `json:"workspace"`
		Task      string `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Cron == "" || req.Workspace == "" || req.Task == "" {
		http.Error(w, "todos os campos sao obrigatorios", http.StatusBadRequest)
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newTask := ScheduledTask{
		ID:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		Name:      req.Name,
		Cron:      req.Cron,
		Workspace: req.Workspace,
		Task:      req.Task,
		Status:    "Active",
		CreatedAt: time.Now().Format("2006-01-02"),
	}

	err = s.cronScheduler.AddJob(newTask.ID, newTask.Cron, func() {
		s.triggerScheduledTask(newTask)
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("cron invalido: %v", err), http.StatusBadRequest)
		return
	}

	tasks = append(tasks, newTask)
	if err := s.saveTasks(tasks); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(newTask)
}

func (s *APIServer) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id obrigatorio", http.StatusBadRequest)
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	var updated []ScheduledTask
	for _, t := range tasks {
		if t.ID == id {
			found = true
			s.cronScheduler.RemoveJob(id)
			continue
		}
		updated = append(updated, t)
	}

	if !found {
		http.Error(w, "tarefa nao encontrada", http.StatusNotFound)
		return
	}

	if err := s.saveTasks(updated); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (s *APIServer) handleRunSchedule(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id obrigatorio", http.StatusBadRequest)
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var target *ScheduledTask
	for _, t := range tasks {
		if t.ID == id {
			target = &t
			break
		}
	}

	if target == nil {
		http.Error(w, "tarefa nao encontrada", http.StatusNotFound)
		return
	}

	go s.triggerScheduledTask(*target)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (s *APIServer) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[APIServer] websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	termID := r.URL.Query().Get("id")
	if termID == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\nErro: Terminal ID obrigatorio\r\n"))
		return
	}

	s.terminalSessionsMu.Lock()
	session, exists := s.terminalSessions[termID]
	if !exists || session.closed {
		var err error
		session, err = NewTerminalSession(termID)
		if err != nil {
			s.terminalSessionsMu.Unlock()
			_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\nErro ao criar sessao de terminal: %v\r\n", err)))
			return
		}
		s.terminalSessions[termID] = session
	}
	s.terminalSessionsMu.Unlock()

	outChan := make(chan []byte, 100)
	listener := &terminalListener{ch: outChan}
	session.mu.Lock()
	session.listeners[listener] = true
	history := make([]byte, len(session.buffer))
	copy(history, session.buffer)
	session.mu.Unlock()

	if len(history) > 0 {
		_ = conn.WriteMessage(websocket.BinaryMessage, history)
	} else {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n--- Terminal Crom Agent Iniciado ---\r\n"))
	}

	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		for data := range outChan {
			err := conn.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				return
			}
		}
	}()

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if mt == websocket.TextMessage || mt == websocket.BinaryMessage {
			_ = session.Write(msg)
		}
	}

	session.mu.Lock()
	delete(session.listeners, listener)
	session.mu.Unlock()
	
	listener.once.Do(func() {
		close(outChan)
	})

	<-doneChan
}

func (s *APIServer) handleTerminalList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	s.terminalSessionsMu.Lock()
	ids := []string{}
	for id, sess := range s.terminalSessions {
		if !sess.closed {
			ids = append(ids, id)
		}
	}
	s.terminalSessionsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ids)
}

func (s *APIServer) handleTerminalClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	termID := r.URL.Query().Get("id")
	if termID == "" {
		http.Error(w, "id obrigatorio", http.StatusBadRequest)
		return
	}

	s.terminalSessionsMu.Lock()
	session, exists := s.terminalSessions[termID]
	if exists {
		session.Close()
		delete(s.terminalSessions, termID)
	}
	s.terminalSessionsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

func (s *APIServer) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	agents := s.manager.ListRunningAgents()
	type AgentInfo struct {
		Workspace string `json:"workspace"`
		Task      string `json:"task"`
		Session   string `json:"session"`
	}
	activeAgents := []AgentInfo{}
	for _, a := range agents {
		activeAgents = append(activeAgents, AgentInfo{
			Workspace: a.WorkspaceName,
			Task:      a.Task,
			Session:   "",
		})
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptimeSeconds := int(time.Since(daemonStartTime).Seconds())

	response := map[string]interface{}{
		"uptime":       uptimeSeconds,
		"go_version":   runtime.Version(),
		"num_goroutine": runtime.NumGoroutine(),
		"alloc_mb":     float64(m.Alloc) / 1024 / 1024,
		"sys_mb":       float64(m.Sys) / 1024 / 1024,
		"active_agents": activeAgents,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *APIServer) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Load keys from environment config
	gDir, err := config.GlobalDir()
	if err != nil {
		http.Error(w, "Global dir error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	env, err := config.LoadEnvVars(gDir)
	if err != nil {
		http.Error(w, "Load env error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	openaiKey := env.Get("OPENAI_API_KEY")
	geminiKey := env.Get("GEMINI_API_KEY")

	// Read body (audio data)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read body error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(bodyBytes) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	// Call OpenAI Whisper if key available
	if openaiKey != "" {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		part, err := writer.CreateFormFile("file", "audio.wav")
		if err != nil {
			http.Error(w, "Create form file error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := part.Write(bodyBytes); err != nil {
			http.Error(w, "Write multipart file error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := writer.WriteField("model", "whisper-1"); err != nil {
			http.Error(w, "Write field error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := writer.Close(); err != nil {
			http.Error(w, "Close writer error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &b)
		if err != nil {
			http.Error(w, "New request error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+openaiKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "OpenAI API call error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Read response error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("OpenAI API error status %d: %s", resp.StatusCode, string(respBytes)), resp.StatusCode)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(respBytes)
		return
	}

	// Call Gemini generateContent if key available
	if geminiKey != "" {
		audioB64 := base64.StdEncoding.EncodeToString(bodyBytes)
		payload := map[string]interface{}{
			"contents": []interface{}{
				map[string]interface{}{
					"parts": []interface{}{
						map[string]interface{}{
							"text": "Transcreva o áudio em português de forma direta e limpa, sem comentários adicionais.",
						},
						map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": "audio/wav",
								"data":     audioB64,
							},
						},
					},
				},
			},
		}

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			http.Error(w, "Marshal json error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", geminiKey)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			http.Error(w, "New request error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Gemini API call error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Read response error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("Gemini API error status %d: %s", resp.StatusCode, string(respBytes)), resp.StatusCode)
			return
		}

		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}

		if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
			http.Error(w, "Unmarshal response error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var text string
		if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
			text = geminiResp.Candidates[0].Content.Parts[0].Text
		}
		text = strings.TrimSpace(text)

		result := map[string]string{"text": text}
		outBytes, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(outBytes)
		return
	}

	// Fallback to local offline Vosk transcription python script
	log.Println("[APIServer] Nenhuma chave de API de terceiros configurada para transcrição. Tentando transcrição offline com Vosk...")

	tempFile := "/tmp/crom_transcribe_temp.wav"
	_ = os.Remove(tempFile)
	if err := os.WriteFile(tempFile, bodyBytes, 0644); err != nil {
		http.Error(w, "Falha ao gravar arquivo temporário para Vosk: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile)

	pythonScript := "/home/j/Área de trabalho/GitHub/crom-agente5/crom-agente/scripts/transcribe.py"
	cmd := exec.Command("python3", pythonScript, tempFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := strings.TrimSpace(stderr.String())
		if errStr == "" {
			errStr = err.Error()
		}
		log.Printf("[APIServer] Vosk transcription failed: %s", errStr)
		http.Error(w, errStr, http.StatusBadRequest)
		return
	}

	transcribedText := strings.TrimSpace(stdout.String())
	log.Printf("[APIServer] Vosk transcription success: %s", transcribedText)

	result := map[string]string{"text": transcribedText}
	outBytes, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(outBytes)
}

func (s *APIServer) handleRecordStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	recType := r.URL.Query().Get("type")
	if recType != "audio" && recType != "screen" {
		http.Error(w, "Invalid recording type. Must be 'audio' or 'screen'", http.StatusBadRequest)
		return
	}

	if recType == "audio" {
		s.audioMutex.Lock()
		defer s.audioMutex.Unlock()

		if s.activeAudioCmd != nil && s.activeAudioCmd.Process != nil {
			_ = s.activeAudioCmd.Process.Signal(os.Interrupt)
			_ = s.activeAudioCmd.Wait()
			s.activeAudioCmd = nil
		}

		_ = os.Remove("/tmp/crom_audio_recording.wav")

		// Build arecord args with optional device and sample rate
		arecordArgs := []string{"-f", "S16_LE", "-c", "1", "-t", "wav"}

		sampleRate := r.URL.Query().Get("sampleRate")
		if sampleRate == "44100" {
			arecordArgs = append(arecordArgs, "-r", "44100")
		} else {
			arecordArgs = append(arecordArgs, "-r", "16000")
		}

		device := r.URL.Query().Get("device")
		if device != "" && device != "default" {
			arecordArgs = append(arecordArgs, "-D", device)
		}

		arecordArgs = append(arecordArgs, "/tmp/crom_audio_recording.wav")

		cmd := exec.Command("arecord", arecordArgs...)
		if err := cmd.Start(); err != nil {
			log.Printf("[APIServer] Error starting arecord: %v", err)
			http.Error(w, "Failed to start audio recording: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.activeAudioCmd = cmd
		log.Printf("[APIServer] Native audio recording started via arecord (device=%s) to /tmp/crom_audio_recording.wav", device)

	} else {
		s.screenMutex.Lock()
		defer s.screenMutex.Unlock()

		if s.activeScreenCmd != nil && s.activeScreenCmd.Process != nil {
			_ = s.activeScreenCmd.Process.Signal(os.Interrupt)
			_ = s.activeScreenCmd.Wait()
			s.activeScreenCmd = nil
		}

		_ = os.Remove("/tmp/crom_screen_recording.webm")

		// Build gst-launch command with optional source targeting
		source := r.URL.Query().Get("source")     // "all", "monitor", "window"
		windowId := r.URL.Query().Get("windowId") // X11 window id hex
		monitorGeom := r.URL.Query().Get("geometry") // e.g. "1366x768+1360+0"

		var gstArgs []string

		if source == "window" && windowId != "" {
			// Record a specific window by XID
			gstArgs = []string{"ximagesrc", "xid=" + windowId, "use-damage=0",
				"!", "video/x-raw,framerate=15/1",
				"!", "videoconvert", "!", "vp8enc", "!", "webmmux",
				"!", "filesink", "location=/tmp/crom_screen_recording.webm"}
		} else if source == "monitor" && monitorGeom != "" {
			// Parse geometry "WxH+X+Y" to get startx, starty, endx, endy
			var gw, gh, gx, gy int
			_, err := fmt.Sscanf(monitorGeom, "%dx%d+%d+%d", &gw, &gh, &gx, &gy)
			if err != nil {
				log.Printf("[APIServer] Invalid monitor geometry %q: %v", monitorGeom, err)
				http.Error(w, "Invalid monitor geometry: "+monitorGeom, http.StatusBadRequest)
				return
			}
			gstArgs = []string{"ximagesrc",
				fmt.Sprintf("startx=%d", gx), fmt.Sprintf("starty=%d", gy),
				fmt.Sprintf("endx=%d", gx+gw-1), fmt.Sprintf("endy=%d", gy+gh-1),
				"use-damage=0",
				"!", "video/x-raw,framerate=15/1",
				"!", "videoconvert", "!", "vp8enc", "!", "webmmux",
				"!", "filesink", "location=/tmp/crom_screen_recording.webm"}
		} else {
			// Default: record all screens
			gstArgs = []string{"ximagesrc", "use-damage=0",
				"!", "video/x-raw,framerate=15/1",
				"!", "videoconvert", "!", "vp8enc", "!", "webmmux",
				"!", "filesink", "location=/tmp/crom_screen_recording.webm"}
		}

		cmd := exec.Command("gst-launch-1.0", gstArgs...)
		if err := cmd.Start(); err != nil {
			log.Printf("[APIServer] Error starting gst-launch-1.0: %v", err)
			http.Error(w, "Failed to start screen recording: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.activeScreenCmd = cmd
		log.Printf("[APIServer] Native screen recording started (source=%s) to /tmp/crom_screen_recording.webm", source)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"recording"}`))
}

func (s *APIServer) handleRecordStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	recType := r.URL.Query().Get("type")
	if recType != "audio" && recType != "screen" {
		http.Error(w, "Invalid recording type. Must be 'audio' or 'screen'", http.StatusBadRequest)
		return
	}

	var filePath string

	if recType == "audio" {
		s.audioMutex.Lock()
		defer s.audioMutex.Unlock()

		if s.activeAudioCmd == nil {
			http.Error(w, "No active audio recording", http.StatusNotFound)
			return
		}

		if s.activeAudioCmd.Process != nil {
			_ = s.activeAudioCmd.Process.Signal(os.Interrupt)
			_ = s.activeAudioCmd.Wait()
		}
		s.activeAudioCmd = nil
		filePath = "/tmp/crom_audio_recording.wav"
		log.Println("[APIServer] Native audio recording stopped")

	} else {
		s.screenMutex.Lock()
		defer s.screenMutex.Unlock()

		if s.activeScreenCmd == nil {
			http.Error(w, "No active screen recording", http.StatusNotFound)
			return
		}

		if s.activeScreenCmd.Process != nil {
			_ = s.activeScreenCmd.Process.Signal(os.Interrupt)
			_ = s.activeScreenCmd.Wait()
		}
		s.activeScreenCmd = nil
		filePath = "/tmp/crom_screen_recording.webm"
		log.Println("[APIServer] Native screen recording stopped")
	}

	time.Sleep(100 * time.Millisecond)

	bytes, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("[APIServer] Error reading recorded file %s: %v", filePath, err)
		http.Error(w, "Failed to read recorded file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = os.Remove(filePath)

	if recType == "audio" {
		w.Header().Set("Content-Type", "audio/wav")
	} else {
		w.Header().Set("Content-Type", "video/webm")
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bytes)
}

func (s *APIServer) handleDevicesAudio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	type AudioDevice struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	devices := []AudioDevice{{ID: "default", Name: "Padrão do sistema"}}

	// Parse arecord -l output
	cmd := exec.Command("arecord", "-l")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "card ") {
				// Example: "card 0: PCH [HDA Intel PCH], device 0: ALC269 Analog [ALC269 Analog]"
				var cardNum, devNum int
				rest := line[5:] // after "card "
				parts := strings.SplitN(rest, ":", 2)
				if len(parts) < 2 {
					continue
				}
				_, scanErr := fmt.Sscanf(parts[0], "%d", &cardNum)
				if scanErr != nil {
					continue
				}

				devPart := parts[1]
				devIdx := strings.Index(devPart, "device ")
				if devIdx < 0 {
					continue
				}
				devStr := devPart[devIdx+7:]
				_, scanErr = fmt.Sscanf(devStr, "%d", &devNum)
				if scanErr != nil {
					continue
				}

				// Extract name from brackets
				nameStart := strings.Index(line, "[")
				nameEnd := strings.Index(line, "]")
				devName := fmt.Sprintf("hw:%d,%d", cardNum, devNum)
				if nameStart >= 0 && nameEnd > nameStart {
					devName = line[nameStart+1 : nameEnd]
				}

				devices = append(devices, AudioDevice{
					ID:   fmt.Sprintf("hw:%d,%d", cardNum, devNum),
					Name: devName,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (s *APIServer) handleDevicesScreens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	type ScreenInfo struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Geometry string `json:"geometry"`
	}
	type WindowInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type ScreensResponse struct {
		Screens []ScreenInfo `json:"screens"`
		Windows []WindowInfo `json:"windows"`
	}

	resp := ScreensResponse{
		Screens: []ScreenInfo{{ID: "all", Name: "Todas as telas", Geometry: ""}},
		Windows: []WindowInfo{},
	}

	// Parse xrandr for monitors
	cmd := exec.Command("xrandr", "--query")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if !strings.Contains(line, " connected ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			outputName := fields[0]
			// Find geometry: WxH+X+Y
			var geometry string
			for _, f := range fields[2:] {
				if strings.Contains(f, "+") && strings.Contains(f, "x") {
					geometry = f
					break
				}
			}
			if geometry == "" {
				continue
			}
			resp.Screens = append(resp.Screens, ScreenInfo{
				ID:       outputName,
				Name:     fmt.Sprintf("%s (%s)", outputName, strings.Split(geometry, "+")[0]),
				Geometry: geometry,
			})
		}
	}

	// Parse wmctrl -l for windows
	cmd = exec.Command("wmctrl", "-l")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			winId := fields[0]
			winName := strings.Join(fields[3:], " ")
			// Skip desktop entries
			if winName == "Área de trabalho" || strings.HasPrefix(winName, "nemo-desktop") {
				continue
			}
			resp.Windows = append(resp.Windows, WindowInfo{
				ID:   winId,
				Name: winName,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
