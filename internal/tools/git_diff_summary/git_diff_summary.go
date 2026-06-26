package git_diff_summary

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
		panic("falha ao carregar metadados de git_diff_summary: " + err.Error())
	}
}

type GitDiffSummaryTool struct {
	workspaceRoot string
}

func NewGitDiffSummaryTool(workspaceRoot string) *GitDiffSummaryTool {
	return &GitDiffSummaryTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *GitDiffSummaryTool) ID() string {
	return metadata.ID
}

func (t *GitDiffSummaryTool) Description() string {
	return metadata.Description
}

func (t *GitDiffSummaryTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho opcional para limitar o diff a um arquivo ou diretório específico"
			}
		}
	}`)
}

func (t *GitDiffSummaryTool) RequiresApproval() bool {
	return false
}

func (t *GitDiffSummaryTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	cmdArgs := []string{"diff"}
	if input.Path != "" {
		cmdArgs = append(cmdArgs, input.Path)
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = t.workspaceRoot
	outBytes, err := cmd.Output()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao executar git diff: %s", err)}, nil
	}

	diffStr := string(outBytes)
	if len(strings.TrimSpace(diffStr)) == 0 {
		return tools.Result{Success: true, Data: "Nenhuma alteração detectada."}, nil
	}

	// Limpar metadados inúteis
	lines := strings.Split(diffStr, "\n")
	var cleanLines []string
	
	for _, line := range lines {
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "diff --git ") {
			continue // Pula metadados do git
		}
		cleanLines = append(cleanLines, line)
	}

	return tools.Result{Success: true, Data: strings.Join(cleanLines, "\n")}, nil
}
