package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/security"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// EventHandler permite que o chamador receba notificações do loop (CLI, SDK, etc.)
type EventHandler interface {
	OnStatusChange(status string)
	OnMessage(role string, content string)
}

// noopHandler é um handler vazio usado quando nenhum handler é fornecido
type noopHandler struct{}

func (n noopHandler) OnStatusChange(string) {}
func (n noopHandler) OnMessage(string, string) {}

// AgenticLoop é o motor de execução do agente seguindo o padrão ReAct
type AgenticLoop struct {
	provider          llm.Provider
	tools             map[string]tools.Tool
	stateManager      *state.StateManager
	handler           EventHandler
	config            *config.ResolvedConfig
	permissionManager interface {
		Authorize(action, target string) (bool, error)
	}
}

func (al *AgenticLoop) SetPermissionManager(pm interface {
	Authorize(action, target string) (bool, error)
}) {
	al.permissionManager = pm
}


// New cria uma nova instância do AgenticLoop
func New(provider llm.Provider, sm *state.StateManager, handler EventHandler, cfg ...*config.ResolvedConfig) *AgenticLoop {
	if handler == nil {
		handler = noopHandler{}
	}

	var resolvedCfg *config.ResolvedConfig
	if len(cfg) > 0 && cfg[0] != nil {
		resolvedCfg = cfg[0]
	} else {
		// Defaults hardcoded para backward compatibility
		resolvedCfg = &config.ResolvedConfig{
			MaxIterations:      15,
			MaxConsecutiveFail: 3,
			ToolTimeoutSeconds: 30,
			MaxMessageHistory:  40,
			AutoVerify:         true,
			PermissionMode:     "scoped",
		}
	}

	return &AgenticLoop{
		provider:     provider,
		tools:        make(map[string]tools.Tool),
		stateManager: sm,
		handler:      handler,
		config:       resolvedCfg,
	}
}

// RegisterTool registra uma ferramenta disponível para o agente
func (al *AgenticLoop) RegisterTool(t tools.Tool) {
	al.tools[t.ID()] = t
}

// GetTools retorna a lista de ferramentas registradas
func (al *AgenticLoop) GetTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(al.tools))
	for _, t := range al.tools {
		result = append(result, t)
	}
	return result
}

