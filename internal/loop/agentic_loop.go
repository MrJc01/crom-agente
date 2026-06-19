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
			Content: "[SYSTEM AGENTIC IDENTITY] Você é um agente autônomo com acesso total de leitura e escrita ao workspace atual do usuário através de ferramentas dedicadas (como 'write_file', 'read_file', 'terminal_command', etc.). Você NUNCA deve alegar ao usuário que é apenas um modelo de linguagem e que não pode criar, ler, editar ou rodar arquivos locais. Se o usuário pedir para criar, modificar, ler ou testar arquivos, use as ferramentas disponíveis imediatamente para realizar a ação no computador dele.",
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
			Content: "[SYSTEM PLANNING REQUIREMENT] Na sua primeira resposta, antes de executar qualquer ferramenta ou comando de terminal, você deve obrigatoriamente descrever e listar um plano de execução detalhado em formato de checklist markdown. Use o formato:\n- [ ] Nome da tarefa\n\nÀ medida que progredir, você deve atualizar o status das tarefas na sua resposta usando o mesmo formato:\n- [/] Tarefa em andamento\n- [x] Tarefa concluída\n\nVocê deve sempre incluir o plano de trabalho atualizado no início de seu conteúdo de texto (content) em todas as respostas (mesmo quando estiver chamando ferramentas) para que o usuário possa acompanhar o progresso de forma estruturada.",
		})

		// 3.5. Exigência de Uso de Ferramentas para Escrita de Arquivos (Evitar responder apenas com markdown)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM TOOL USAGE REQUIREMENT] IMPORTANTE: Responder apenas com blocos de código markdown no chat NÃO cria, altera ou escreve arquivos no workspace do usuário. Se o seu plano envolve criar, editar ou excluir arquivos, você deve OBRIGATORIAMENTE chamar as ferramentas apropriadas (como 'write_file', 'diff_replace', etc.) para realizar essas ações no disco. Nunca marque uma tarefa de criação/modificação de código como concluída (- [x]) a menos que você tenha efetivamente executado a chamada de ferramenta correspondente com sucesso.",
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
				Content: fmt.Sprintf("[SYSTEM SESSION ISOLATION] Qualquer arquivo de planejamento adicional (exceto o plan.md automático), scripts temporários, rascunhos de testes, checklists de tarefas (como task.md) ou artefatos gerados especificamente para esta sessão devem ser salvos OBRIGATORIAMENTE dentro do diretório desta sessão: %s/. Use este caminho para ler/escrever recursos relacionados ao contexto deste chat.", displayDir),
			})
		}
		saveMsgs(messages)
	}

	hasExecutedTool := false
	hasVerified := false
	hasSelfChecked := false
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

		// Sem tool calls: verificar se devemos encerrar ou entrar em fase de verificação
		if len(msg.ToolCalls) == 0 {
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

			// Resposta vazia: auto-correção
			if msg.Content == "" {
				al.handler.OnMessage("system", "Auto-correção: resposta vazia recebida.")
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "error",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"error": AgentError{Code: ErrLLMEmptyResponse, Message: "Resposta vazia recebida do LLM"},
					},
				})
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: "[SYSTEM AUTO-CORRECTION] Sua resposta estava vazia. Forneça uma resposta textual ou execute uma ferramenta.",
				})
				saveMsgs(messages)
				consecutiveFailures++
				if consecutiveFailures >= al.config.MaxConsecutiveFail {
					al.handler.OnEvent(AgentEvent{
						Timestamp: time.Now(),
						Event:     "finished",
						Iteration: i + 1,
						Data: map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
					})
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

			// CORREÇÃO: Se nenhuma ferramenta foi executada e a resposta contém tarefas pendentes (plano),
			// forçar o agente a começar a executar em vez de encerrar prematuramente.
			if !hasExecutedTool && containsPendingPlan(msg.Content) {
				al.handler.OnMessage("system", "Plano detectado sem execução. Forçando início da execução.")
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: "[SYSTEM EXECUTION REQUIRED] Você descreveu um plano com tarefas pendentes (- [ ]) mas NÃO executou nenhuma ferramenta. Pare de apenas descrever e COMECE A EXECUTAR AGORA usando as ferramentas disponíveis (write_file, terminal_command, etc.). Crie os arquivos, escreva o código e execute os comandos necessários. Não peça confirmação — execute o plano imediatamente.",
				})
				saveMsgs(messages)
				continue
			}

			// CORREÇÃO: Se nenhuma ferramenta foi executada, mas o assistente marcou itens de escrita/criação como concluídos
			// e apresentou blocos de código, alertar que as ferramentas físicas de escrita de arquivos devem ser chamadas.
			if !hasExecutedTool && hasCodeBlocksAndCompletedPlan(msg.Content) {
				al.handler.OnMessage("system", "Código detectado em markdown sem execução de ferramenta.")
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: "[SYSTEM TOOL USAGE REQUIRED] Você marcou tarefas de criação/modificação como concluídas e forneceu o código no chat, mas NÃO executou nenhuma ferramenta de escrita de arquivos (como write_file ou diff_replace). Lembre-se: blocos de código markdown no chat NÃO criam arquivos no disco do usuário. Você deve obrigatoriamente chamar a ferramenta apropriada para salvar o código nos arquivos correspondentes no disco antes de considerar a tarefa concluída.",
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
				if ok, errMsg := al.autoValidate(ctx, workspaceDir); !ok {
					hasVerified = true // Para impedir loop infinito
					al.handler.OnMessage("system", "Falha na auto-validação estática.")
					messages = append(messages, llm.Message{
						Role:    "user",
						Content: errMsg,
					})
					saveMsgs(messages)
					continue
				}
			}

			// Auto-verificação final: comparar com o pedido original do usuário antes de encerrar
			if al.config.AutoSelfCheck && !hasSelfChecked {
				hasSelfChecked = true
				userIntent := extractLastUserIntent(messages)
				if userIntent != "" {
					al.handler.OnMessage("system", "Auto-verificação: confirmando se a tarefa foi concluída.")
					messages = append(messages, llm.Message{
						Role: "system",
						Content: fmt.Sprintf(`[SYSTEM SELF-CHECK] Antes de encerrar, verifique se você realmente completou o que o usuário pediu.
O pedido original/último do usuário foi: %q
Se você NÃO criou todos os arquivos necessários ou NÃO executou todos os passos, continue trabalhando agora usando as ferramentas.
Se tudo foi realmente concluído, responda com um resumo final do que foi feito.`, userIntent),
					})
					saveMsgs(messages)
					continue
				}
			}

			// Loop encerrado normalmente
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
		iterationHasFailure := false
		for _, tc := range msg.ToolCalls {
			hasExecutedTool = true
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

// containsPendingPlan verifica se o conteúdo da mensagem contém itens de checklist pendentes
func containsPendingPlan(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [/]") {
			return true
		}
	}
	return false
}

