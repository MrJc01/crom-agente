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
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/orchestrator"
)

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

func (s *APIServer) handleOllamaTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(ollamaHost + "/api/tags")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *APIServer) handleGetToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Referer()
	}

	isAllowed := false
	if strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "tauri://") ||
		strings.HasPrefix(origin, "http://tauri.localhost") ||
		origin == "" {
		isAllowed = true
	}

	if !isAllowed {
		log.Printf("[APIServer handleGetToken] Rejeitado: Origem nao autorizada %q", origin)
		http.Error(w, "Proibido: Origem nao autorizada", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token": s.SessionToken,
	})
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

		ext := strings.ToLower(filepath.Ext(filePath))
		contentType := "text/plain; charset=utf-8"
		switch ext {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		case ".ico":
			contentType = "image/x-icon"
		case ".svg":
			contentType = "image/svg+xml"
		case ".pdf":
			contentType = "application/pdf"
		case ".mp3":
			contentType = "audio/mpeg"
		case ".wav":
			contentType = "audio/wav"
		case ".ogg":
			contentType = "audio/ogg"
		case ".mp4":
			contentType = "video/mp4"
		case ".webm":
			contentType = "video/webm"
		}

		w.Header().Set("Content-Type", contentType)
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

func (s *APIServer) handleReveal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if !s.authorize(w, r) {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "caminho obrigatorio", http.StatusBadRequest)
		return
	}

	err := openPath(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
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

	possiblePaths := []string{
		"/home/j/Documentos/GitHub/crom-agente/scripts/transcribe.py",
		"/home/j/Área de trabalho/GitHub/crom-agente5/crom-agente/scripts/transcribe.py",
		"./scripts/transcribe.py",
	}
	var pythonScript string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			pythonScript = p
			break
		}
	}
	if pythonScript == "" {
		pythonScript = "/home/j/Documentos/GitHub/crom-agente/scripts/transcribe.py"
	}
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

func (s *APIServer) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
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
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if s.manager.MCPManager == nil {
		w.Write([]byte("[]"))
		return
	}

	data, err := s.manager.MCPManager.MCPStatusJSON()
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao gerar JSON de status MCP: %v", err), http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

// handleBrowserProxy atua como um proxy HTTP para carregar páginas web dentro de um iframe,
// limpando os cabeçalhos de segurança (X-Frame-Options, Content-Security-Policy) e injetando
// a tag <base href="..."> para que todas as requisições relativas da página apontem de volta
// ao domínio original.
func (s *APIServer) handleBrowserProxy(w http.ResponseWriter, r *http.Request) {
	// Permitir CORS
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

	mode := r.URL.Query().Get("mode")
	workspace := r.URL.Query().Get("workspace")

	if mode == "agent" && workspace != "" {
		html, url, err := s.manager.GetBrowserPageContent(workspace)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
body {
  background-color: #0b0b0d;
  color: #a1a1aa;
  font-family: sans-serif;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 90vh;
  margin: 0;
  overflow: hidden;
}
.spinner {
  border: 3px solid rgba(255,255,255,0.05);
  border-radius: 50%;
  border-top: 3px solid #10b981;
  width: 28px;
  height: 28px;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}
@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
.msg {
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.025em;
}
</style>
</head>
<body>
<div class="spinner"></div>
<div class="msg">Aguardando o agente iniciar o navegador ou abrir uma página...</div>
</body>
</html>`))
			return
		}

		baseTag := fmt.Sprintf("<base href=\"%s\">", url)
		var newHTML string
		htmlStr := html
		headIdx := strings.Index(strings.ToLower(htmlStr), "<head>")
		if headIdx != -1 {
			insertPos := headIdx + len("<head>")
			newHTML = htmlStr[:insertPos] + "\n" + baseTag + htmlStr[insertPos:]
		} else {
			htmlIdx := strings.Index(strings.ToLower(htmlStr), "<html>")
			if htmlIdx != -1 {
				insertPos := htmlIdx + len("<html>")
				newHTML = htmlStr[:insertPos] + "\n" + baseTag + htmlStr[insertPos:]
			} else {
				newHTML = baseTag + "\n" + htmlStr
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cleanHTMLIntegrity(newHTML)))
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Parâmetro 'url' ou 'workspace' ausente", http.StatusBadRequest)
		return
	}

	// Tratar requisições sem protocolo (ex: www.google.com)
	if !strings.HasPrefix(strings.ToLower(targetURL), "http://") && !strings.HasPrefix(strings.ToLower(targetURL), "https://") {
		targetURL = "http://" + targetURL
	}

	// Criar a requisição de proxy
	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Falha ao criar request: %v", err), http.StatusBadRequest)
		return
	}

	// Definir User-Agent padrão para evitar bloqueio por bot-detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao buscar a URL via proxy: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copiar os cabeçalhos de resposta, EXCLUINDO os cabeçalhos de segurança que impedem iframes
	for key, values := range resp.Header {
		lowerKey := strings.ToLower(key)
		if lowerKey == "x-frame-options" || lowerKey == "content-security-policy" || lowerKey == "content-security-policy-report-only" {
			continue // Ignora cabeçalhos que bloqueiam iframe
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		// Ler corpo HTML
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}
		htmlStr := string(bodyBytes)

		// Injetar tag <base href="..."> para que recursos e rotas relativas funcionem no domínio correto
		baseTag := fmt.Sprintf("<base href=\"%s\">", targetURL)

		var newHTML string
		headIdx := strings.Index(strings.ToLower(htmlStr), "<head>")
		if headIdx != -1 {
			insertPos := headIdx + len("<head>")
			newHTML = htmlStr[:insertPos] + "\n" + baseTag + htmlStr[insertPos:]
		} else {
			htmlIdx := strings.Index(strings.ToLower(htmlStr), "<html>")
			if htmlIdx != -1 {
				insertPos := htmlIdx + len("<html>")
				newHTML = htmlStr[:insertPos] + "\n" + baseTag + htmlStr[insertPos:]
			} else {
				newHTML = baseTag + "\n" + htmlStr
			}
		}
		w.Write([]byte(cleanHTMLIntegrity(newHTML)))
	} else {
		// Copiar dados binários ou outro tipo de resposta diretamente
		_, _ = io.Copy(w, resp.Body)
	}
}

var (
	integrityRegex   = regexp.MustCompile(`(?i)\s+integrity\s*=\s*["'][^"']*["']`)
	crossoriginRegex = regexp.MustCompile(`(?i)\s+crossorigin\s*=\s*["'][^"']*["']`)
)

func cleanHTMLIntegrity(html string) string {
	html = integrityRegex.ReplaceAllString(html, "")
	html = crossoriginRegex.ReplaceAllString(html, "")
	return html
}