// Execute roda o loop ReAct completo para a tarefa dada
func (al *AgenticLoop) Execute(ctx context.Context, intent string) error {
	al.handler.OnStatusChange("thinking")
	if al.stateManager != nil {
		_ = al.stateManager.SetActiveTask(intent)
	}

	saveMsgs := func(msgs []llm.Message) {
		if al.stateManager != nil {
			_ = al.stateManager.SetMessages(msgs)
		}
	}

	var messages []llm.Message
	if al.stateManager != nil {
		messages = al.stateManager.GetMessages()
	}

	if len(messages) == 0 {
		messages = []llm.Message{
			{Role: "user", Content: intent},
		}
	} else {
		messages = append(messages, llm.Message{Role: "user", Content: intent})
	}
	saveMsgs(messages)

	workspaceDir := ""
	if al.stateManager != nil {
		workspaceDir = filepath.Dir(filepath.Dir(al.stateManager.FilePath()))
	}

	// Primeira iteração da sessão: injetar regras locais, stack e instruções de planejamento
	if len(messages) <= 2 && workspaceDir != "" {
		// 1. Detectar stack técnica
		stack := al.detectStack(workspaceDir)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: fmt.Sprintf("[SYSTEM STACK DETECTED] A stack técnica deste projeto foi identificada como: %s. Priorize comandos e validações desta stack.", stack),
		})

		// 2. Carregar regras locais
		if localRules := al.loadLocalRules(workspaceDir); localRules != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("[SYSTEM LOCAL RULES] Respeite estritamente as seguintes regras do workspace:\n\n%s", localRules),
			})
		}

		// 3. Forçar Planejamento
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM PLANNING REQUIREMENT] Na sua primeira resposta, antes de executar qualquer ferramenta de gravação ou terminal, você deve obrigatoriamente descrever e listar um plano de execução subdividido em sub-tarefas claras.",
		})
		saveMsgs(messages)
	}

	hasExecutedTool := false
	hasVerified := false
	consecutiveFailures := 0

	for i := 0; i < al.config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			al.handler.OnMessage("system", "Loop cancelado pelo contexto.")
			return ctx.Err()
		default:
		}

		al.handler.OnStatusChange("thinking")

		// Detectar loops repetitivos
		if detectRepetitiveLoop(messages) {
			al.handler.OnMessage("system", "⚠️ Loop repetitivo detectado. Injetando correção.")
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "⚠️ [REPETITIVE_LOOP_WARNING] Você está repetindo ações anteriores. Mude sua estratégia imediatamente.",
			})
			saveMsgs(messages)
		}

		// Construir definições de ferramentas para o LLM
		opts := al.buildRequestOptions()

		// Injetar plano de trabalho atualizado de forma dinâmica no contexto
		runMessages := messages
		if planCtx := SyncPlanToContext(al.stateManager); planCtx != "" {
			// Cria uma cópia rasa para injetar a mensagem do sistema sem corromper o histórico salvo
			runMessages = make([]llm.Message, len(messages))
			copy(runMessages, messages)
			runMessages = append(runMessages, llm.Message{
				Role:    "system",
				Content: planCtx,
			})
		}

		// Chamar o LLM
		resp, err := al.provider.SendMessages(ctx, compactMessages(runMessages), opts)
		if err != nil {
			al.handler.OnMessage("system", fmt.Sprintf("Erro na requisição ao LLM: %s", err.Error()))
			return fmt.Errorf("falha na chamada ao LLM: %w", err)
		}

		// Registrar tokens
		if al.stateManager != nil && resp.Usage.TotalTokens > 0 {
			_ = al.stateManager.RecordTokens(resp.Usage.TotalTokens)
		}

		msg := resp.Message
		messages = append(messages, msg)
		saveMsgs(messages)

		// Atualiza o plano a partir do conteúdo textual da mensagem do assistente
		UpdatePlannerFromMessage(al.stateManager, msg.Content)

		// Emitir texto do assistente
		if msg.Content != "" {
			al.handler.OnMessage("assistant", msg.Content)
		}

		// Sem tool calls: verificar se devemos encerrar ou entrar em fase de verificação
		if len(msg.ToolCalls) == 0 {
			// Resposta vazia: auto-correção
			if msg.Content == "" {
				al.handler.OnMessage("system", "Auto-correção: resposta vazia recebida.")
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "[SYSTEM AUTO-CORRECTION] Sua resposta estava vazia. Forneça uma resposta textual ou execute uma ferramenta.",
				})
				saveMsgs(messages)
				consecutiveFailures++
				if consecutiveFailures >= al.config.MaxConsecutiveFail {
					return fmt.Errorf("abortando: %d falhas consecutivas", consecutiveFailures)
				}
				continue
			}

			// Detectar tool call leaking no texto
			if !hasExecutedTool && looksLikeLeakedToolCall(msg.Content) {
				al.handler.OnMessage("system", "Auto-correção: tool call detectada no texto.")
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "[SYSTEM AUTO-CORRECTION] Você escreveu texto parecido com uma chamada de ferramenta. Use a API nativa de function calling.",
				})
				saveMsgs(messages)
				continue
			}

			// Fase de verificação
			if hasExecutedTool && !hasVerified {
				hasVerified = true
				al.handler.OnMessage("system", "Entrando na fase de verificação.")
				messages = append(messages, llm.Message{
					Role: "system",
					Content: `Fase de Verificação:
Analise as alterações que você fez. Tem certeza absoluta de que tudo está correto?
Se houver testes ou comandos de compilação, execute-os agora para validar.
Se encontrar erros, corrija-os antes de finalizar.`,
				})
				saveMsgs(messages)
				continue
			}

			// Fase de auto-validação lógica antes de encerrar
			if al.config.AutoVerify && !hasVerified && workspaceDir != "" {
				stack := al.detectStack(workspaceDir)
				if strings.Contains(stack, "Go (golang)") {
					al.handler.OnStatusChange("thinking")
					al.handler.OnMessage("system", "Executando auto-validação de sintaxe ('go vet')...")

					cmdVet := exec.CommandContext(ctx, "go", "vet", "./...")
					cmdVet.Dir = workspaceDir
					out, errVet := cmdVet.CombinedOutput()

					if errVet != nil {
						hasVerified = true // Para impedir loop infinito
						errMsg := fmt.Sprintf("[AUTO-VALIDATION FAILURE] O linter ('go vet') detectou erros de sintaxe:\n%s\nCorrija estes problemas de código antes de finalizar.", string(out))
						al.handler.OnMessage("system", "Falha na auto-validação estática.")
						messages = append(messages, llm.Message{
							Role:    "user",
							Content: errMsg,
						})
						saveMsgs(messages)
						continue
					}

					// go fmt check
					cmdFmt := exec.CommandContext(ctx, "go", "fmt", "./...")
					cmdFmt.Dir = workspaceDir
					_ = cmdFmt.Run() // Executa o fmt para auto-corrigir estilo
				}
			}

			// Loop encerrado normalmente
			if al.stateManager != nil {
				_ = al.stateManager.SetStatus("idle")
				_ = al.stateManager.AddLog(fmt.Sprintf("Tarefa concluída: %s", intent))
			}
			al.handler.OnStatusChange("finished")
			return nil
		}

		// Executar tool calls
		iterationHasFailure := false
		for _, tc := range msg.ToolCalls {
			hasExecutedTool = true
			toolID := tc.Function.Name

			tool, exists := al.tools[toolID]
			if !exists {
				errMsg := fmt.Sprintf("Ferramenta '%s' não encontrada.", toolID)
				al.handler.OnMessage("system", errMsg)
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID,
					Content: formatToolError(toolID, errMsg),
				})
				iterationHasFailure = true
				continue
			}

			if toolID == "read_file" || toolID == "list_dir" || toolID == "grep_search" || toolID == "tree" || toolID == "git_status" || toolID == "git_log" || toolID == "git_diff" {
				al.handler.OnStatusChange("reading")
			} else if toolID == "write_file" || toolID == "diff_replace" || toolID == "rename_file" || toolID == "delete_file" || toolID == "git_add" || toolID == "git_commit" || toolID == "git_branch" {
				al.handler.OnStatusChange("writing")
			} else {
				al.handler.OnStatusChange("executing_tool")
			}
			al.handler.OnMessage("system", fmt.Sprintf("Executando ferramenta: %s", toolID))

			if al.stateManager != nil {
				_ = al.stateManager.AddLog(fmt.Sprintf("[tool_start] %s", toolID))
			}

			// Verificar autorização do PermissionManager
			if al.permissionManager != nil {
				var target string
				if toolID == "terminal_command" {
					var argsCmd struct {
						Command string `json:"command"`
					}
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsCmd)
					target = argsCmd.Command
				} else if toolID == "write_file" || toolID == "read_file" {
					var argsPath struct {
						Path string `json:"path"`
					}
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsPath)
					target = argsPath.Path
				} else if toolID == "diff_replace" {
					var argsPath struct {
						Path string `json:"path"`
					}
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &argsPath)
					target = argsPath.Path
				}

				// DiffZones: Renderizar diff colorido antes de pedir autorização para ferramentas de escrita
				if toolID == "write_file" || toolID == "diff_replace" {
					al.renderDiffZone(tc.Function.Arguments, toolID, workspaceDir)
				}

				approved, authErr := al.permissionManager.Authorize(toolID, target)
				if authErr != nil || !approved {
					errMsg := fmt.Sprintf("Ação '%s' rejeitada pelo usuário ou pelas políticas de segurança.", toolID)
					al.handler.OnMessage("system", errMsg)
					messages = append(messages, llm.Message{
						Role: "tool", ToolCallID: tc.ID, Name: toolID,
						Content: formatToolError(toolID, errMsg),
					})
					iterationHasFailure = true
					continue
				}
			}


			// Executar com timeout
			toolCtx, cancel := context.WithTimeout(ctx, time.Duration(al.config.ToolTimeoutSeconds)*time.Second)
			result, execErr := tool.Execute(toolCtx, json.RawMessage(tc.Function.Arguments))
			cancel()

			if execErr != nil {
				errContent := formatToolError(toolID, execErr.Error())
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", fmt.Sprintf("Exceção na ferramenta %s: %s", toolID, execErr.Error()))
				iterationHasFailure = true
				continue
			}

			if result.Success {
				if strings.HasPrefix(result.Data, "image:base64:") {
					// 1. Adiciona a resposta da ferramenta como texto simples para validação do esquema da API
					messages = append(messages, llm.Message{
						Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: "✓ Captura de tela realizada com sucesso.",
					})
					// 2. Adiciona uma mensagem de usuário auxiliar contendo o payload da imagem para o VLM processar
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: result.Data,
					})
				} else {
					messages = append(messages, llm.Message{
						Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: result.Data,
					})
				}
				al.handler.OnMessage("system", fmt.Sprintf("Ferramenta %s executada com sucesso.", toolID))

			} else {
				errContent := formatToolError(toolID, result.Error)
				if toolID == "terminal_command" {
					errContent = FormatContextualError(errContent)
				}
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", fmt.Sprintf("Erro na ferramenta %s: %s", toolID, result.Error))
				iterationHasFailure = true
			}
		}
		saveMsgs(messages)

		if iterationHasFailure {
			consecutiveFailures++
			if consecutiveFailures >= al.config.MaxConsecutiveFail {
				al.handler.OnMessage("system", fmt.Sprintf("Abortando: %d falhas consecutivas.", al.config.MaxConsecutiveFail))
				return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
			}
		} else {
			consecutiveFailures = 0
		}
	}

	al.handler.OnMessage("system", "Limite de iterações atingido.")
	al.handler.OnStatusChange("idle")
	return fmt.Errorf("limite de %d iterações atingido", al.config.MaxIterations)
}

