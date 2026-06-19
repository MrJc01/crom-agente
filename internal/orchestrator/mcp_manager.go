package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/mcp"
	"github.com/crom/crom-agente/internal/tools"
)

// MCPServerHandle representa um servidor MCP ativo (subprocesso ou SSE remoto)
type MCPServerHandle struct {
	Config  config.MCPServerConfig
	Client  mcp.MCPCaller  // pode ser *mcp.MCPClient (subprocesso) ou *mcp.MCPClientSSE (remoto)
	Tools   []*mcp.MCPToolWrapper
	sseConn io.ReadCloser // para servidores SSE remotos
}

// MCPManager gerencia o ciclo de vida de todos os servidores MCP configurados
type MCPManager struct {
	mu      sync.RWMutex
	servers map[string]*MCPServerHandle // chave: nome do servidor
}

// NewMCPManager cria um novo gerenciador de servidores MCP
func NewMCPManager() *MCPManager {
	return &MCPManager{
		servers: make(map[string]*MCPServerHandle),
	}
}

// StartAll inicia todos os servidores MCP da configuração global.
// Servidores com Command são iniciados como subprocessos.
// Servidores com URL são conectados via SSE.
func (m *MCPManager) StartAll(ctx context.Context, cfgs []config.MCPServerConfig) {
	for _, cfg := range cfgs {
		if cfg.Name == "" {
			log.Printf("[MCPManager] Servidor MCP sem nome ignorado.")
			continue
		}
		if err := m.StartServer(ctx, cfg); err != nil {
			log.Printf("[MCPManager] Erro ao iniciar servidor MCP '%s': %v", cfg.Name, err)
		} else {
			log.Printf("[MCPManager] Servidor MCP '%s' iniciado com sucesso.", cfg.Name)
		}
	}
}

// StartServer inicia um único servidor MCP (subprocesso ou SSE)
func (m *MCPManager) StartServer(ctx context.Context, cfg config.MCPServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[cfg.Name]; exists {
		return fmt.Errorf("servidor MCP '%s' já está rodando", cfg.Name)
	}

	handle := &MCPServerHandle{Config: cfg}

	if cfg.URL != "" {
		// Modo SSE remoto
		if err := m.connectSSE(ctx, cfg, handle); err != nil {
			return err
		}
	} else if cfg.Command != "" {
		// Modo subprocesso
		if err := m.startSubprocess(ctx, cfg, handle); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("servidor MCP '%s' precisa de 'command' ou 'url'", cfg.Name)
	}

	m.servers[cfg.Name] = handle
	return nil
}

// startSubprocess inicia o servidor MCP como subprocesso e conecta via stdio
func (m *MCPManager) startSubprocess(ctx context.Context, cfg config.MCPServerConfig, handle *MCPServerHandle) error {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)

	// Injeta variáveis de ambiente adicionais
	cmd.Env = append(os.Environ(), cfg.Env...)

	client, err := mcp.NewMCPClientFromCmd(cmd)
	if err != nil {
		return fmt.Errorf("falha ao iniciar subprocesso MCP '%s': %w", cfg.Name, err)
	}

	// Handshake de inicialização com timeout
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Initialize(initCtx); err != nil {
		client.Close()
		return fmt.Errorf("falha no handshake MCP '%s': %w", cfg.Name, err)
	}

	// Descobre ferramentas disponíveis
	listCtx, listCancel := context.WithTimeout(ctx, 10*time.Second)
	defer listCancel()

	mcpTools, err := client.ListTools(listCtx)
	if err != nil {
		client.Close()
		return fmt.Errorf("falha ao listar ferramentas MCP '%s': %w", cfg.Name, err)
	}

	handle.Client = client
	for _, t := range mcpTools {
		handle.Tools = append(handle.Tools, mcp.NewMCPToolWrapper(client, t))
	}

	log.Printf("[MCPManager] Subprocesso '%s' iniciado com %d ferramentas: %v",
		cfg.Name, len(mcpTools), toolNames(mcpTools))
	return nil
}

