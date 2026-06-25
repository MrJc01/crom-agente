package tooling

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/crom/crom-agente/internal/i18n"
)

// FormatToolError gera um erro visualmente formatado para o contexto de mensagens do LLM
func FormatToolError(toolID string, errMsg string) string {
	border := strings.Repeat(i18n.Get("ui.diff_border"), 50)
	return fmt.Sprintf(i18n.Get("ui.tool_error_format"), border, toolID, border, errMsg, border)
}

// RollbackGit desfaz qualquer modificação efetuada nos arquivos locais
func RollbackGit(workspaceDir string) error {
	cmdReset := exec.Command("git", "reset", "--hard", "HEAD")
	cmdReset.Dir = workspaceDir
	if err := cmdReset.Run(); err != nil {
		return err
	}

	cmdClean := exec.Command("git", "clean", "-fd")
	cmdClean.Dir = workspaceDir
	return cmdClean.Run()
}