// buildRequestOptions constrói as definições de ferramentas para enviar ao LLM
func (al *AgenticLoop) buildRequestOptions() llm.RequestOptions {
	if len(al.tools) == 0 {
		return llm.RequestOptions{}
	}

	defs := make([]llm.ToolDefinition, 0, len(al.tools))
	for _, t := range al.tools {
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.ID(),
				Description: t.Description(),
				Parameters:  t.ParametersSchema(),
			},
		})
	}

	return llm.RequestOptions{
		Tools:      defs,
		ToolChoice: "auto",
	}
}

// detectRepetitiveLoop verifica se as últimas mensagens do assistant indicam um loop repetitivo.
// Compara assinaturas que incluem tanto texto quanto tool calls.
func detectRepetitiveLoop(messages []llm.Message) bool {
	if len(messages) < 4 {
		return false
	}

	// Gera assinatura das duas últimas mensagens do assistant
	var lastTwo []string
	for i := len(messages) - 1; i >= 0 && len(lastTwo) < 2; i-- {
		if messages[i].Role == "assistant" {
			lastTwo = append(lastTwo, assistantSignature(messages[i]))
		}
	}

	if len(lastTwo) == 2 && lastTwo[0] != "" && lastTwo[0] == lastTwo[1] {
		return true
	}

	return false
}

