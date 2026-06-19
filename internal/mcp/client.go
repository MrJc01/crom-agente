package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

// JSONRPCRequest define uma requisição JSON-RPC 2.0
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int64       `json:"id"`
}

// JSONRPCResponse define uma resposta JSON-RPC 2.0
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      int64           `json:"id"`
}

// JSONRPCError define uma estrutura de erro RPC
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPTool representa a definição de ferramenta exposta pelo servidor MCP
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// MCPClient gerencia a comunicação JSON-RPC com o servidor MCP subprocesso
type MCPClient struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	scanner  *bufio.Scanner
	idGen    int64
	mu       sync.Mutex
	pending  map[int64]chan *JSONRPCResponse
	isClosed bool
}

// NewMCPClient inicia o servidor subprocesso e cria o cliente MCP
func NewMCPClient(command string, args []string) (*MCPClient, error) {
	c := exec.Command(command, args...)
	return NewMCPClientFromCmd(c)
}

// NewMCPClientFromCmd aceita um exec.Cmd já configurado (com env, dir, etc.) e inicia o cliente MCP
func NewMCPClientFromCmd(c *exec.Cmd) (*MCPClient, error) {
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := c.Start(); err != nil {
		return nil, err
	}

	client := &MCPClient{
		cmd:     c,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
		pending: make(map[int64]chan *JSONRPCResponse),
	}

	go client.listen()

	return client, nil
}

func (c *MCPClient) listen() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // Ignora mensagens não JSON ou inválidas
		}

		c.mu.Lock()
		ch, exists := c.pending[resp.ID]
		if exists {
			ch <- &resp
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
	}
}

// Call executa uma chamada JSON-RPC síncrona
func (c *MCPClient) Call(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := atomic.AddInt64(&c.idGen, 1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *JSONRPCResponse, 1)

	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return nil, fmt.Errorf("cliente mcp fechado")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	_, err = c.stdin.Write(append(jsonData, '\n'))
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("erro RPC %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

// Initialize realiza o handshake de inicialização com o servidor
func (c *MCPClient) Initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "crom-agente",
			"version": "0.1.0",
		},
	}

	_, err := c.Call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("erro ao inicializar: %w", err)
	}

	// Envia notificação initialized
	notification := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	jsonData, _ := json.Marshal(notification)
	_, _ = c.stdin.Write(append(jsonData, '\n'))

	return nil
}

// ListTools recupera todas as ferramentas expostas pelo servidor
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	return result.Tools, nil
}

// CallTool executa uma ferramenta do MCP
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsedArgs map[string]interface{}
	_ = json.Unmarshal(args, &parsedArgs)

	params := map[string]interface{}{
		"name":      name,
		"arguments": parsedArgs,
	}

	resp, err := c.Call(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("resposta mcp sem conteúdo")
	}

	return result.Content[0].Text, nil
}

// Close encerra o processo do servidor
func (c *MCPClient) Close() {
	c.mu.Lock()
	if c.isClosed {
		c.mu.Unlock()
		return
	}
	c.isClosed = true
	c.mu.Unlock()

	_ = c.stdin.Close()
	_ = c.stdout.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

// MCPCaller é uma interface que abstrai a comunicação com servidores MCP (subprocesso ou SSE)
type MCPCaller interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	Close()
}

// MCPToolWrapper implementa a interface tools.Tool para envelopar ferramentas do MCP
type MCPToolWrapper struct {
	client MCPCaller
	tool   MCPTool
}

// NewMCPToolWrapper cria um wrapper para ferramentas MCP (aceita qualquer MCPCaller)
func NewMCPToolWrapper(client MCPCaller, tool MCPTool) *MCPToolWrapper {
	return &MCPToolWrapper{
		client: client,
		tool:   tool,
	}
}

// ID retorna o identificador único
func (w *MCPToolWrapper) ID() string {
	return w.tool.Name
}

// Description retorna a descrição
func (w *MCPToolWrapper) Description() string {
	return w.tool.Description
}

// ParametersSchema retorna o JSON Schema
func (w *MCPToolWrapper) ParametersSchema() json.RawMessage {
	return w.tool.InputSchema
}

// RequiresApproval indica aprovação obrigatória por segurança
func (w *MCPToolWrapper) RequiresApproval() bool {
	return true
}


// Execute roda a chamada remota da ferramenta
func (w *MCPToolWrapper) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	out, err := w.client.CallTool(ctx, w.tool.Name, args)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}
	return tools.Result{Success: true, Data: out}, nil
}

// ---------------------------------------------------------------------------
// MCPClientSSE: cliente MCP para servidores remotos via HTTP (SSE / REST-RPC)
// ---------------------------------------------------------------------------

// MCPClientSSE conecta a um servidor MCP remoto que aceita requisições JSON-RPC via HTTP POST
// e emite eventos via Server-Sent Events (SSE).
type MCPClientSSE struct {
	baseURL    string
	httpClient *http.Client
	idGen      int64
}

// NewMCPClientSSE cria um cliente para um servidor MCP remoto acessível por URL
func NewMCPClientSSE(baseURL string) (*MCPClientSSE, error) {
	c := &MCPClientSSE{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	return c, nil
}

func (c *MCPClientSSE) call(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := atomic.AddInt64(&c.idGen, 1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("erro RPC SSE %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return &rpcResp, nil
}

// Initialize realiza o handshake com o servidor remoto
func (c *MCPClientSSE) Initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "crom-agente",
			"version": "0.1.0",
		},
	}
	_, err := c.call(ctx, "initialize", params)
	return err
}

// ListTools lista as ferramentas disponíveis no servidor remoto
func (c *MCPClientSSE) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool executa uma ferramenta no servidor remoto
func (c *MCPClientSSE) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var parsedArgs map[string]interface{}
	_ = json.Unmarshal(args, &parsedArgs)

	params := map[string]interface{}{
		"name":      name,
		"arguments": parsedArgs,
	}
	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("resposta mcp-sse sem conteúdo")
	}
	return result.Content[0].Text, nil
}

// Close fecha o cliente SSE (sem-op pois não há conexão persistente)
func (c *MCPClientSSE) Close() {}

