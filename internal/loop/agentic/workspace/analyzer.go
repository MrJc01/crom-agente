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

// CheckWorkspaceQuota verifica se o tamanho total do diretório excedeu o limite (ex: 200MB).
func CheckWorkspaceQuota(workspaceDir string, maxBytes int64) (bool, int64, error) {
	if workspaceDir == "" {
		return false, 0, nil
	}
	var size int64
	err := filepath.Walk(workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			// Ignorar pastas que NÃO são conteúdo do usuário: estado do próprio
			// agente (.crom), cache/import do Godot (.godot/.import), controle de
			// versão (.git) e dependências (node_modules). Sem isso um projeto
			// Godot com o plugin CromAI (que traz ~200MB de binários em
			// addons/crom_ai/bin) estoura a quota e o loop é abortado sem motivo.
			if name == ".crom" || name == ".godot" || name == ".import" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			// A pasta de binários do próprio plugin CromAI (crom-agente + godot-mcp
			// para todos os alvos) não conta como conteúdo do projeto.
			if strings.HasSuffix(path, filepath.Join("addons", "crom_ai", "bin")) {
				return filepath.SkipDir
			}
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	
	if size > maxBytes {
		return true, size, err
	}
	return false, size, err
}

// GenerateDirectoryTree gera uma árvore de arquivos rudimentar ignorando diretórios ocultos ou muito grandes (.git, node_modules)
func GenerateDirectoryTree(workspaceDir string, maxDepth int) string {
	if workspaceDir == "" {
		return ""
	}
	var sb strings.Builder
	var walk func(dir string, prefix string, depth int)
	walk = func(dir string, prefix string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		
		// Filtrar entradas indesejadas primeiro
		var validEntries []os.DirEntry
		for _, e := range entries {
			name := e.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".crom" || name == ".venv" || name == "__pycache__" {
				continue
			}
			validEntries = append(validEntries, e)
		}

		for i, entry := range validEntries {
			name := entry.Name()
			isLast := i == len(validEntries)-1
			connector := "├── "
			if isLast {
				connector = "└── "
			}
			sb.WriteString(prefix + connector + name + "\n")
			if entry.IsDir() {
				nextPrefix := prefix + "│   "
				if isLast {
					nextPrefix = prefix + "    "
				}
				walk(filepath.Join(dir, name), nextPrefix, depth+1)
			}
		}
	}
	sb.WriteString(filepath.Base(workspaceDir) + "\n")
	walk(workspaceDir, "", 1)
	return sb.String()
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
