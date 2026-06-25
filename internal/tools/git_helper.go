package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ValidateGitRepo verifica se o diretório é um repositório git válido
func ValidateGitRepo(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = gitDir // evita warning
		return fmt.Errorf("o diretório '%s' não é um repositório Git válido: %s", dir, strings.TrimSpace(string(out)))
	}
	return nil
}
