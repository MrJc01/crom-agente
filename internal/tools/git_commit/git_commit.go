package git_commit

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
		panic("falha ao carregar metadados de git_commit: " + err.Error())
	}
}

// GitCommitTool executa git commit com mensagem fornecida
type GitCommitTool struct {
	workspaceRoot string
}

// NewGitCommitTool cria a ferramenta git_commit
func NewGitCommitTool(workspaceRoot string) *GitCommitTool {
	return &GitCommitTool{workspaceRoot: workspaceRoot}
}

func (t *GitCommitTool) ID() string { return metadata.ID }

func (t *GitCommitTool) Description() string {
	return metadata.Description
}

func (t *GitCommitTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"message": {
				"type": "string",
				"description": "Mensagem do commit. Deve seguir Conventional Commits: type(scope): description"
			}
		},
		"required": ["message"]
	}`)
}

// RequiresApproval — commits são ações com impacto médio, exigem HITL
func (t *GitCommitTool) RequiresApproval() bool { return true }

func (t *GitCommitTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if err := tools.ValidateGitRepo(t.workspaceRoot); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var input struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	msg := strings.TrimSpace(input.Message)
	if msg == "" {
		return tools.Result{Success: false, Error: "mensagem de commit não pode ser vazia"}, nil
	}

	// Validar formato Conventional Commits (soft check)
	if !isConventionalCommit(msg) {
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("mensagem de commit não segue Conventional Commits. Formato esperado: type(scope): description. Recebido: %q", msg),
		}, nil
	}

	// Verificar se há algo no stage
	checkCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	checkCmd.Dir = t.workspaceRoot
	if err := checkCmd.Run(); err == nil {
		return tools.Result{Success: false, Error: "nenhuma alteração no stage para commitar. Use git_add antes."}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "commit", "-m", msg)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao executar git commit: %s (%s)", err, string(out))}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Commit criado com sucesso.\n%s", strings.TrimSpace(string(out))),
	}, nil
}

// conventionalTypes são os tipos permitidos pelo padrão Conventional Commits
var conventionalTypes = []string{
	"feat", "fix", "docs", "style", "refactor", "perf",
	"test", "build", "ci", "chore", "revert",
}

// isConventionalCommit verifica se a mensagem segue o formato type(scope): description
// ou type: description (scope é opcional)
func isConventionalCommit(msg string) bool {
	for _, t := range conventionalTypes {
		// type(scope): description
		if strings.HasPrefix(msg, t+"(") {
			idx := strings.Index(msg, "): ")
			if idx > len(t)+1 {
				return true
			}
		}
		// type: description
		if strings.HasPrefix(msg, t+": ") {
			return true
		}
		// type!: breaking change
		if strings.HasPrefix(msg, t+"!: ") {
			return true
		}
	}
	return false
}
