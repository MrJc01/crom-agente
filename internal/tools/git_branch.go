package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ---------- git_branch ----------

// GitBranchTool gerencia branches Git: listar, criar e trocar
type GitBranchTool struct {
	workspaceRoot string
}

// NewGitBranchTool cria a ferramenta git_branch
func NewGitBranchTool(workspaceRoot string) *GitBranchTool {
	return &GitBranchTool{workspaceRoot: workspaceRoot}
}

func (t *GitBranchTool) ID() string { return "git_branch" }

func (t *GitBranchTool) Description() string {
	return `Gerencia branches Git. Ações disponíveis:
- "list": Lista todas as branches locais (marca a atual com *)
- "create": Cria uma nova branch e faz checkout para ela
- "checkout": Troca para uma branch existente
Flags perigosas como --force são bloqueadas por segurança.`
}

func (t *GitBranchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "create", "checkout"],
				"description": "Ação a ser executada: list, create ou checkout"
			},
			"name": {
				"type": "string",
				"description": "Nome da branch (obrigatório para create e checkout)"
			}
		},
		"required": ["action"]
	}`)
}

// RequiresApproval — troca e criação de branches exige aprovação
func (t *GitBranchTool) RequiresApproval() bool { return true }

// blockedBranchFlags são flags que jamais devem ser permitidas
var blockedBranchFlags = []string{
	"--force", "-f",
	"--force-with-lease",
	"--delete", "-D", "-d",
}

func (t *GitBranchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if err := validateGitRepo(t.workspaceRoot); err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	var input struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	// Validação de segurança: bloquear flags perigosas no nome
	name := strings.TrimSpace(input.Name)
	for _, flag := range blockedBranchFlags {
		if strings.Contains(name, flag) {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("segurança: flag '%s' é bloqueada para operações de branch", flag),
			}, nil
		}
	}

	switch input.Action {
	case "list":
		return t.listBranches(ctx)
	case "create":
		if name == "" {
			return Result{Success: false, Error: "nome da branch é obrigatório para 'create'"}, nil
		}
		return t.createBranch(ctx, name)
	case "checkout":
		if name == "" {
			return Result{Success: false, Error: "nome da branch é obrigatório para 'checkout'"}, nil
		}
		return t.checkoutBranch(ctx, name)
	default:
		return Result{Success: false, Error: fmt.Sprintf("ação desconhecida: %q. Use 'list', 'create' ou 'checkout'", input.Action)}, nil
	}
}

func (t *GitBranchTool) listBranches(ctx context.Context) (Result, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--list", "-v")
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao listar branches: %s (%s)", err, string(out))}, nil
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		output = "(nenhuma branch encontrada)"
	}
	return Result{Success: true, Data: output}, nil
}

func (t *GitBranchTool) createBranch(ctx context.Context, name string) (Result, error) {
	// Validar nome de branch
	if err := validateBranchName(name); err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao criar branch: %s (%s)", err, string(out))}, nil
	}

	return Result{
		Success: true,
		Data:    fmt.Sprintf("Branch '%s' criada e ativada.\n%s", name, strings.TrimSpace(string(out))),
	}, nil
}

func (t *GitBranchTool) checkoutBranch(ctx context.Context, name string) (Result, error) {
	cmd := exec.CommandContext(ctx, "git", "checkout", name)
	cmd.Dir = t.workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao trocar para branch '%s': %s (%s)", name, err, string(out))}, nil
	}

	return Result{
		Success: true,
		Data:    fmt.Sprintf("Branch ativa: '%s'\n%s", name, strings.TrimSpace(string(out))),
	}, nil
}

// validateBranchName valida se o nome de branch é seguro
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de branch não pode ser vazio")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("nome de branch não pode começar com '-'")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("nome de branch não pode conter '..'")
	}
	if strings.ContainsAny(name, " ~^:?*[\\") {
		return fmt.Errorf("nome de branch contém caracteres inválidos")
	}
	return nil
}
