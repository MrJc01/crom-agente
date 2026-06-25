package git_status

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
		panic("falha ao carregar metadados de git_status: " + err.Error())
	}
}

// GitStatusEntry representa um arquivo alterado no git status
type GitStatusEntry struct {
	Status string `json:"status"` // "M", "A", "D", "??"
	File   string `json:"file"`
}

// GitStatusTool retorna git status de forma estruturada
type GitStatusTool struct {
	workspaceRoot string
}

// NewGitStatusTool cria a ferramenta git_status
func NewGitStatusTool(workspaceRoot string) *GitStatusTool {
	return &GitStatusTool{workspaceRoot: workspaceRoot}
}

func (t *GitStatusTool) ID() string { return metadata.ID }

func (t *GitStatusTool) Description() string {
	return metadata.Description
}

func (t *GitStatusTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *GitStatusTool) RequiresApproval() bool { return false }

func (t *GitStatusTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if err := tools.ValidateGitRepo(t.workspaceRoot); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1")
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao executar git status: %s (%s)", err, string(out))}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]GitStatusEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		file := strings.TrimSpace(line[3:])
		entries = append(entries, GitStatusEntry{Status: status, File: file})
	}

	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}
