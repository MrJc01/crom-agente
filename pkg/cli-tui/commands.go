package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
)

// HandleSlashCommand processa um comando iniciado por "/" no REPL e retorna a mensagem e se foi tratado.
func HandleSlashCommand(input string, model *TUIModel) (string, bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", false
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/exit", "/quit":
		model.shouldExit = true
		return "Encerrando sessão interativa...", true

	case "/clear":
		// Limpa a tela do terminal
		fmt.Print("\033[H\033[2J")
		return "Tela limpa.", true

	case "/help":
		helpText := `Comandos de Barra (Slash Commands):
  /add <arquivo>    Injeta o conteúdo de um arquivo no próximo prompt
  /session <nome>   Chaveia para a sessão especificada
  /diff             Exibe as alterações git atuais com cores inline
  /cost             Exibe os tokens gastos nesta sessão
  /btw <pergunta>   Faz uma pergunta lateral rápida (sem salvar no histórico de chat)
  /compact          Compacta o histórico de conversa da sessão atual
  /color <cor>      Muda a cor do prompt (red, green, blue, yellow, purple, cyan, orange, pink)
  /clear            Limpa o terminal
  /exit ou /quit    Sai do modo interativo`
		return helpText, true

	case "/session":
		if len(args) == 0 {
			return "Erro: Especifique o nome da sessão. Ex: /session minha-sessao", true
		}
		newSession := args[0]
		model.options.SessionName = newSession
		// Inicializa o novo StateManager
		model.stateManager = state.NewSessionStateManager(model.options.StoragePath, newSession)
		_ = model.stateManager.LoadState()
		return fmt.Sprintf("✓ Chaveado para a sessão '%s' com sucesso.", newSession), true

	case "/add":
		if len(args) == 0 {
			return "Erro: Especifique o arquivo a ser adicionado. Ex: /add main.go", true
		}
		filename := args[0]
		fullPath := filepath.Join(model.options.WorkspacePath, filename)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Sprintf("Erro ao ler arquivo %s: %v", filename, err), true
		}

		// Adiciona aos anexos
		model.attachments[filename] = string(data)
		return fmt.Sprintf("✓ Arquivo [%s] (%d bytes) adicionado com sucesso e será anexado ao seu próximo envio.", filename, len(data)), true

	case "/diff":
		cmd := exec.Command("git", "diff")
		cmd.Dir = model.options.WorkspacePath
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Erro ao rodar git diff (certifique-se de que é um repositório git): %v\n%s", err, string(out)), true
		}
		diffStr := string(out)
		if len(strings.TrimSpace(diffStr)) == 0 {
			return "Nenhuma alteração pendente no repositório git.", true
		}
		return formatDiff(diffStr), true

	case "/cost":
		s := model.stateManager.GetState()
		return fmt.Sprintf("Tokens gastos nesta sessão: \033[1;36m%d\033[0m | Turnos totais: \033[1;36m%d\033[0m", s.TokensGastos, s.TotalTurnos), true

	case "/btw":
		if len(args) == 0 {
			return "Erro: Especifique a pergunta. Ex: /btw qual é a versão do Go?", true
		}
		question := strings.Join(args, " ")
		fmt.Printf("\n\033[1;34m[BTW - Pergunta Lateral]\033[0m %s\n", question)

		// Salva as mensagens originais
		originalMessages := model.stateManager.GetMessages()

		// Executa
		model.spinner.Start("Pensando (BTW)")
		ctx := context.Background()
		err := model.agentLoop.Execute(ctx, question)
		model.spinner.Stop()

		// Restaura histórico original
		_ = model.stateManager.SetMessages(originalMessages)

		if err != nil {
			return fmt.Sprintf("Erro no BTW: %v", err), true
		}
		return "✓ BTW concluído (histórico da sessão preservado).", true

	case "/compact":
		msgs := model.stateManager.GetMessages()
		if len(msgs) > 10 {
			compacted := make([]llm.Message, 0, 10)
			compacted = append(compacted, msgs[0])
			compacted = append(compacted, msgs[len(msgs)-9:]...)
			_ = model.stateManager.SetMessages(compacted)
			return fmt.Sprintf("✓ Histórico compactado de %d para %d mensagens.", len(msgs), len(compacted)), true
		}
		return "Histórico da sessão curto demais para compactação.", true

	case "/color":
		if len(args) == 0 {
			return "Erro: Especifique uma cor (red, green, blue, yellow, purple, cyan, orange, pink)", true
		}
		color := strings.ToLower(args[0])
		var ansiCode string
		switch color {
		case "red":
			ansiCode = "\033[1;31m"
		case "green":
			ansiCode = "\033[1;32m"
		case "yellow":
			ansiCode = "\033[1;33m"
		case "blue":
			ansiCode = "\033[1;34m"
		case "purple":
			ansiCode = "\033[1;35m"
		case "cyan":
			ansiCode = "\033[1;36m"
		case "orange":
			ansiCode = "\033[38;5;208m"
		case "pink":
			ansiCode = "\033[38;5;205m"
		default:
			return fmt.Sprintf("Cor desconhecida: %s. Escolha entre red, green, blue, yellow, purple, cyan, orange, pink.", color), true
		}
		model.promptColor = ansiCode
		return fmt.Sprintf("Cor do prompt atualizada para '%s'.", color), true

	default:
		return fmt.Sprintf("Comando desconhecido: %s. Digite /help para a ajuda.", cmd), true
	}
}

// formatDiff estiliza a saída do git diff com cores vermelha e verde nativas de terminal
func formatDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var formatted []string
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			formatted = append(formatted, "\033[32m"+line+"\033[0m") // verde
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			formatted = append(formatted, "\033[31m"+line+"\033[0m") // vermelho
		} else if strings.HasPrefix(line, "@@") {
			formatted = append(formatted, "\033[36m"+line+"\033[0m") // ciano
		} else if strings.HasPrefix(line, "diff") || strings.HasPrefix(line, "index") {
			formatted = append(formatted, "\033[1;30m"+line+"\033[0m") // cinza escuro
		} else {
			formatted = append(formatted, line)
		}
	}
	return strings.Join(formatted, "\n")
}
