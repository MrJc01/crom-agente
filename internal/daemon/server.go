package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

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
	pendingAction string
	pendingTarget string
	mu            sync.Mutex
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

func (h *daemonAPIEventHandler) AskPermission(ctx context.Context, action, target string) (bool, bool) {
	if h.autoApprove {
		log.Printf("[daemonAPIEventHandler] Auto-approving permission check for action: %s - %s", action, target)
		return true, false
	}

	h.mu.Lock()
	h.pendingAction = action
	h.pendingTarget = target
	h.mu.Unlock()

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

	select {
	case <-ctx.Done():
		h.mu.Lock()
		h.pendingAction = ""
		h.pendingTarget = ""
		h.mu.Unlock()
		return false, false
	case res := <-h.permRespChan:
		h.mu.Lock()
		h.pendingAction = ""
		h.pendingTarget = ""
		h.mu.Unlock()
		return res.approved, res.remember
	}
}

func (h *daemonAPIEventHandler) OnEvent(event loop.AgentEvent) {
	payload, _ := json.Marshal(event)
	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})
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
	mux.HandleFunc("/api/tags", s.handleOllamaTags)
	mux.HandleFunc("/api/token", s.handleGetToken)
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
	mux.HandleFunc("/api/mcp/status", s.handleMCPStatus)
	mux.HandleFunc("/api/browser/proxy", s.handleBrowserProxy)

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
		log.Printf("[APIServer authorize] Autorizacao FALHOU para %s %s. Token recebido: %q, Token esperado: %q", r.Method, r.URL.Path, token, s.SessionToken)
		http.Error(w, "Nao autorizado: token de sessao invalido", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *APIServer) ScheduleTimerTask(workspaceName, sessionName, task string, delaySecs int, provider, model string) {
	log.Printf("[Timer] Agendando tarefa %q em %d segundos para workspace: %s (provedor: %s, modelo: %s)", task, delaySecs, workspaceName, provider, model)
	time.AfterFunc(time.Duration(delaySecs)*time.Second, func() {
		log.Printf("[Timer] Timer expirou. Executando tarefa %q no workspace: %s", task, workspaceName)

		ctx := context.Background()
		if provider != "" {
			ctx = context.WithValue(ctx, "provider_override", provider)
		}
		if model != "" {
			ctx = context.WithValue(ctx, "model_override", model)
		}

		var err error
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(30 * time.Second)

		for {
			s.mu.Lock()
			// Cria um handler do daemon WebSocket para transmitir eventos
			handler := &daemonAPIEventHandler{
				workspaceName: workspaceName,
				router:        s.router,
				permRespChan:  make(chan permissionResult, 1),
				autoApprove:   true, // auto-aprova ações executadas pelo despertador automático do agente
			}
			handler.onFinished = func() {
				s.mu.Lock()
				delete(s.activeHandlers, workspaceName)
				s.mu.Unlock()
			}
			s.activeHandlers[workspaceName] = handler
			s.mu.Unlock()

			err = s.manager.StartAgent(ctx, workspaceName, sessionName, task, handler)
			if err == nil {
				log.Printf("[Timer] Agente iniciado com sucesso após timer no workspace: %s", workspaceName)
				return
			}

			// Se falhou por outro motivo que não seja "já existe um agente em execução", não adianta tentar novamente
			if !strings.Contains(err.Error(), "já existe um agente em execução") {
				log.Printf("[Timer] Erro fatal ao iniciar agente após timer: %v", err)
				s.mu.Lock()
				delete(s.activeHandlers, workspaceName)
				s.mu.Unlock()
				return
			}

			// Limpa o handler temporário que falhou ao iniciar e tenta novamente
			s.mu.Lock()
			delete(s.activeHandlers, workspaceName)
			s.mu.Unlock()

			select {
			case <-ticker.C:
				log.Printf("[Timer] Workspace %s ainda ocupado com iteração anterior, tentando novamente...", workspaceName)
			case <-timeout:
				log.Printf("[Timer] Timeout de 30s esgotado tentando iniciar agente no workspace %s ocupado: %v", workspaceName, err)
				return
			}
		}
	})
}