// assistantSignature gera uma string de assinatura para uma mensagem do assistant,
// combinando texto e tool calls para permitir comparação de repetição.
func assistantSignature(msg llm.Message) string {
	sig := msg.Content
	for _, tc := range msg.ToolCalls {
		sig += "|" + tc.Function.Name + ":" + tc.Function.Arguments
	}
	return sig
}

// looksLikeLeakedToolCall detecta se o LLM tentou gerar uma tool call no texto
func looksLikeLeakedToolCall(content string) bool {
	indicators := []string{`"name":`, `<call>`, `write_file`, `read_file`, `terminal_command`}
	for _, indicator := range indicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}
	return false
}

// compactMessages aplica compactação simples removendo mensagens antigas se houver muitas
func compactMessages(messages []llm.Message) []llm.Message {
	const maxMessages = 40
	if len(messages) <= maxMessages {
		return messages
	}

	// Preserva a primeira mensagem (user intent) e as últimas N
	keepFromEnd := maxMessages - 1
	compacted := make([]llm.Message, 0, maxMessages)
	compacted = append(compacted, messages[0])
	compacted = append(compacted, messages[len(messages)-keepFromEnd:]...)

	log.Printf("[AgenticLoop] Compactou histórico de %d para %d mensagens", len(messages), len(compacted))
	return compacted
}

