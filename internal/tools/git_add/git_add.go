package git_add

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de git_add: " + err.Error())
	}
}

// GitAddTool executa git add em arquivos específicos ou em todos os arquivos
type GitAddTool struct {
	workspaceRoot string
}

// NewGitAddTool cria a ferramenta git_add
func NewGitAddTool(workspaceRoot string) *GitAddTool {
	return &GitAddTool{workspaceRoot: workspaceRoot}
}

func (t *GitAddTool) ID() string { return metadata.ID }

func (t *GitAddTool) Description() string {
	return metadata.Description
}

func (t *GitAddTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"paths": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Lista de caminhos de arquivos para adicionar ao stage. Use [\".\"] para adicionar tudo."
			}
		},
		"required": ["paths"]
	}`)
}

// RequiresApproval — git add altera o stage, exige aprovação média
func (t *GitAddTool) RequiresApproval() bool { return true }

func (t *GitAddTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if err := tools.ValidateGitRepo(t.workspaceRoot); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var input struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	if len(input.Paths) == 0 {
		return tools.Result{Success: false, Error: "nenhum caminho especificado para git add"}, nil
	}

	gitArgs := append([]string{"add"}, input.Paths...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao executar git add: %s (%s)", err, string(out))}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Arquivos adicionados ao stage: %s", strings.Join(input.Paths, ", ")),
	}, nil
}
