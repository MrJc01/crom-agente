package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
)

func TestMCPManager_SSEConnection(t *testing.T) {
	// Cria servidor HTTP mock para simular o endpoint SSE e RPC do servidor MCP
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			// Envia uma linha de evento para manter a conexão aberta/inicializar
			w.Write([]byte("data: {}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// Segura a conexão aberta
			time.Sleep(1 * time.Second)
			return
		}

		if r.URL.Path == "/rpc" && r.Method == http.MethodPost {
			var req struct {
				Method string `json:"method"`
				ID     int64  `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if req.Method == "initialize" {
				w.Write([]byte(`{"jsonrpc": "2.0", "id": 1, "result": {"protocolVersion": "2024-11-05", "serverInfo": {"name": "test-sse"}}}`))
			} else if req.Method == "tools/list" {
				w.Write([]byte(`{"jsonrpc": "2.0", "id": 2, "result": {"tools": [{"name": "test_tool", "description": "desc", "inputSchema": {"type": "object"}}]}}`))
			} else {
				w.Write([]byte(`{"jsonrpc": "2.0", "id": 3, "result": {}}`))
			}
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	mgr := NewMCPManager()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg := config.MCPServerConfig{
		Name: "test-sse-server",
		URL:  server.URL,
	}

	err := mgr.StartServer(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to start SSE server: %v", err)
	}
	defer mgr.StopAll()

	statuses := mgr.GetServerStatus()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	status := statuses[0]
	if status.Name != "test-sse-server" || status.Mode != "sse" || !status.Running {
		t.Errorf("unexpected server status: %+v", status)
	}

	tools := mgr.GetAllTools()
	if len(tools) != 1 || tools[0].ID() != "test_tool" {
		t.Errorf("unexpected tools: %+v", tools)
	}

	statusJSON, err := mgr.MCPStatusJSON()
	if err != nil {
		t.Fatalf("failed to get status JSON: %v", err)
	}

	var parsedStatuses []MCPServerStatus
	if err := json.Unmarshal(statusJSON, &parsedStatuses); err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}
	if len(parsedStatuses) != 1 || parsedStatuses[0].Name != "test-sse-server" {
		t.Errorf("unexpected parsed statuses: %+v", parsedStatuses)
	}

	err = mgr.StopServer("test-sse-server")
	if err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	statusesAfter := mgr.GetServerStatus()
	if len(statusesAfter) != 0 {
		t.Errorf("expected 0 statuses after stopping, got %d", len(statusesAfter))
	}
}
