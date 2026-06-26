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
	
	hint := ""
	if toolID == "edit_file" && strings.Contains(errMsg, "não foi encontrado no arquivo") {
		hint = "\n\n💡 DICA DE RECUPERAÇÃO: O conteúdo que você tentou substituir não existe exatamente dessa forma no arquivo atual. Talvez a indentação, espaços em branco ou algumas linhas tenham mudado. Por favor, use a ferramenta de busca ou leia o arquivo original e copie o 'target_content' EXATAMENTE igual."
	} else if toolID == "edit_file" && strings.Contains(errMsg, "substituição ambígua") {
		hint = "\n\n💡 DICA DE RECUPERAÇÃO: O 'target_content' fornecido apareceu várias vezes no arquivo. Forneça um bloco maior de código (incluindo as linhas acima/abaixo) para torná-lo único, ou use os campos 'start_line' e 'end_line' para restringir a busca."
	}

	return fmt.Sprintf(i18n.Get("ui.tool_error_format"), border, toolID, border, errMsg+hint, border)
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
