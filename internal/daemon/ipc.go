package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/orchestrator"
)

// SocketPath retorna o caminho do socket IPC
func SocketPath() (string, error) {
	dir, err := config.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crom-agente.sock"), nil
}

// PIDPath retorna o caminho do arquivo de PID
func PIDPath() (string, error) {
	dir, err := config.GlobalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crom-agente.pid"), nil
}

// IPCMessage eh o protocolo de comunicacao enviado pelo cliente
type IPCMessage struct {
	Type        string          `json:"type"`                // "run", "status", "stop", "ping", "permission_response"
	Workspace   string          `json:"workspace,omitempty"` // nome do workspace
	Task        string          `json:"task,omitempty"`
	Session     string          `json:"session,omitempty"` // ID ou nome da sessão
	Payload     json.RawMessage `json:"payload,omitempty"`
	Provider    string          `json:"provider,omitempty"`
	Model       string          `json:"model,omitempty"`
	AutoApprove bool            `json:"auto_approve,omitempty"`
}

// IPCResponse eh a resposta enviada do daemon para o cliente
type IPCResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Stream  bool            `json:"stream"` // Indica se mais pacotes serao enviados
}

type permissionResult struct {
	approved bool
	remember bool
}

// ipcConnectionHandler gerencia o envio de eventos e HITL para uma conexao especifica
type ipcConnectionHandler struct {
	workspaceName string
	router        *AgentEventsRouter
	permRespChan  chan permissionResult
	enc           *json.Encoder
	encMu         sync.Mutex
}

func (h *ipcConnectionHandler) writeResponse(resp IPCResponse) {
	h.encMu.Lock()
	defer h.encMu.Unlock()
	_ = h.enc.Encode(resp)
}

func (h *ipcConnectionHandler) OnStatusChange(status string) {
	payload, _ := json.Marshal(map[string]string{
		"type":   "status",
		"status": status,
	})

	isFinished := status == "finished" || status == "idle" || status == "waiting_user_input" || strings.HasPrefix(status, "error:")
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
}

func (h *ipcConnectionHandler) OnMessage(role string, content string) {
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

func (h *ipcConnectionHandler) AskPermission(action, target string) (bool, bool) {
	payload, _ := json.Marshal(map[string]string{
		"type":   "ask_permission",
		"action": action,
		"target": target,
	})

	h.writeResponse(IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})

	res := <-h.permRespChan
	return res.approved, res.remember
}

func (h *ipcConnectionHandler) OnEvent(event loop.AgentEvent) {
	payload, _ := json.Marshal(event)
	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})
}

// AgentEventsRouter gerencia canais de eventos por workspace
type AgentEventsRouter struct {
	mu        sync.Mutex
	listeners map[string]map[chan IPCResponse]struct{}
}

func NewAgentEventsRouter() *AgentEventsRouter {
	return &AgentEventsRouter{
		listeners: make(map[string]map[chan IPCResponse]struct{}),
	}
}

func (r *AgentEventsRouter) Register(workspaceName string, ch chan IPCResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listeners[workspaceName] == nil {
		r.listeners[workspaceName] = make(map[chan IPCResponse]struct{})
	}
	r.listeners[workspaceName][ch] = struct{}{}
}

func (r *AgentEventsRouter) Unregister(workspaceName string, ch chan IPCResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listeners[workspaceName] != nil {
		delete(r.listeners[workspaceName], ch)
	}
}

func (r *AgentEventsRouter) Broadcast(workspaceName string, resp IPCResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ch := range r.listeners[workspaceName] {
		select {
		case ch <- resp:
		default:
		}
	}
}

// IPCServer escuta comandos no socket Unix
type IPCServer struct {
	listener net.Listener
	manager  *orchestrator.MultiAgentManager
	router   *AgentEventsRouter
	quit     chan struct{}
}

func NewIPCServer(manager *orchestrator.MultiAgentManager) *IPCServer {
	return &IPCServer{
		manager: manager,
		router:  NewAgentEventsRouter(),
		quit:    make(chan struct{}),
	}
}

func (s *IPCServer) Start() error {
	sockPath, err := SocketPath()
	if err != nil {
		return err
	}

	_ = os.Remove(sockPath) // Garante que nao haja socket obsoleto

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("falha ao abrir socket IPC: %w", err)
	}
	s.listener = l

	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					log.Printf("[IPCServer] Erro ao aceitar conexao: %v", err)
					continue
				}
			}
			go s.handleConnection(conn)
		}
	}()

	return nil
}

func (s *IPCServer) Stop() {
	close(s.quit)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	sockPath, _ := SocketPath()
	if sockPath != "" {
		_ = os.Remove(sockPath)
	}
}

func (s *IPCServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	permRespChan := make(chan permissionResult, 1)
	closeChan := make(chan struct{})

	handler := &ipcConnectionHandler{
		router:       s.router,
		permRespChan: permRespChan,
		enc:          enc,
	}

	// Read loop rodando em background para a conexao
	go func() {
		defer close(closeChan)
		for {
			var msg IPCMessage
			err := dec.Decode(&msg)
			if err != nil {
				return
			}

			switch msg.Type {
			case "permission_response":
				var permPayload struct {
					Approved bool `json:"approved"`
					Remember bool `json:"remember"`
				}
				_ = json.Unmarshal(msg.Payload, &permPayload)
				select {
				case permRespChan <- permissionResult{approved: permPayload.Approved, remember: permPayload.Remember}:
				default:
				}

			case "ping":
				handler.writeResponse(IPCResponse{Success: true, Data: json.RawMessage(`"pong"`)})

			case "status":
				agents := s.manager.ListRunningAgents()
				wsList, _ := orchestrator.LoadWorkspaces()
				type WsStatus struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					Task   string `json:"task"`
				}
				var list []WsStatus
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
				handler.writeResponse(IPCResponse{Success: true, Data: data})

			case "stop":
				err := s.manager.StopAgent(msg.Workspace)
				if err != nil {
					handler.writeResponse(IPCResponse{Success: false, Error: err.Error()})
				} else {
					handler.writeResponse(IPCResponse{Success: true})
				}

			case "run":
				handler.workspaceName = msg.Workspace
				eventCh := make(chan IPCResponse, 100)
				s.router.Register(msg.Workspace, eventCh)

				ctx := context.Background()
				if msg.Provider != "" {
					ctx = context.WithValue(ctx, "provider_override", msg.Provider)
				}
				if msg.Model != "" {
					ctx = context.WithValue(ctx, "model_override", msg.Model)
				}

				// Loop de streaming
				go func(wName string, ec chan IPCResponse) {
					defer s.router.Unregister(wName, ec)
					for {
						select {
						case resp, ok := <-ec:
							if !ok {
								return
							}
							handler.writeResponse(resp)
							if !resp.Stream {
								return
							}
						case <-closeChan:
							return
						}
					}
				}(msg.Workspace, eventCh)

				// Notifica inicio
				startedPayload, _ := json.Marshal(map[string]string{"type": "started"})
				handler.writeResponse(IPCResponse{Success: true, Stream: true, Data: startedPayload})

				go func() {
					err := s.manager.StartAgent(ctx, msg.Workspace, msg.Session, msg.Task, handler)
					if err != nil {
						s.router.Unregister(msg.Workspace, eventCh)
						handler.writeResponse(IPCResponse{Success: false, Error: err.Error()})
						return
					}
				}()
			}
		}
	}()

	<-closeChan
}