// extractLastUserIntent retorna o conteúdo da última mensagem do usuário (não-system) para auto-verificação
func extractLastUserIntent(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && !strings.HasPrefix(messages[i].Content, "[SYSTEM") {
			return messages[i].Content
		}
	}
	return ""
}

// hasCodeBlocksAndCompletedPlan verifica se a mensagem contém blocos de código e tarefas completas que deveriam ter usado ferramentas
func hasCodeBlocksAndCompletedPlan(content string) bool {
	if !strings.Contains(content, "```") {
		return false
	}
	lines := strings.Split(content, "\n")
	actionVerbs := []string{
		"criar", "create", "implementar", "implement", "escrever", "write",
		"salvar", "save", "adicionar", "add", "editar", "edit", "atualizar",
		"update", "excluir", "delete", "remover", "remove", "gerar", "generate",
		"crud", "setup", "configurar", "definir", "define",
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isCompleted := false
		var title string
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "* [x]") ||
			strings.HasPrefix(trimmed, "- [X]") || strings.HasPrefix(trimmed, "* [X]") {
			isCompleted = true
			title = strings.TrimSpace(trimmed[5:])
		}
		if isCompleted && title != "" {
			titleLower := strings.ToLower(title)
			for _, verb := range actionVerbs {
				if strings.Contains(titleLower, verb) {
					return true
				}
			}
		}
	}
	return false
}

