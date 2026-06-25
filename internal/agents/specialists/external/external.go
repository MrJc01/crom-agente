package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/crom/crom-agente/internal/agents/core"
)

// ExternalAgent implementa a interface core.Agent permitindo rodar agentes em subprocessos (ex: python, node)
type ExternalAgent struct {
	name         string
	description  string
	systemPrompt string
	toolIDs      []string
	execPath     string
	args         []string
	timeout      time.Duration
}

// NewExternalAgent cria uma nova instância de ExternalAgent
func NewExternalAgent(name, description, systemPrompt string, toolIDs []string, execPath string, args []string, timeout time.Duration) *ExternalAgent {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ExternalAgent{
		name:         name,
		description:  description,
		systemPrompt: systemPrompt,
		toolIDs:      toolIDs,
		execPath:     execPath,
		args:         args,
		timeout:      timeout,
	}
}

// Name retorna o nome do agente externo
func (e *ExternalAgent) Name() string {
	return e.name
}

// Description retorna a descrição do agente externo
func (e *ExternalAgent) Description() string {
	return e.description
}

// SystemPrompt retorna as diretrizes do sistema para o agente externo
func (e *ExternalAgent) SystemPrompt() string {
	return e.systemPrompt
}

// ToolIDs retorna a lista de ferramentas que o agente externo necessita
func (e *ExternalAgent) ToolIDs() []string {
	return e.toolIDs
}

// Execute executa o subprocesso passando entrada JSON no stdin e lendo o resultado no stdout
func (e *ExternalAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, e.execPath, e.args...)

	input := struct {
		Prompt       string `json:"prompt"`
		PriorSummary string `json:"prior_summary"`
	}{
		Prompt:       prompt,
		PriorSummary: priorSummary,
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return core.AgentResult{Success: false}, fmt.Errorf("falha ao serializar entrada do external agent: %w", err)
	}

	cmd.Stdin = bytes.NewReader(inputBytes)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return core.AgentResult{Success: false}, fmt.Errorf("timeout na execução do external agent (%s)", e.timeout)
		}
		return core.AgentResult{Success: false}, fmt.Errorf("falha ao executar subprocesso: %w (stderr: %s)", err, stderrBuf.String())
	}

	var result core.AgentResult
	if err := json.Unmarshal(stdoutBuf.Bytes(), &result); err != nil {
		// Fallback se não for JSON: trata stdout cru como output de sucesso
		return core.AgentResult{
			Success:        true,
			Output:         stdoutBuf.String(),
			ContextSummary: "",
		}, nil
	}

	return result, nil
}
