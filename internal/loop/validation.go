package loop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectStack identifica a tecnologia principal do projeto
func (al *AgenticLoop) detectStack(workspaceDir string) string {
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

// loadLocalRules lê arquivos de regras customizadas do workspace
func (al *AgenticLoop) loadLocalRules(workspaceDir string) string {
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

// autoValidate executa validações de qualidade específicas da stack técnica
func (al *AgenticLoop) autoValidate(ctx context.Context, workspaceDir string) (bool, string) {
	stack := al.detectStack(workspaceDir)
	if strings.Contains(stack, "Go (golang)") {
		al.handler.OnStatusChange("thinking")
		al.handler.OnMessage("system", "Executando auto-validação de sintaxe ('go vet')...")

		cmdVet := exec.CommandContext(ctx, "go", "vet", "./...")
		cmdVet.Dir = workspaceDir
		out, errVet := cmdVet.CombinedOutput()

		if errVet != nil {
			errMsg := fmt.Sprintf("[AUTO-VALIDATION FAILURE] O linter ('go vet') detectou erros de sintaxe:\n%s\nCorrija estes problemas de código antes de finalizar.", string(out))
			return false, errMsg
		}

		// Executa go fmt de forma transparente
		cmdFmt := exec.CommandContext(ctx, "go", "fmt", "./...")
		cmdFmt.Dir = workspaceDir
		_ = cmdFmt.Run()
	}
	return true, ""
}
