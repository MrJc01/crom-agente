package git_log

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
		panic("falha ao carregar metadados de git_log: " + err.Error())
	}
}

// GitLogEntry representa um commit do git log
type GitLogEntry struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// GitLogTool retorna git log de forma estruturada
type GitLogTool struct {
	workspaceRoot string
}

// NewGitLogTool cria a ferramenta git_log
func NewGitLogTool(workspaceRoot string) *GitLogTool {
	return &GitLogTool{workspaceRoot: workspaceRoot}
}

func (t *GitLogTool) ID() string { return metadata.ID }

func (t *GitLogTool) Description() string {
	return metadata.Description
}

func (t *GitLogTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"limit": {
				"type": "integer",
				"description": "Número máximo de commits a retornar (default: 20)",
				"default": 20
			}
		},
		"required": []
	}`)
}

func (t *GitLogTool) RequiresApproval() bool { return false }

func (t *GitLogTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if err := tools.ValidateGitRepo(t.workspaceRoot); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	var input struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(args, &input)
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}

	// Formato delimitado por \x1f (unit separator) para parse seguro
	format := "%H\x1f%an\x1f%ai\x1f%s"
	cmd := exec.CommandContext(ctx, "git", "log",
		fmt.Sprintf("-n%d", input.Limit),
		fmt.Sprintf("--format=%s", format),
	)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao executar git log: %s (%s)", err, string(out))}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	entries := make([]GitLogEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) < 4 {
			continue
		}
		entries = append(entries, GitLogEntry{
			Hash:    parts[0][:min(12, len(parts[0]))],
			Author:  parts[1],
			Date:    parts[2],
			Subject: parts[3],
		})
	}

	data, _ := json.MarshalIndent(entries, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b

}