// formatToolError formata erros de ferramenta em estilo CLI
func formatToolError(toolID string, errMsg string) string {
	border := strings.Repeat("═", 50)
	return fmt.Sprintf("\n╔%s╗\n║ ERROR: Tool \"%s\" failed\n╟%s╢\n║ %s\n╚%s╝\n",
		border, toolID, border, errMsg, border)
}

// RegisterSpawnSubagentTool registra a ferramenta spawn_subagent no loop ativo
func (al *AgenticLoop) RegisterSpawnSubagentTool() {
	spawner := func(ctx context.Context, task string) (tools.Result, error) {
		subagentID := fmt.Sprintf("subagent-%d", time.Now().UnixNano())
		storageDir := filepath.Join(filepath.Dir(al.stateManager.FilePath()), "agents", subagentID)

		subSM := state.NewStateManager(storageDir)
		_ = subSM.LoadState()

		// Herdando configurações
		subAL := New(al.provider, subSM, al.handler, al.config)
		subAL.permissionManager = al.permissionManager
		for _, t := range al.tools {
			subAL.RegisterTool(t)
		}

		// Executa
		err := subAL.Execute(ctx, task)
		if err != nil {
			// Subagente falhou: efetua rollback baseado em Git
			workspaceDir := filepath.Dir(filepath.Dir(al.stateManager.FilePath()))
			_ = rollbackGit(workspaceDir)

			return tools.Result{
				Success: false,
				Error:   fmt.Sprintf("subagente falhou: %s. Rollback automático executado.", err.Error()),
			}, nil
		}

		return tools.Result{
			Success: true,
			Data:    "Subagente concluiu a tarefa com sucesso.",
		}, nil
	}

	al.RegisterTool(tools.NewSpawnSubagentTool(spawner))
}

func rollbackGit(workspaceDir string) error {
	cmd := exec.Command("git", "reset", "--hard", "HEAD")
	cmd.Dir = workspaceDir
	return cmd.Run()
}

func (al *AgenticLoop) detectStack(workspaceDir string) string {
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

func (al *AgenticLoop) loadLocalRules(workspaceDir string) string {
	if workspaceDir == "" {
		return ""
	}
	var rules []string
	for _, ruleFile := range []string{".cromrules", ".voidrules"} {
		path := filepath.Join(workspaceDir, ruleFile)
		if data, err := os.ReadFile(path); err == nil {
			rules = append(rules, fmt.Sprintf("=== Regras de %s ===\n%s", ruleFile, string(data)))
		}
	}
	return strings.Join(rules, "\n\n")
}

// renderDiffZone renderiza a visualização colorida de diffs no terminal antes de solicitar autorização HITL.
// Para write_file, compara o conteúdo atual do arquivo com o novo conteúdo proposto.
// Para diff_replace, compara o target_content com o replacement_content no contexto do arquivo.
func (al *AgenticLoop) renderDiffZone(rawArgs string, toolID string, workspaceDir string) {
	switch toolID {
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return
		}

		// Resolver caminho do arquivo
		filePath := args.Path
		if !filepath.IsAbs(filePath) && workspaceDir != "" {
			filePath = filepath.Join(workspaceDir, filePath)
		}

		// Ler conteúdo atual do arquivo (pode não existir se for arquivo novo)
		oldContent := ""
		if data, err := os.ReadFile(filePath); err == nil {
			oldContent = string(data)
		}

		diffOutput := security.RenderDiff(args.Path, oldContent, args.Content)
		al.handler.OnMessage("system", diffOutput)

	case "diff_replace":
		var args struct {
			Path               string `json:"path"`
			TargetContent      string `json:"target_content"`
			ReplacementContent string `json:"replacement_content"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return
		}

		// Para diff_replace, mostramos o diff do trecho alterado
		diffOutput := security.RenderDiff(args.Path, args.TargetContent, args.ReplacementContent)
		al.handler.OnMessage("system", diffOutput)
	}
}


