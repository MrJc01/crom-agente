package tui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// tuiEventHandler escuta os eventos do loop ReAct e os imprime diretamente no stdout, integrando com o spinner
type tuiEventHandler struct {
	spinner *InlineSpinner
}

func (h *tuiEventHandler) OnStatusChange(status string) {
	h.spinner.Update(status)
}

func (h *tuiEventHandler) OnMessage(role string, content string) {
	// Para o spinner antes de imprimir para evitar entrelaçamento de texto
	h.spinner.Stop()

	switch role {
	case "assistant":
		// Obtém a largura do terminal para formatação ideal do Glamour
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}
		rendered := RenderMarkdown(content, width)
		fmt.Printf("\n\033[1;36m🤖 Agent:\033[0m\n%s\n\n", rendered)
	case "system":
		fmt.Printf("\033[33m⚙️ System:\033[0m %s\n", content)
	case "user":
		fmt.Printf("\033[32m👤 User:\033[0m %s\n", content)
	case "tool":
		fmt.Printf("\033[35m🛠️ Tool:\033[0m %s\n", content)
	}

	// Reinicia o spinner
	h.spinner.Start("Processando")
}
