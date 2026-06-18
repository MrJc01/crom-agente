package daemon

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

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

			case "set_auto_approve":
				s.mu.Lock()
				h, exists := s.activeHandlers[msg.Workspace]
				if exists {
					h.autoApprove = msg.AutoApprove
					log.Printf("[daemon WS] Dynamically set autoApprove to %t for workspace %s\n", msg.AutoApprove, msg.Workspace)
				}
				s.mu.Unlock()

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
					autoApprove:   msg.AutoApprove,
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

			case "stop":
				log.Printf("[daemon WS stop] Stopping agent for workspace: %s\n", msg.Workspace)
				err := s.manager.StopAgent(msg.Workspace)
				if err != nil {
					log.Printf("[daemon WS stop] Error: %v\n", err)
				} else {
					stoppedPayload, _ := json.Marshal(map[string]string{"type": "status", "status": "idle"})
					eventCh <- IPCResponse{Success: true, Stream: false, Data: stoppedPayload}
				}
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
