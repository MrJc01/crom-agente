package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

// Helper process para simular servidor MCP em subprocesso de teste
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		var resp JSONRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = json.RawMessage(`{"protocolVersion": "2024-11-05", "serverInfo": {"name": "mock-server"}}`)
		case "tools/list":
			resp.Result = json.RawMessage(`{"tools": [{"name": "mcp_tool", "description": "mock tool", "inputSchema": {"type": "object"}}]}`)
		case "tools/call":
			resp.Result = json.RawMessage(`{"content": [{"type": "text", "text": "mcp response text"}]}`)
		default:
			continue
		}

		data, _ := json.Marshal(resp)
		fmt.Println(string(data))
	}
}

func TestMCPClient_E2E(t *testing.T) {
	// Cria o subprocesso de teste apontando para o próprio binário do teste
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := os.Args[0]
	args := []string{"-test.run=TestHelperProcess"}

	// Define variável de ambiente para ativar o loop do helper process
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	client, err := NewMCPClient(cmd, args)
	if err != nil {
		t.Fatalf("erro ao criar MCPClient: %v", err)
	}
	defer client.Close()

	// 1. Testa Inicialização (Handshake)
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("falha no handshake do MCP: %v", err)
	}

	// 2. Testa Listagem de Ferramentas
	list, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("falha ao listar ferramentas do MCP: %v", err)
	}
	if len(list) != 1 || list[0].Name != "mcp_tool" {
		t.Fatalf("lista de ferramentas MCP inválida: %+v", list)
	}

	// 3. Testa Execução de Ferramenta
	val, err := client.CallTool(ctx, "mcp_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("falha ao rodar ferramenta MCP: %v", err)
	}
	if val != "mcp response text" {
		t.Fatalf("saída incorreta: %s", val)
	}

	// 4. Testa Tool Wrapper
	wrapper := NewMCPToolWrapper(client, list[0])
	if wrapper.ID() != "mcp_tool" {
		t.Fatalf("ID incorreto no wrapper")
	}

	res, err := wrapper.Execute(ctx, json.RawMessage(`{}`))
	if err != nil || !res.Success {
		t.Fatalf("erro na execução do wrapper: %v, res: %+v", err, res)
	}
	if res.Data != "mcp response text" {
		t.Fatalf("dados do wrapper incorretos: %s", res.Data)
	}
}
