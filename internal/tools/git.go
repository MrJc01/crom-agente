package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---------- git_status ----------

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

func (t *GitStatusTool) ID() string { return "git_status" }

func (t *GitStatusTool) Description() string {
	return "Retorna o status de arquivos modificados, adicionados e não rastreados no repositório Git do workspace, em formato estruturado."
}

func (t *GitStatusTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *GitStatusTool) RequiresApproval() bool { return false }

func (t *GitStatusTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if err := validateGitRepo(t.workspaceRoot); err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1")
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao executar git status: %s (%s)", err, string(out))}, nil
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
	return Result{Success: true, Data: string(data)}, nil
}

// ---------- git_log ----------

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

func (t *GitLogTool) ID() string { return "git_log" }

func (t *GitLogTool) Description() string {
	return "Retorna o histórico de commits do repositório Git em formato estruturado. Aceita parâmetro 'limit' para limitar a quantidade de commits retornados."
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

func (t *GitLogTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if err := validateGitRepo(t.workspaceRoot); err != nil {
		return Result{Success: false, Error: err.Error()}, nil
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
		return Result{Success: false, Error: fmt.Sprintf("erro ao executar git log: %s (%s)", err, string(out))}, nil
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
	return Result{Success: true, Data: string(data)}, nil
}

// ---------- git_diff ----------

// GitDiffTool retorna o git diff atual (staged ou unstaged)
type GitDiffTool struct {
	workspaceRoot string
}

// NewGitDiffTool cria a ferramenta git_diff
func NewGitDiffTool(workspaceRoot string) *GitDiffTool {
	return &GitDiffTool{workspaceRoot: workspaceRoot}
}

func (t *GitDiffTool) ID() string { return "git_diff" }

func (t *GitDiffTool) Description() string {
	return "Retorna o diff atual do repositório Git. Aceita 'staged' (booleano) para mostrar apenas alterações no stage."
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

func (t *GitDiffTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if err := validateGitRepo(t.workspaceRoot); err != nil {
		return Result{Success: false, Error: err.Error()}, nil
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
		return Result{Success: false, Error: fmt.Sprintf("erro ao executar git diff: %s (%s)", err, string(out))}, nil
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

	return Result{Success: true, Data: output}, nil
}

// ---------- Helpers ----------

// validateGitRepo verifica se o diretório é um repositório git válido
func validateGitRepo(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = gitDir // evita warning
		return fmt.Errorf("o diretório '%s' não é um repositório Git válido: %s", dir, strings.TrimSpace(string(out)))
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
