package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creack/pty"
)

// RunTestsTool executa testes unitários/integração baseando-se na stack técnica detectada
type RunTestsTool struct {
	workspaceRoot string
}

// NewRunTestsTool cria a ferramenta run_tests
func NewRunTestsTool(workspaceRoot string) *RunTestsTool {
	return &RunTestsTool{workspaceRoot: workspaceRoot}
}

func (t *RunTestsTool) ID() string { return "run_tests" }

func (t *RunTestsTool) Description() string {
	return `Descobre e executa rotinas de teste no workspace com base na stack detectada (Go, Node, Rust, Python).
Aceita opcionalmente um parâmetro "command" para customizar o comando de testes.`
}

func (t *RunTestsTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Comando de teste customizado (se omitido, a ferramenta detecta automaticamente)"
			}
		},
		"required": []
	}`)
}

// RequiresApproval — Executar testes exige aprovação HITL média
func (t *RunTestsTool) RequiresApproval() bool { return true }

func (t *RunTestsTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(args, &input)

	cmdStr := strings.TrimSpace(input.Command)
	if cmdStr == "" {
		// Detecta comando baseado na stack
		cmdStr = t.detectTestCommand()
	}

	if cmdStr == "" {
		return Result{Success: false, Error: "não foi possível detectar uma suite de testes automática no workspace. Por favor, forneça o comando explicitamente."}, nil
	}

	// Executa em PTY
	c := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	c.Dir = t.workspaceRoot

	f, err := pty.Start(c)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao iniciar runner de teste: %s", err)}, nil
	}
	defer f.Close()

	// Lê output
	outChan := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 1024)
		for {
			n, readErr := f.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		outChan <- sb.String()
	}()

	select {
	case <-ctx.Done():
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		return Result{Success: false, Error: "testes cancelados por timeout ou interrupção"}, ctx.Err()
	case out := <-outChan:
		_ = c.Wait()
		return Result{
			Success: c.ProcessState.Success(),
			Data:    out,
		}, nil
	}
}

// detectTestCommand escaneia o workspace em busca de arquivos de build/config e sugere o comando de testes correspondente
func (t *RunTestsTool) detectTestCommand() string {
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "go.mod")); err == nil {
		return "go test ./..."
	}
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "package.json")); err == nil {
		return "npm test"
	}
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "Cargo.toml")); err == nil {
		return "cargo test"
	}
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "requirements.txt")); err == nil {
		return "pytest"
	}
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "pyproject.toml")); err == nil {
		return "poetry run pytest"
	}
	return ""
}
