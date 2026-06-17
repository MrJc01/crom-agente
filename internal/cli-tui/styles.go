package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Cores da paleta (Estilo moderno / Neon Dark)
	colorIndigo = lipgloss.Color("#6366f1")
	colorCyan   = lipgloss.Color("#06b6d4")
	colorGreen  = lipgloss.Color("#10b981")
	colorGray   = lipgloss.Color("#64748b")
	colorDarkBg = lipgloss.Color("#1e293b")
	colorLight  = lipgloss.Color("#f8fafc")
	colorRed    = lipgloss.Color("#ef4444")
	colorOrange = lipgloss.Color("#f97316")

	// Estilos do layout principal
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorLight).
			Background(colorIndigo).
			Padding(0, 2).
			MarginBottom(1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	ViewportBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorIndigo).
			Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
			Background(colorDarkBg).
			Foreground(colorGray).
			Padding(0, 1).
			Height(1)

	StatusActiveStyle = lipgloss.NewStyle().
				Background(colorCyan).
				Foreground(colorDarkBg).
				Bold(true).
				Padding(0, 1)

	PromptStyle = lipgloss.NewStyle().
			Foreground(colorCyan).
			Bold(true)

	// Estilos de balões / blocos de mensagens
	UserMsgStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colorGreen).
			PaddingLeft(1).
			MarginBottom(1)

	AssistantMsgStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorCyan).
				PaddingLeft(1).
				MarginBottom(1)

	SystemMsgStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Italic(true).
			MarginBottom(1)

	ToolMsgStyle = lipgloss.NewStyle().
			Foreground(colorCyan).
			Italic(true).
			MarginBottom(1)

	ErrorMsgStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true).
			MarginBottom(1)

	// HITL / Confirmações interativas
	DialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOrange).
			Padding(1, 2).
			Align(lipgloss.Center)

	DialogButtonStyle = lipgloss.NewStyle().
				Foreground(colorLight).
				Background(colorGray).
				Padding(0, 2).
				Margin(0, 1)

	DialogButtonActiveStyle = lipgloss.NewStyle().
				Foreground(colorLight).
				Background(colorOrange).
				Bold(true).
				Padding(0, 2).
				Margin(0, 1)
)
