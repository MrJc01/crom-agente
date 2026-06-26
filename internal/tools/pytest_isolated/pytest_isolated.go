package pytest_isolated

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de pytest_isolated: " + err.Error())
	}
}

type PytestIsolatedTool struct {
	workspaceRoot string
}

func NewPytestIsolatedTool(workspaceRoot string) *PytestIsolatedTool {
	return &PytestIsolatedTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *PytestIsolatedTool) ID() string {
	return metadata.ID
}

func (t *PytestIsolatedTool) Description() string {
	return metadata.Description
}

func (t *PytestIsolatedTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"target": {
				"type": "string",
				"description": "Caminho do arquivo de teste ou spec do teste (ex: path/to/test.py::test_func)"
			}
		},
		"required": ["target"]
	}`)
}

func (t *PytestIsolatedTool) RequiresApproval() bool {
	return false
}

func (t *PytestIsolatedTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Target string `json:"target"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	// Forçar timeout de 15 segundos para o pytest isolado
	pyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmdArgs := []string{"-v", "--no-header", "--tb=short", "--disable-warnings", input.Target}
	cmd := exec.CommandContext(pyCtx, "pytest", cmdArgs...)
	cmd.Dir = t.workspaceRoot
	outBytes, err := cmd.CombinedOutput()

	if pyCtx.Err() == context.DeadlineExceeded {
		return tools.Result{Success: false, Error: "Pytest abortado: Timeout de 15 segundos excedido. Você provávelmente causou um loop infinito no código."}, nil
	}

	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("Pytest falhou:\n%s", string(outBytes))}, nil
	}

	return tools.Result{Success: true, Data: fmt.Sprintf("Pytest passou com sucesso:\n%s", string(outBytes))}, nil
}
