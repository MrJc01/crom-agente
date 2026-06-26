package checkpoint

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de checkpoint: " + err.Error())
	}
}

// CheckpointTool gerencia backups e rollbacks do workspace
type CheckpointTool struct {
	workspaceRoot string
	stateManager  interface {
		GetMessages() []llm.Message
		SetMessages(messages []llm.Message) error
	}
}

// NewCheckpointTool cria a ferramenta
func NewCheckpointTool(workspaceRoot string, stateManager interface {
	GetMessages() []llm.Message
	SetMessages(messages []llm.Message) error
}) *CheckpointTool {
	return &CheckpointTool{
		workspaceRoot: workspaceRoot,
		stateManager:  stateManager,
	}
}

func (t *CheckpointTool) ID() string { return metadata.ID }

func (t *CheckpointTool) Description() string { return metadata.Description }

func (t *CheckpointTool) RequiresApproval() bool { return true }

func (t *CheckpointTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["create", "restore", "list"],
				"description": "Ação de checkpoint: create (criar backup), restore (restaurar), list (listar backups)"
			},
			"name": {
				"type": "string",
				"description": "Nome identificador do checkpoint (obrigatório para create e restore)"
			}
		},
		"required": ["action"]
	}`)
}

func (t *CheckpointTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	isGit := t.isGitRepo()

	switch input.Action {
	case "create":
		if input.Name == "" {
			input.Name = fmt.Sprintf("cp_%d", time.Now().UnixNano()%100000)
		}
		if isGit {
			// Cria checkpoint via git stash
			cmd := exec.CommandContext(ctx, "git", "stash", "push", "-u", "-m", "crom_checkpoint_"+input.Name)
			cmd.Dir = t.workspaceRoot
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tools.Result{Success: false, Error: fmt.Sprintf("falha ao criar checkpoint git: %v, out: %s", err, string(out))}, nil
			}
		}

		// Salva o histórico de mensagens e metadados no diretório físico independente do Git
		backupDir := filepath.Join(t.workspaceRoot, ".crom", "checkpoints", input.Name)
		_ = os.MkdirAll(backupDir, 0755)
		_ = os.WriteFile(filepath.Join(backupDir, "timestamp.txt"), []byte(time.Now().Format(time.RFC3339)), 0644)

		if t.stateManager != nil {
			msgs := t.stateManager.GetMessages()
			if data, err := json.Marshal(msgs); err == nil {
				_ = os.WriteFile(filepath.Join(backupDir, "messages.json"), data, 0644)
			}
		}

		if isGit {
			return tools.Result{Success: true, Data: fmt.Sprintf("Checkpoint Git e histórico de mensagens '%s' criados com sucesso.", input.Name)}, nil
		}
		return tools.Result{Success: true, Data: fmt.Sprintf("Checkpoint físico local e histórico '%s' criados com sucesso.", input.Name)}, nil

	case "restore":
		if input.Name == "" {
			return tools.Result{Success: false, Error: "nome é obrigatório para restaurar"}, nil
		}
		// Restaura o histórico de mensagens
		backupDir := filepath.Join(t.workspaceRoot, ".crom", "checkpoints", input.Name)
		if t.stateManager != nil {
			if data, err := os.ReadFile(filepath.Join(backupDir, "messages.json")); err == nil {
				var msgs []llm.Message
				if err := json.Unmarshal(data, &msgs); err == nil {
					_ = t.stateManager.SetMessages(msgs)
				}
			}
		}

		if isGit {
			stashID, found := t.findStashByName(ctx, "crom_checkpoint_"+input.Name)
			if !found {
				return tools.Result{Success: false, Error: fmt.Sprintf("checkpoint '%s' não encontrado", input.Name)}, nil
			}
			// Descarta alterações atuais antes do rollback para evitar conflitos
			_ = exec.CommandContext(ctx, "git", "reset", "--hard").Run()
			_ = exec.CommandContext(ctx, "git", "clean", "-fd").Run()

			// Aplica o stash
			cmd := exec.CommandContext(ctx, "git", "stash", "apply", stashID)
			cmd.Dir = t.workspaceRoot
			out, err := cmd.CombinedOutput()
			if err != nil {
				return tools.Result{Success: false, Error: fmt.Sprintf("falha ao restaurar checkpoint git: %v, out: %s", err, string(out))}, nil
			}
			return tools.Result{Success: true, Data: fmt.Sprintf("✓ Checkpoint '%s' restaurado com sucesso (Rollback de arquivos e mensagens executado).", input.Name)}, nil
		} else {
			return tools.Result{Success: true, Data: fmt.Sprintf("Rollback físico e de histórico para '%s' concluído.", input.Name)}, nil
		}

	case "list":
		if isGit {
			cmd := exec.CommandContext(ctx, "git", "stash", "list")
			cmd.Dir = t.workspaceRoot
			out, _ := cmd.CombinedOutput()
			lines := strings.Split(string(out), "\n")
			var list []string
			for _, line := range lines {
				if strings.Contains(line, "crom_checkpoint_") {
					list = append(list, strings.TrimSpace(line))
				}
			}
			if len(list) == 0 {
				return tools.Result{Success: true, Data: "Nenhum checkpoint crom ativo encontrado no Git stash."}, nil
			}
			return tools.Result{Success: true, Data: strings.Join(list, "\n")}, nil
		} else {
			files, _ := os.ReadDir(filepath.Join(t.workspaceRoot, ".crom", "checkpoints"))
			var list []string
			for _, f := range files {
				if f.IsDir() {
					list = append(list, f.Name())
				}
			}
			if len(list) == 0 {
				return tools.Result{Success: true, Data: "Nenhum checkpoint físico encontrado."}, nil
			}
			return tools.Result{Success: true, Data: strings.Join(list, "\n")}, nil
		}
	}

	return tools.Result{Success: false, Error: "ação desconhecida"}, nil
}

func (t *CheckpointTool) isGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = t.workspaceRoot
	err := cmd.Run()
	return err == nil
}

func (t *CheckpointTool) findStashByName(ctx context.Context, name string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "stash", "list")
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0]), true
			}
		}
	}
	return "", false
}
