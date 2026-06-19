package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

// EventHandler permite que o chamador receba notificações do loop (CLI, SDK, etc.)
type EventHandler interface {
	OnStatusChange(status string)
	OnMessage(role string, content string)
	OnEvent(event AgentEvent) // Eventos estruturados com metadados completos
}

// noopHandler é um handler vazio usado quando nenhum handler é fornecido
type noopHandler struct{}

func (n noopHandler) OnStatusChange(string)  {}
func (n noopHandler) OnMessage(string, string) {}
func (n noopHandler) OnEvent(AgentEvent)       {}

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

// truncateStr trunca uma string para um tamanho máximo, adicionando "..." se necessário
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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
	sessionDir := ""
	if al.stateManager != nil {
		workspaceDir = al.stateManager.GetWorkspaceDir()
		sessionDir = filepath.Dir(al.stateManager.FilePath())
	}

	// Primeira iteração da sessão: injetar regras locais, stack e instruções de planejamento
	if len(messages) <= 2 && workspaceDir != "" {
		// 0.5. Identidade Agêntica (Lembrar o modelo de suas ferramentas e capacidade de alterar o sistema)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM AGENTIC IDENTITY] Você é um agente autônomo de IA com acesso completo ao sistema do usuário. Você pode ler/escrever arquivos locais (usando 'read_file', 'write_file', 'diff_replace'), executar comandos de terminal ('terminal_command'), e navegar na internet ou tirar prints de websites usando o navegador ('browser_action'). Você não necessita que o diretório atual seja um repositório Git para funcionar — trabalhe normalmente mesmo sem Git. Você NUNCA deve alegar ao usuário que é apenas um modelo de linguagem e que não pode criar/editar arquivos, rodar comandos, acessar a internet ou tirar screenshots. Use as ferramentas disponíveis imediatamente para executar o pedido do usuário.",
		})

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
			Content: "[SYSTEM PLANNING REQUIREMENT] Se a tarefa solicitada pelo usuário for complexa ou envolver múltiplos passos, você deve descrever e listar um plano de execução detalhado em formato de checklist markdown no início de sua resposta. Use o formato:\n- [ ] Nome da tarefa\n\nÀ medida que progredir, atualize o status das tarefas:\n- [/] Tarefa em andamento\n- [x] Tarefa concluída\n\nVocê deve sempre incluir o plano de trabalho atualizado no início de seu conteúdo de texto (content) em todas as respostas (mesmo quando estiver chamando ferramentas) para que o usuário possa acompanhar o progresso de forma estruturada. Se a tarefa for simples (como uma saudação 'oi' ou conversa rápida), responda diretamente e de forma amigável sem criar um plano.",
		})

		// 3.5. Exigência de Uso de Ferramentas para Escrita de Arquivos (Evitar responder apenas com markdown)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM TOOL USAGE REQUIREMENT] IMPORTANTE: Responder apenas com blocos de código markdown no chat NÃO cria, altera ou escreve arquivos no workspace do usuário. Se o seu plano envolve criar, editar ou excluir arquivos, você deve OBRIGATORIAMENTE chamar as ferramentas apropriadas (como 'write_file', 'diff_replace', etc.) para realizar essas ações no disco. Nunca marque uma tarefa de criação/modificação de código como concluída (- [x]) a menos que você tenha efetivamente executado a chamada de ferramenta correspondente com sucesso.",
		})

		// 3.6. Exigência de Uso do parâmetro 'path' para Capturas de Tela
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM SCREENSHOT PATH REQUIREMENT] IMPORTANTE: Ao tirar capturas de tela (screenshots) usando as ferramentas 'browser_action' ou 'computer_control', você deve OBRIGATORIAMENTE fornecer o parâmetro 'path' (ex: 'screenshot.png') se o usuário solicitou salvar a imagem. Se você não fornecer o 'path', a imagem NÃO será salva no disco e você não conseguirá salvá-la posteriormente usando a ferramenta 'write_file', pois a ferramenta 'write_file' serve apenas para arquivos de texto e a imagem é um arquivo binário.",
		})

		// 4. Diretório de Sessão para Artefatos, Tasks e Scripts
		if sessionDir != "" && strings.Contains(al.stateManager.FilePath(), "sessions") {
			relSessionDir, errRel := filepath.Rel(workspaceDir, sessionDir)
			displayDir := sessionDir
			if errRel == nil {
				displayDir = relSessionDir
			}
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: fmt.Sprintf("[SYSTEM SESSION ISOLATION] Qualquer arquivo de planejamento interno adicional (exceto o plan.md automático), scripts temporários internos do agente, rascunhos de testes ou checklists de tarefas internas (como task.md) devem ser salvos OBRIGATORIAMENTE dentro do diretório desta sessão: %s/. No entanto, arquivos de código fonte do projeto, capturas de tela/imagens solicitadas pelo usuário, relatórios finais ou quaisquer ativos/entregáveis que façam parte do projeto do usuário DEVEM ser salvos na pasta raiz do workspace ou no caminho explicitamente solicitado pelo usuário, e NÃO na pasta da sessão.", displayDir),
			})
		}
		saveMsgs(messages)
	}

	consecutiveFailures := 0
	timerScheduled := false

	for i := 0; i < al.config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			al.handler.OnMessage("system", "Loop cancelado pelo contexto.")
			al.handler.OnEvent(AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": AgentError{Code: ErrContextCanceled, Message: "Loop cancelado pelo contexto"},
				},
			})
			return ctx.Err()
		default:
		}

		iterationHasFailure := false
		al.handler.OnStatusChange("thinking")
		al.handler.OnEvent(AgentEvent{
			Timestamp: time.Now(),
			Event:     "thinking",
			Iteration: i + 1,
			Data: map[string]interface{}{
				"provider": al.provider.Name(),
				"model":    al.config.Model,
			},
		})

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
			errMsg := err.Error()
			al.handler.OnMessage("system", fmt.Sprintf("Erro na requisição ao LLM: %s", errMsg))

			// Determinar código de erro tipado
			errCode := ErrToolExecution
			if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "rate") || strings.Contains(errMsg, "Rate") {
				errCode = ErrLLMRateLimit
			} else if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "Unauthorized") {
				errCode = ErrLLMAuth
			}

			al.handler.OnEvent(AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": AgentError{
						Code:    errCode,
						Message: errMsg,
						Details: map[string]interface{}{"provider": al.provider.Name()},
					},
				},
			})
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

		// Emitir evento de mensagem estruturado com token usage
		al.handler.OnEvent(AgentEvent{
			Timestamp: time.Now(),
			Event:     "message",
			Iteration: i + 1,
			Data: map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
				"usage": map[string]int{
					"prompt_tokens":     resp.Usage.PromptTokens,
					"completion_tokens": resp.Usage.CompletionTokens,
					"total_tokens":      resp.Usage.TotalTokens,
				},
				"has_tool_calls": len(msg.ToolCalls) > 0,
			},
		})

		// Emitir texto do assistente (legado)
		if msg.Content != "" {
			al.handler.OnMessage("assistant", msg.Content)
		}

		// Se a resposta for totalmente vazia (sem texto e sem tool calls), é uma falha
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			al.handler.OnMessage("system", "Resposta vazia do LLM. Solicitando resposta válida.")
			al.handler.OnEvent(AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": AgentError{
						Code:    ErrLLMEmptyResponse,
						Message: "O modelo retornou uma resposta em branco.",
					},
				},
			})
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "[SYSTEM CORRECTION] Você enviou uma resposta em branco. Por favor, responda com texto ou execute uma chamada de ferramenta válida.",
			})
			saveMsgs(messages)
			iterationHasFailure = true
			consecutiveFailures++
			if consecutiveFailures >= al.config.MaxConsecutiveFail {
				al.handler.OnMessage("system", fmt.Sprintf("Abortando: %d falhas consecutivas.", al.config.MaxConsecutiveFail))
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data: map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
				})
				return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
			}
			continue
		} else if len(msg.ToolCalls) == 0 {
			if timerScheduled {
				al.handler.OnMessage("system", "Timer agendado. Suspendendo execução do agente até o timer expirar.")
				if al.stateManager != nil {
					_ = al.stateManager.SetStatus("idle")
					_ = al.stateManager.AddLog("Suspenso aguardando timer")
				}
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data: map[string]interface{}{"reason": "suspended_timer", "total_iterations": i + 1},
				})
				al.handler.OnStatusChange("idle")
				return nil
			}

			// Se não há chamadas de ferramentas, a tarefa foi concluída ou o agente respondeu textualmente ao usuário.
			// Finaliza o loop ReAct normalmente.
			if al.stateManager != nil {
				_ = al.stateManager.SetStatus("idle")
				_ = al.stateManager.AddLog(fmt.Sprintf("Tarefa concluída: %s", intent))
			}
			al.handler.OnEvent(AgentEvent{
				Timestamp: time.Now(),
				Event:     "finished",
				Iteration: i + 1,
				Data: map[string]interface{}{"reason": "completed", "total_iterations": i + 1},
			})
			al.handler.OnStatusChange("finished")
			return nil
		}

		// Executar tool calls
		iterationHasFailure = false
		for _, tc := range msg.ToolCalls {
			toolID := tc.Function.Name

			tool, exists := al.tools[toolID]
			if !exists {
				errMsg := fmt.Sprintf("Ferramenta '%s' não encontrada.", toolID)
				al.handler.OnMessage("system", errMsg)
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "error",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"error": AgentError{Code: ErrToolNotFound, Message: errMsg, Details: map[string]interface{}{"tool": toolID}},
					},
				})
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

			// Emitir evento estruturado de tool_call
			al.handler.OnEvent(AgentEvent{
				Timestamp: time.Now(),
				Event:     "tool_call",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"tool_call_id": tc.ID,
					"tool":         toolID,
					"arguments":    json.RawMessage(tc.Function.Arguments),
				},
			})

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
					al.handler.OnEvent(AgentEvent{
						Timestamp: time.Now(),
						Event:     "error",
						Iteration: i + 1,
						Data: map[string]interface{}{
							"error": AgentError{Code: ErrPermissionDenied, Message: errMsg, Details: map[string]interface{}{"tool": toolID, "target": target}},
						},
					})
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

				// Evento estruturado de tool_result com erro
				errCode := ErrToolExecution
				if toolCtx.Err() != nil {
					errCode = ErrToolTimeout
				}
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "tool_result",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"tool_call_id": tc.ID,
						"tool":         toolID,
						"success":      false,
						"error":        execErr.Error(),
						"error_code":   errCode,
					},
				})
				iterationHasFailure = true
				continue
			}

			if result.Success {
				if toolID == "schedule_timer" {
					timerScheduled = true
				}
				if strings.HasPrefix(result.Data, "image:base64:") {
					// Extrai texto adicional se houver
					toolMsgContent := "✓ Captura de tela realizada com sucesso."
					if idx := strings.Index(result.Data, "\n"); idx != -1 {
						toolMsgContent += " " + strings.TrimSpace(result.Data[idx+1:])
					}
					// 1. Adiciona a resposta da ferramenta como texto simples para validação do esquema da API
					messages = append(messages, llm.Message{
						Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: toolMsgContent,
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

				// Evento estruturado de tool_result com sucesso
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "tool_result",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"tool_call_id": tc.ID,
						"tool":         toolID,
						"success":      true,
						"output":       truncateStr(result.Data, 500),
					},
				})

			} else {
				errContent := formatToolError(toolID, result.Error)
				if toolID == "terminal_command" {
					errContent = FormatContextualError(errContent)
				}
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", fmt.Sprintf("Erro na ferramenta %s: %s", toolID, result.Error))

				// Evento estruturado de tool_result com falha lógica
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "tool_result",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"tool_call_id": tc.ID,
						"tool":         toolID,
						"success":      false,
						"error":        result.Error,
						"error_code":   ErrToolExecution,
					},
				})
				iterationHasFailure = true
			}
		}
		saveMsgs(messages)

		if iterationHasFailure {
			consecutiveFailures++
			if consecutiveFailures >= al.config.MaxConsecutiveFail {
				al.handler.OnMessage("system", fmt.Sprintf("Abortando: %d falhas consecutivas.", al.config.MaxConsecutiveFail))
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data: map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
				})
				return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
			}
		} else {
			consecutiveFailures = 0
		}
	}

	al.handler.OnMessage("system", "Limite de iterações atingido.")
	al.handler.OnEvent(AgentEvent{
		Timestamp: time.Now(),
		Event:     "finished",
		Iteration: al.config.MaxIterations,
		Data: map[string]interface{}{"reason": "max_iterations", "total_iterations": al.config.MaxIterations},
	})
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


