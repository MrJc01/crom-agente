package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/tools"
)

// Result contem o status e sumario da execucao da tarefa
type Result struct {
	Summary string
	Status  string
}

// sdkEventHandler escuta os eventos da execucao
type sdkEventHandler struct {
	mu          sync.Mutex
	status      string
	lastMessage string
	customTools []tools.Tool
}

func (h *sdkEventHandler) OnStatusChange(status string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = status
}

func (h *sdkEventHandler) OnMessage(role string, content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if role == "assistant" {
		h.lastMessage = content
	}
}

func (h *sdkEventHandler) OnEvent(event loop.AgentEvent) {
	// SDK armazena eventos para consumo programático futuro
}

// sdkEventWaitHandler escuta as mudancas de status e sinaliza doneChan ao terminar
type sdkEventWaitHandler struct {
	inner    *sdkEventHandler
	doneChan chan struct{}
}

func (h *sdkEventWaitHandler) OnStatusChange(status string) {
	h.inner.OnStatusChange(status)
	isFinished := status == "finished" || status == "idle" || strings.HasPrefix(status, "error:")
	if isFinished {
		select {
		case <-h.doneChan:
			// Ja sinalizado
		default:
			close(h.doneChan)
		}
	}
}

func (h *sdkEventWaitHandler) OnMessage(role string, content string) {
	h.inner.OnMessage(role, content)
}

func (h *sdkEventWaitHandler) OnEvent(event loop.AgentEvent) {
	h.inner.OnEvent(event)
}

func (h *sdkEventWaitHandler) GetCustomTools() []tools.Tool {
	return h.inner.customTools
}

// Agent representa a instancia programatica do agente
type Agent struct {
	agentID     string
	config      AgentConfig
	manager     *Manager
	workspace   string
	path        string
	customTools []tools.Tool
	SessionName string // ID ou nome da sessão para isolamento
}

// RegisterTool adiciona uma ferramenta customizada em Go para o agente
func (a *Agent) RegisterTool(t tools.Tool) {
	a.customTools = append(a.customTools, t)
}

// ExecuteTask executa uma tarefa utilizando o ReAct Loop do agente e aguarda a conclusao
func (a *Agent) ExecuteTask(ctx context.Context, task string) (*Result, error) {
	handler := &sdkEventHandler{
		customTools: a.customTools,
	}

	doneChan := make(chan struct{})
	wrappedHandler := &sdkEventWaitHandler{
		inner:    handler,
		doneChan: doneChan,
	}

	// Executa agente em background
	err := a.manager.internal.StartAgent(ctx, a.workspace, a.SessionName, task, wrappedHandler)
	if err != nil {
		return nil, err
	}

	// Aguarda conclusao do loop
	select {
	case <-doneChan:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	handler.mu.Lock()
	res := &Result{
		Summary: handler.lastMessage,
		Status:  handler.status,
	}
	handler.mu.Unlock()

	return res, nil
}

// scriptTool representa um script externo carregado dinamicamente
type scriptTool struct {
	path string
	name string
}

func (s *scriptTool) ID() string {
	return s.name
}

func (s *scriptTool) Description() string {
	return fmt.Sprintf("Executa o script local %s com os argumentos fornecidos", s.name)
}

func (s *scriptTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"arguments": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Lista de argumentos de linha de comando para passar ao script"
			}
		},
		"required": ["arguments"]
	}`)
}

func (s *scriptTool) RequiresApproval() bool {
	return true
}

func (s *scriptTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Arguments []string `json:"arguments"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos invalidos"}, err
	}

	cmd := exec.CommandContext(ctx, s.path, input.Arguments...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("erro ao executar script: %v. Saida: %s", err, string(output)),
		}, nil
	}

	return tools.Result{
		Success: true,
		Data:    string(output),
	}, nil
}

// LoadScriptsFromDir carrega todos os scripts de uma pasta como ferramentas do agente
func (a *Agent) LoadScriptsFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		name := entry.Name()
		toolName := strings.Split(name, ".")[0]
		toolName = strings.ReplaceAll(toolName, "-", "_")
		toolName = strings.ReplaceAll(toolName, " ", "_")

		a.RegisterTool(&scriptTool{
			path: path,
			name: toolName,
		})
	}
	return nil
}
