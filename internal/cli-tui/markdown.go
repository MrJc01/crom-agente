package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// RenderMarkdown formata texto em markdown para exibição no terminal usando Glamour.
func RenderMarkdown(content string, width int) string {
	// Cria um renderizador Glamour com tema dark/light automático
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width - 4),
	)
	if err != nil {
		// Fallback caso falhe
		return content
	}

	out, err := r.Render(content)
	if err != nil {
		return content
	}

	// Remove quebras de linha extras inúteis do glamour
	return strings.TrimSpace(out)
}