// connectSSE conecta a um servidor MCP remoto via Server-Sent Events (SSE)
func (m *MCPManager) connectSSE(ctx context.Context, cfg config.MCPServerConfig, handle *MCPServerHandle) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL+"/sse", nil)
	if err != nil {
		return fmt.Errorf("falha ao criar requisição SSE para '%s': %w", cfg.Name, err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao conectar ao servidor SSE '%s': %w", cfg.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("servidor SSE '%s' retornou status %d", cfg.Name, resp.StatusCode)
	}

	handle.sseConn = resp.Body

	// Cria um cliente MCP customizado que lê do SSE e posta para o endpoint HTTP do servidor
	sseClient, err := mcp.NewMCPClientSSE(cfg.URL)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("falha ao criar cliente SSE para '%s': %w", cfg.Name, err)
	}

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := sseClient.Initialize(initCtx); err != nil {
		sseClient.Close()
		resp.Body.Close()
		return fmt.Errorf("falha no handshake SSE MCP '%s': %w", cfg.Name, err)
	}

	listCtx, listCancel := context.WithTimeout(ctx, 10*time.Second)
	defer listCancel()

	mcpTools, err := sseClient.ListTools(listCtx)
	if err != nil {
		sseClient.Close()
		resp.Body.Close()
		return fmt.Errorf("falha ao listar ferramentas SSE MCP '%s': %w", cfg.Name, err)
	}

	// Goroutine para ler eventos SSE e logar
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)
				if data != "" && data != "[DONE]" {
					log.Printf("[MCPManager SSE '%s'] Evento: %s", cfg.Name, data)
				}
			}
		}
	}()

	handle.Client = sseClient
	for _, t := range mcpTools {
		handle.Tools = append(handle.Tools, mcp.NewMCPToolWrapper(sseClient, t))
	}

	log.Printf("[MCPManager] Servidor SSE '%s' conectado com %d ferramentas: %v",
		cfg.Name, len(mcpTools), toolNames(mcpTools))
	return nil
}

// StopAll encerra todos os servidores MCP ativos de forma graciosa
func (m *MCPManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, handle := range m.servers {
		if handle.Client != nil {
			handle.Client.Close()
		}
		if handle.sseConn != nil {
			_ = handle.sseConn.Close()
		}
		log.Printf("[MCPManager] Servidor MCP '%s' encerrado.", name)
		delete(m.servers, name)
	}
}

// StopServer encerra um servidor MCP específico
func (m *MCPManager) StopServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	handle, exists := m.servers[name]
	if !exists {
		return fmt.Errorf("servidor MCP '%s' não está rodando", name)
	}

	if handle.Client != nil {
		handle.Client.Close()
	}
	if handle.sseConn != nil {
		_ = handle.sseConn.Close()
	}
	delete(m.servers, name)
	log.Printf("[MCPManager] Servidor MCP '%s' encerrado.", name)
	return nil
}

// GetAllTools retorna todos os wrappers de ferramentas MCP registrados (de todos os servidores)
func (m *MCPManager) GetAllTools() []tools.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []tools.Tool
	for _, handle := range m.servers {
		for _, t := range handle.Tools {
			result = append(result, t)
		}
	}
	return result
}

// GetServerStatus retorna um snapshot do status de todos os servidores MCP
func (m *MCPManager) GetServerStatus() []MCPServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []MCPServerStatus
	for name, handle := range m.servers {
		mode := "subprocess"
		if handle.Config.URL != "" {
			mode = "sse"
		}
		toolNames := make([]string, 0, len(handle.Tools))
		for _, t := range handle.Tools {
			toolNames = append(toolNames, t.ID())
		}
		statuses = append(statuses, MCPServerStatus{
			Name:      name,
			Mode:      mode,
			ToolCount: len(handle.Tools),
			Tools:     toolNames,
			Running:   handle.Client != nil,
		})
	}
	return statuses
}

// MCPServerStatus representa o estado atual de um servidor MCP
type MCPServerStatus struct {
	Name      string   `json:"name"`
	Mode      string   `json:"mode"` // "subprocess" ou "sse"
	ToolCount int      `json:"tool_count"`
	Tools     []string `json:"tools"`
	Running   bool     `json:"running"`
}

// toolNames extrai apenas os nomes de uma lista de MCPTool para logging
func toolNames(ts []mcp.MCPTool) []string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Name
	}
	return names
}

// MCPStatusJSON retorna o status dos servidores serializado como JSON
func (m *MCPManager) MCPStatusJSON() ([]byte, error) {
	statuses := m.GetServerStatus()
	return json.Marshal(statuses)
}
