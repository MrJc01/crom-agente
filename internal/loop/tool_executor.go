package loop

import (
	"fmt"
	"os/exec"
	"strings"
)

// formatToolError gera um erro visualmente formatado para o contexto de mensagens do LLM
func formatToolError(toolID string, errMsg string) string {
	border := strings.Repeat("═", 50)
	return fmt.Sprintf("\n╔%s╗\n║ ERROR: Tool \"%s\" failed\n╟%s╢\n║ %s\n╚%s╝\n",
		border, toolID, border, errMsg, border)
}

// rollbackGit desfaz qualquer modificação efetuada nos arquivos locais
func rollbackGit(workspaceDir string) error {
	cmdReset := exec.Command("git", "reset", "--hard", "HEAD")
	cmdReset.Dir = workspaceDir
	if err := cmdReset.Run(); err != nil {
		return err
	}
	
	cmdClean := exec.Command("git", "clean", "-fd")
	cmdClean.Dir = workspaceDir
	return cmdClean.Run()
}
