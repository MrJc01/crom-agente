package tui

import (
	"fmt"
	"os"

	"github.com/crom/crom-agente/internal/loop"
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

func (h *tuiEventHandler) OnEvent(event loop.AgentEvent) {
	h.spinner.Stop()
	switch event.Event {
	case "thinking":
		h.spinner.Start(fmt.Sprintf("Pensando (iter %d)", event.Iteration))
	case "tool_call":
		toolName, _ := event.Data["tool"].(string)
		h.spinner.Start(fmt.Sprintf("Executando %s", toolName))
	case "tool_result":
		toolName, _ := event.Data["tool"].(string)
		success, _ := event.Data["success"].(bool)
		if success {
			fmt.Printf("\033[32m  ✅ %s: OK\033[0m\n", toolName)
		} else {
			errMsg, _ := event.Data["error"].(string)
			fmt.Printf("\033[31m  ❌ %s: %s\033[0m\n", toolName, errMsg)
		}
		h.spinner.Start("Processando")
	case "finished":
		reason, _ := event.Data["reason"].(string)
		fmt.Printf("\033[1;32m  🏁 Finalizado (%s)\033[0m\n", reason)
	}
}
