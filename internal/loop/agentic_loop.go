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
		Authorize(ctx context.Context, action, target string) (bool, error)
	}
	promptManager     *config.PromptManager
}

func (al *AgenticLoop) SetPermissionManager(pm interface {
	Authorize(ctx context.Context, action, target string) (bool, error)
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
			MaxIterations:             15,
			MaxConsecutiveFail:        3,
			ToolTimeoutSeconds:        30,
			MaxMessageHistory:         40,
			AutoVerify:                true,
			PermissionMode:            "scoped",
			DisablePromptOptimization: true, // Disables by default in tests that don't pass a custom config!
		}
	}

	var pm *config.PromptManager
	if sm != nil {
		pm = config.NewPromptManager(sm.GetWorkspaceDir())
	}

	return &AgenticLoop{
		provider:      provider,
		tools:         make(map[string]tools.Tool),
		stateManager:  sm,
		handler:       handler,
		config:        resolvedCfg,
		promptManager: pm,
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
		if al.config == nil || !al.config.DisablePromptOptimization {
			// Otimização do prompt inicial via camada agêntica
			optimized, err := al.OptimizePrompt(ctx, intent)
			if err == nil && optimized != "" {
				al.handler.OnMessage("system", fmt.Sprintf("🔮 Prompt otimizado pelo Agente:\n%s", optimized))
				intent = optimized
				if al.stateManager != nil {
					_ = al.stateManager.SetActiveTask(intent)
				}
			}
		}
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
	hasAgenticIdentity := false
	for _, m := range messages {
		if m.Role == "system" && strings.Contains(m.Content, "[SYSTEM AGENTIC IDENTITY]") {
			hasAgenticIdentity = true
			break
		}
	}

	if !hasAgenticIdentity && workspaceDir != "" {
		if al.promptManager != nil {
			for _, p := range al.promptManager.GetAllEnabled() {
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: p.Content,
				})
			}
		}

		// 1. Detectar stack técnica (Dinâmico)
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

		// 4. Diretório de Sessão para Artefatos, Tasks e Scripts (Dinâmico)
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

		// 5. Injetar fase atual (Planning ou Execution)
		phase := GetCurrentPhase(al.stateManager)
		if al.promptManager != nil {
			var phasePrompt config.PromptTemplate
			var ok bool
			if phase == PhasePlanning {
				phasePrompt, ok = al.promptManager.GetPrompt("phase_planning")
			} else {
				phasePrompt, ok = al.promptManager.GetPrompt("phase_execution")
			}
			if ok && phasePrompt.Enabled {
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: phasePrompt.Content,
				})
			}
		}
		saveMsgs(messages)
	}

	consecutiveFailures := 0
	timerScheduled := false

	for i := 0; i < al.config.MaxIterations; i++ {
		iterLog := state.IterationLog{
			Iteration: i + 1,
			Timestamp: time.Now(),
		}

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
		opts := al.buildRequestOptions(intent)

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
		compactedMsgs := al.compactMessages(ctx, runMessages)
		resp, err := al.provider.SendMessages(ctx, compactedMsgs, opts)
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

		iterLog.PromptTokens = resp.Usage.PromptTokens
		iterLog.CompletionTokens = resp.Usage.CompletionTokens
		iterLog.TotalTokens = resp.Usage.TotalTokens
		iterLog.MessagesCount = len(compactedMsgs)
		iterLog.Messages = make([]llm.Message, len(compactedMsgs))
		copy(iterLog.Messages, compactedMsgs)

		// Interceptar chamadas de ferramentas alucinadas no formato Python /tool_code
		if pyToolCalls := tryParseToolCode(msg.Content); len(pyToolCalls) > 0 {
			msg.ToolCalls = append(msg.ToolCalls, pyToolCalls...)
		}

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
				if al.stateManager != nil {
					_ = al.stateManager.SaveIterationLog(i+1, iterLog)
				}
				return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
			}
			if al.stateManager != nil {
				_ = al.stateManager.SaveIterationLog(i+1, iterLog)
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
				if al.stateManager != nil {
					_ = al.stateManager.SaveIterationLog(i+1, iterLog)
				}
				return nil
			}


			// Se não há chamadas de ferramentas, a tarefa foi concluída ou o agente respondeu textualmente ao usuário.
			// Porém, se ainda houver tarefas pendentes ou em progresso no plano, avisa o agente e continua a iteração.
			if al.stateManager != nil {
				plan := al.stateManager.GetPlan()
				if len(plan) > 0 && HasPendingTasks(plan) {
					warning := GeneratePendingTasksWarning(plan)
					al.handler.OnMessage("system", "⚠️ Plano de trabalho com tarefas pendentes. Solicitando continuação.")
					messages = append(messages, llm.Message{
						Role:    "system",
						Content: warning,
					})
					saveMsgs(messages)
					if al.stateManager != nil {
						_ = al.stateManager.SaveIterationLog(i+1, iterLog)
					}
					continue
				}
			}

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
			if al.stateManager != nil {
				_ = al.stateManager.SaveIterationLog(i+1, iterLog)
			}
			return nil
		}

		// Executar tool calls
		iterationHasFailure = false
		for idx := range msg.ToolCalls {
			tc := &msg.ToolCalls[idx]
			toolID := tc.Function.Name

			// Fallback para modelos que confundem e chamam "screenshot" diretamente em vez de "browser_action" ou "computer_control"
			if toolID == "screenshot" {
				if _, hasBrowser := al.tools["browser_action"]; hasBrowser {
					toolID = "browser_action"
					tc.Function.Name = "browser_action"
					var rawArgs map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &rawArgs); err == nil {
						if _, hasAction := rawArgs["action"]; !hasAction {
							rawArgs["action"] = "screenshot"
							if newArgs, errMar := json.Marshal(rawArgs); errMar == nil {
								tc.Function.Arguments = string(newArgs)
							}
						}
					}
				} else if _, hasComputer := al.tools["computer_control"]; hasComputer {
					toolID = "computer_control"
					tc.Function.Name = "computer_control"
					var rawArgs map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &rawArgs); err == nil {
						if _, hasAction := rawArgs["action"]; !hasAction {
							rawArgs["action"] = "screenshot"
							if newArgs, errMar := json.Marshal(rawArgs); errMar == nil {
								tc.Function.Arguments = string(newArgs)
							}
						}
					}
				}
			}

			tool, exists := al.tools[toolID]
			if !exists {
				errMsg := fmt.Sprintf("Ferramenta '%s' não encontrada.", toolID)
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       tc.Function.Arguments,
					Success:    false,
					Output:     errMsg,
				})
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
			if al.permissionManager != nil && tool.RequiresApproval() {
				target := tc.Function.Arguments // Default to raw JSON arguments so it is never empty
				if toolID == "terminal_command" {
					var argsCmd struct {
						Command string `json:"command"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsCmd); err == nil && argsCmd.Command != "" {
						target = argsCmd.Command
					}
				} else if toolID == "write_file" || toolID == "read_file" {
					var argsPath struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsPath); err == nil && argsPath.Path != "" {
						target = argsPath.Path
					}
				} else if toolID == "diff_replace" {
					var argsPath struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsPath); err == nil && argsPath.Path != "" {
						target = argsPath.Path
					}
				} else if toolID == "browser_action" {
					var argsBrowser struct {
						Action   string `json:"action"`
						URL      string `json:"url,omitempty"`
						Selector string `json:"selector,omitempty"`
						Text     string `json:"text,omitempty"`
						Path     string `json:"path,omitempty"`
					}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsBrowser); err == nil {
						var parts []string
						parts = append(parts, fmt.Sprintf("Ação: %s", argsBrowser.Action))
						if argsBrowser.URL != "" {
							parts = append(parts, fmt.Sprintf("URL: %s", argsBrowser.URL))
						}
						if argsBrowser.Selector != "" {
							parts = append(parts, fmt.Sprintf("Seletor: %s", argsBrowser.Selector))
						}
						if argsBrowser.Text != "" {
							parts = append(parts, fmt.Sprintf("Texto: %s", argsBrowser.Text))
						}
						if argsBrowser.Path != "" {
							parts = append(parts, fmt.Sprintf("Salvar em: %s", argsBrowser.Path))
						}
						target = strings.Join(parts, " | ")
					}
				}

				// DiffZones: Renderizar diff colorido antes de pedir autorização para ferramentas de escrita
				if toolID == "write_file" || toolID == "diff_replace" {
					al.renderDiffZone(tc.Function.Arguments, toolID, workspaceDir)
				}

				approved, authErr := al.permissionManager.Authorize(ctx, toolID, target)
				if authErr != nil || !approved {
					errMsg := fmt.Sprintf("Ação '%s' rejeitada pelo usuário ou pelas políticas de segurança.", toolID)
					iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
						ToolName:   toolID,
						Args:       tc.Function.Arguments,
						Success:    false,
						Output:     errMsg,
					})
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
			toolStartTime := time.Now()
			toolCtx, cancel := context.WithTimeout(ctx, time.Duration(al.config.ToolTimeoutSeconds)*time.Second)
			result, execErr := tool.Execute(toolCtx, json.RawMessage(tc.Function.Arguments))
			cancel()
			toolDuration := time.Since(toolStartTime).Milliseconds()

			if execErr != nil {
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       tc.Function.Arguments,
					Success:    false,
					Output:     execErr.Error(),
					DurationMs: toolDuration,
				})
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
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       tc.Function.Arguments,
					Success:    true,
					Output:     result.Data,
					DurationMs: toolDuration,
				})
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
				errMsg := result.Error
				if errMsg == "" && result.Data != "" {
					errMsg = result.Data
				}
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       tc.Function.Arguments,
					Success:    false,
					Output:     errMsg,
					DurationMs: toolDuration,
				})
				errContent := formatToolError(toolID, errMsg)
				if toolID == "terminal_command" {
					errContent = FormatContextualError(errContent)
				}
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", fmt.Sprintf("Erro na ferramenta %s: %s", toolID, errMsg))

				// Evento estruturado de tool_result com falha lógica
				al.handler.OnEvent(AgentEvent{
					Timestamp: time.Now(),
					Event:     "tool_result",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"tool_call_id": tc.ID,
						"tool":         toolID,
						"success":      false,
						"error":        errMsg,
						"error_code":   ErrToolExecution,
					},
				})
				iterationHasFailure = true
			}
		}
		saveMsgs(messages)

		if al.stateManager != nil {
			_ = al.stateManager.SaveIterationLog(i+1, iterLog)
		}

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
func (al *AgenticLoop) buildRequestOptions(intent string) llm.RequestOptions {
	if len(al.tools) == 0 {
		return llm.RequestOptions{}
	}

	defs := make([]llm.ToolDefinition, 0, len(al.tools))
	intentLower := strings.ToLower(intent)
	
	for _, t := range al.tools {
		// Tool Pruning Rudimentar: se temos muitas ferramentas, podemos podar ferramentas super específicas
		// se a intenção atual claramente não envolve seus domínios (ex: mcp)
		if strings.HasPrefix(t.ID(), "mcp_") && !strings.Contains(intentLower, "mcp") && !strings.Contains(intentLower, "external") {
			// Skip MCP tools se não parecerem relevantes
			continue
		}

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

// compactMessages aplica compactação inteligente usando o LLM para resumir o meio da conversa
func (al *AgenticLoop) compactMessages(ctx context.Context, messages []llm.Message) []llm.Message {
	maxMsgs := al.config.MaxMessageHistory
	if maxMsgs <= 0 {
		maxMsgs = 40
	}

	if len(messages) <= maxMsgs {
		return messages
	}

	keepRecent := 15
	if maxMsgs < 20 {
		keepRecent = 5
	}

	middleStart := 0
	for i, m := range messages {
		if m.Role == "user" {
			middleStart = i + 1
			break
		}
	}

	if middleStart == 0 || middleStart >= len(messages)-keepRecent {
		// Fallback para truncamento simples
		keepFromEnd := maxMsgs - 1
		compacted := make([]llm.Message, 0, maxMsgs)
		compacted = append(compacted, messages[0])
		compacted = append(compacted, messages[len(messages)-keepFromEnd:]...)
		return compacted
	}

	middleEnd := len(messages) - keepRecent

	var toSummarize string
	for i := middleStart; i < middleEnd; i++ {
		content := truncateStr(messages[i].Content, 500)
		toSummarize += fmt.Sprintf("[%s]: %s\n", messages[i].Role, content)
		if len(messages[i].ToolCalls) > 0 {
			toSummarize += fmt.Sprintf("[%s] executou %d chamadas de ferramenta.\n", messages[i].Role, len(messages[i].ToolCalls))
		}
	}

	summaryPrompt := fmt.Sprintf("Resuma de forma extremamente concisa (max 2 parágrafos) os eventos, resultados e decisões do bloco de conversa a seguir para que o agente principal saiba o que já foi tentado e concluído.\n\nCONVERSA:\n%s", toSummarize)

	resp, err := al.provider.SendMessages(ctx, []llm.Message{
		{Role: "system", Content: "Você resume histórico técnico do agente. Seja direto e objetivo."},
		{Role: "user", Content: summaryPrompt},
	}, llm.RequestOptions{})

	var compacted []llm.Message
	if err == nil && resp.Message.Content != "" {
		al.handler.OnMessage("system", "Otimização de tokens: histórico intermediário resumido via LLM.")
		compacted = append(compacted, messages[:middleStart]...)
		compacted = append(compacted, llm.Message{
			Role:    "system",
			Content: fmt.Sprintf("[SYSTEM HISTORY SUMMARY] Um trecho intermediário da conversa foi compactado:\n%s", resp.Message.Content),
		})
		compacted = append(compacted, messages[middleEnd:]...)
	} else {
		// Fallback
		keepFromEnd := maxMsgs - 1
		compacted = append(compacted, messages[0])
		compacted = append(compacted, messages[len(messages)-keepFromEnd:]...)
	}

	log.Printf("[AgenticLoop] Compactou histórico de %d para %d mensagens", len(messages), len(compacted))
	return compacted
}

const optimizerSystemPrompt = `Você é um Engenheiro de Prompt Especialista e Arquiteto de Software de Elite, especializado em projetar instruções de alta fidelidade para agentes autônomos de IA (como Claude Code e Antigravity).
Sua tarefa é analisar o prompt original do usuário e transformá-lo em uma instrução otimizada, detalhada e estruturada para um agente autônomo.

Ao reescrever o prompt, você deve formatar o resultado com as seguintes seções explícitas:

1. **Objetivo Principal**: Descrição clara e inequívoca do que deve ser alcançado.
2. **Questões de Alinhamento / Clarificações**: Instrua o agente a identificar e listar no início de sua primeira resposta quaisquer dúvidas cruciais, ambiguidades técnicas ou decisões de design arquitetural importantes.
3. **Plano de Mudanças de Arquivos (Proposed Changes)**: Exija que o agente liste explicitamente todos os arquivos a criar (NEW), modificar (MODIFY) ou deletar (DELETE) antes de fazer modificações no disco.
4. **Instruções de Execução Ativa e Uso de Ferramentas**:
   - O agente deve começar a criar e escrever arquivos reais imediatamente no primeiro turno usando as ferramentas apropriadas ('write_file', 'terminal_command', etc.).
   - O agente NUNCA deve apenas planejar na primeira iteração ou apresentar blocos de código em markdown no chat sem invocar as respectivas ferramentas correspondentes para aplicar as mudanças físicas no disco.
   - O agente deve manter um plano de trabalho atualizado no início de todas as mensagens usando checklists markdown ('- [ ]' para pendente, '- [/]' para em andamento, '- [x]' para concluído).
   - Mesmo que o agente precise pedir confirmações ou tenha questões de alinhamento, ele deve obrigatoriamente chamar ao menos uma ferramenta na primeira resposta (ex: ler arquivos, listar diretórios, criar arquivos de esqueleto) para iniciar ativamente a execução e evitar a suspensão por inatividade.
5. **Requisitos Não Funcionais**: Requisitos de segurança, performance, tratamento de erros robusto e arquitetura limpa adequados para a stack técnica identificada.
6. **Critérios de Aceitação e Testes**: Conjunto de asserções que o agente deve validar para confirmar o sucesso do projeto (ex: compilação, testes unitários, validação manual).
7. **Uso de Ferramentas Nativas de Navegação**:
   - O agente possui ferramentas nativas de navegação na internet e automação de navegador ('browser_action' e 'browser_subagent').
   - Se o prompt original do usuário solicitar tarefas de navegação, acesso a sites, cliques, digitações ou screenshots de páginas da web, instrua o agente a utilizar suas ferramentas nativas do navegador ('browser_action' ou 'browser_subagent') diretamente em tempo real, em vez de planejar a criação ou compilação de scripts de automação de terceiros (como Selenium, Playwright, Puppeteer ou scripts Python/JS) a menos que o usuário tenha solicitado explicitamente o código-fonte de um programa de automação.

O seu retorno deve ser APENAS o novo prompt otimizado estruturado, sem introduções, explicações ou notas de rodapé adicionais.`

// OptimizePrompt executa uma chamada de LLM para refinar e enriquecer o prompt do usuário antes do loop ReAct
func (al *AgenticLoop) OptimizePrompt(ctx context.Context, rawPrompt string) (string, error) {
	// Se o prompt for muito curto ou for um comando simples/TUI slash command, não otimiza
	if len(rawPrompt) < 5 || strings.HasPrefix(rawPrompt, "/") {
		return rawPrompt, nil
	}

	messages := []llm.Message{
		{Role: "system", Content: optimizerSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Prompt original do usuário: %q\n\nPor favor, retorne o prompt otimizado de forma direta:", rawPrompt)},
	}

	resp, err := al.provider.SendMessages(ctx, messages, llm.RequestOptions{})
	if err != nil {
		return "", err
	}

	optimized := strings.TrimSpace(resp.Message.Content)
	if optimized == "" {
		return "", fmt.Errorf("resposta de otimização em branco")
	}

	return optimized, nil
}



