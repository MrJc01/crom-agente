package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/i18n"
)

// MessageHandler interface para desacoplar a emissão de logs
type MessageHandler interface {
	OnMessage(role, msg string)
	OnStatusChange(status string)
}

// DetectStack identifica a tecnologia principal do projeto
func DetectStack(workspaceDir string) string {
	if workspaceDir == "" {
		return "Desconhecida"
	}
	var stacks []string
	if _, err := os.Stat(filepath.Join(workspaceDir, "go.mod")); err == nil {
		stacks = append(stacks, "Go (golang)")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "package.json")); err == nil {
		stacks = append(stacks, "Node.js (JavaScript/TypeScript)")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "Cargo.toml")); err == nil {
		stacks = append(stacks, "Rust (Cargo)")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "requirements.txt")); err == nil {
		stacks = append(stacks, "Python (pip)")
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "pyproject.toml")); err == nil {
		stacks = append(stacks, "Python (Poetry/Pipenv)")
	}
	if len(stacks) == 0 {
		return "Desconhecida"
	}
	return strings.Join(stacks, ", ")
}

// LoadLocalRules lê arquivos de regras customizadas do workspace
func LoadLocalRules(workspaceDir string) string {
	if workspaceDir == "" {
		return ""
	}
	var rules []string
	// Lê da raiz do workspace
	for _, ruleFile := range []string{".cromrules", ".voidrules"} {
		path := filepath.Join(workspaceDir, ruleFile)
		if data, err := os.ReadFile(path); err == nil {
			rules = append(rules, fmt.Sprintf("=== Regras de %s ===\n%s", ruleFile, string(data)))
		}
	}
	// Lê também do subdiretório .crom/ se existir
	for _, ruleFile := range []string{".cromrules", ".voidrules"} {
		path := filepath.Join(workspaceDir, ".crom", ruleFile)
		if data, err := os.ReadFile(path); err == nil {
			rules = append(rules, fmt.Sprintf("=== Regras de .crom/%s ===\n%s", ruleFile, string(data)))
		}
	}
	return strings.Join(rules, "\n\n")
}

// AutoValidate executa validações de qualidade específicas da stack técnica
func AutoValidate(ctx context.Context, workspaceDir string, handler MessageHandler) (bool, string) {
	stack := DetectStack(workspaceDir)
	if strings.Contains(stack, "Go (golang)") {
		if handler != nil {
			handler.OnStatusChange(i18n.Get("system.thinking"))
			handler.OnMessage("system", i18n.Get("system.auto_validation_start"))
		}

		cmdVet := exec.CommandContext(ctx, "go", "vet", "./...")
		cmdVet.Dir = workspaceDir
		out, errVet := cmdVet.CombinedOutput()

		if errVet != nil {
			errMsg := i18n.Get("system.auto_validation_failure", string(out))
			return false, errMsg
		}

		// Executa go fmt de forma transparente
		cmdFmt := exec.CommandContext(ctx, "go", "fmt", "./...")
		cmdFmt.Dir = workspaceDir
		_ = cmdFmt.Run()
	}
	return true, ""
}
