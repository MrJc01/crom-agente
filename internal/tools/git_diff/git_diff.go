package git_diff

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de git_diff: " + err.Error())
	}
}

// GitDiffTool retorna o git diff atual (staged ou unstaged)
type GitDiffTool struct {
	workspaceRoot string
}

// NewGitDiffTool cria a ferramenta git_diff
func NewGitDiffTool(workspaceRoot string) *GitDiffTool {
	return &GitDiffTool{workspaceRoot: workspaceRoot}
}

func (t *GitDiffTool) ID() string { return metadata.ID }

func (t *GitDiffTool) Description() string {
	return metadata.Description
}

func (t *GitDiffTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"staged": {
				"type": "boolean",
				"description": "Se true, mostra apenas diff de arquivos no stage (git diff --cached)",
				"default": false
			}
		},
		"required": []
	}`)
}

func (t *GitDiffTool) RequiresApproval() bool { return false }

func (t *GitDiffTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if err := tools.ValidateGitRepo(t.workspaceRoot); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var input struct {
		Staged bool `json:"staged"`
	}
	_ = json.Unmarshal(args, &input)

	gitArgs := []string{"diff"}
	if input.Staged {
		gitArgs = append(gitArgs, "--cached")
	}

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao executar git diff: %s (%s)", err, string(out))}, nil
	}

	output := string(out)
	if output == "" {
		output = "(sem diferenças)"
	}

	// Limitar output para evitar estourar contexto do LLM
	const maxDiffLen = 32000
	if len(output) > maxDiffLen {
		output = output[:maxDiffLen] + "\n\n... [diff truncado, total: " + fmt.Sprintf("%d", len(string(out))) + " bytes]"
	}

	return tools.Result{Success: true, Data: output}, nil
}
