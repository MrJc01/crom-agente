package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// registerRoutes registra todos os endpoints HTTP e WebSocket do daemon
func (s *APIServer) registerRoutes(mux *http.ServeMux) {
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
	mux.HandleFunc("/api/terminal/buffer", s.handleTerminalBuffer)
	mux.HandleFunc("/api/terminal/info", s.handleTerminalInfo)
	mux.HandleFunc("/api/project/files", s.handleProjectFiles)
	mux.HandleFunc("/api/system/info", s.handleSystemInfo)
	mux.HandleFunc("/api/transcribe", s.handleTranscribe)
	mux.HandleFunc("/api/record/start", s.handleRecordStart)
	mux.HandleFunc("/api/record/stop", s.handleRecordStop)
	mux.HandleFunc("/api/devices/audio", s.handleDevicesAudio)
	mux.HandleFunc("/api/devices/screens", s.handleDevicesScreens)
	mux.HandleFunc("/api/mcp/status", s.handleMCPStatus)
	mux.HandleFunc("/api/browser/proxy", s.handleBrowserProxy)
	mux.HandleFunc("/api/reveal", s.handleReveal)
	mux.HandleFunc("/api/agent/telemetry", s.handleAgentTelemetry)
	mux.HandleFunc("/api/agent/telemetry/ws", s.handleAgentTelemetryWS)
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

	if isFinished && h.onFinished != nil {
		h.onFinished()
	}
}

func (h *daemonAPIEventHandler) OnStreamChunk(chunk string) {
	payload, _ := json.Marshal(map[string]string{
		"type":    "stream_chunk",
		"content": chunk,
	})

	h.router.Broadcast(h.workspaceName, IPCResponse{
		Success: true,
		Stream:  true,
		Data:    payload,
	})
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

func (s *APIServer) handleAgentTelemetry(w http.ResponseWriter, r *http.Request) {
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

	workspace := r.URL.Query().Get("workspace")
	if workspace == "" {
		http.Error(w, "parametro 'workspace' obrigatorio", http.StatusBadRequest)
		return
	}
	session := r.URL.Query().Get("session")

	telemetry, err := s.manager.GetAgentTelemetry(workspace, session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(telemetry)
}

func (s *APIServer) handleAgentTelemetryWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		log.Printf("[APIServer Telemetry WS] Rejeitando conexao por falha na autorizacao")
		return
	}

	workspace := r.URL.Query().Get("workspace")
	if workspace == "" {
		http.Error(w, "parametro 'workspace' obrigatorio", http.StatusBadRequest)
		return
	}
	session := r.URL.Query().Get("session")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[APIServer Telemetry WS] Erro ao realizar upgrade: %v", err)
		return
	}
	defer conn.Close()

	// Envia snapshot inicial
	telemetry, err := s.manager.GetAgentTelemetry(workspace, session)
	if err == nil {
		if wErr := conn.WriteJSON(telemetry); wErr != nil {
			log.Printf("[APIServer Telemetry WS] Erro WriteJSON inicial: %v", wErr)
		} else {
			log.Printf("[APIServer Telemetry WS] Snapshot inicial enviado com sucesso para %s", workspace)
		}
	} else {
		log.Printf("[APIServer Telemetry WS] Erro no snapshot inicial (GetAgentTelemetry): %v", err)
	}

	eventCh := make(chan IPCResponse, 100)
	s.router.Register(workspace, eventCh)
	defer s.router.Unregister(workspace, eventCh)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	closeChan := make(chan struct{})

	// Read loop (apenas para detectar desconexao ou mensagens do cliente)
	go func() {
		defer close(closeChan)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	var lastJSON string

	sendUpdate := func() {
		telemetry, err := s.manager.GetAgentTelemetry(workspace, session)
		if err != nil {
			log.Printf("[APIServer Telemetry WS] Erro GetAgentTelemetry em sendUpdate: %v", err)
			return
		}
		data, err := json.Marshal(telemetry)
		if err != nil {
			return
		}
		currentJSON := string(data)
		if currentJSON != lastJSON {
			lastJSON = currentJSON
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	for {
		select {
		case <-eventCh:
			sendUpdate()
		case <-ticker.C:
			sendUpdate()
		case <-closeChan:
			return
		}
	}
}
