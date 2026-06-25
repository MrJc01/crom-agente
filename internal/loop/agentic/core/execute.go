package core

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"

	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
	"github.com/crom/crom-agente/internal/loop/agentic/workspace"
)

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
		// se tiver + de 500 palavras
		if al.config == nil || !al.config.DisablePromptOptimization || len(strings.Fields(intent)) > 500 {
			// Otimização do prompt inicial via camada agêntica
			optimized, err := prompting.OptimizePrompt(ctx, al.provider, al.promptManager, al.GetTools(), intent)
			if err == nil && optimized != "" {
				al.handler.OnMessage("system", i18n.Get("system.optimized_prompt_log", optimized))
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
				content := p.Content
				if p.ID == "SYSTEM_AGENTIC_IDENTITY" {
					content += "\n" + prompting.BuildToolsInstructions(al.promptManager, al.GetTools())
				}
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: content,
				})
			}
		}

		// 1. Detectar stack técnica (Dinâmico)
		stack := workspace.DetectStack(workspaceDir)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: i18n.Get("system.stack_detected", stack),
		})

		// 2. Carregar regras locais
		if localRules := workspace.LoadLocalRules(workspaceDir); localRules != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: i18n.Get("system.local_rules", localRules),
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
				Content: i18n.Get("system.session_isolation", displayDir),
			})
		}

		// 5. Injetar fase atual (Planning ou Execution)
		phase := loop.GetCurrentPhase(al.stateManager)
		if al.promptManager != nil {
			var phasePrompt config.PromptTemplate
			var ok bool
			if phase == loop.PhasePlanning {
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
			al.handler.OnMessage("system", i18n.Get("system.loop_canceled"))
			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": loop.AgentError{Code: loop.ErrContextCanceled, Message: "Loop cancelado pelo contexto"},
				},
			})
			return ctx.Err()
		default:
		}

		iterationHasFailure := false
		al.handler.OnStatusChange("thinking")
		al.handler.OnEvent(loop.AgentEvent{
			Timestamp: time.Now(),
			Event:     "thinking",
			Iteration: i + 1,
			Data: map[string]interface{}{
				"provider": al.provider.Name(),
				"model":    al.config.Model,
			},
		})

		// Detectar loops repetitivos
		if DetectRepetitiveLoop(messages) {
			al.handler.OnMessage("system", i18n.Get("system.repetitive_loop_detected"))
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: i18n.Get("system.repetitive_correction_prompt", 3),
			})
			saveMsgs(messages)
		}

		// Construir definições de ferramentas para o LLM
		opts := tooling.BuildRequestOptions(al.tools, intent)

		// Injetar plano de trabalho atualizado de forma dinâmica no contexto
		runMessages := messages
		if planCtx := loop.SyncPlanToContext(al.stateManager); planCtx != "" {
			// Cria uma cópia rasa para injetar a mensagem do sistema sem corromper o histórico salvo
			runMessages = make([]llm.Message, len(messages))
			copy(runMessages, messages)
			runMessages = append(runMessages, llm.Message{
				Role:    "system",
				Content: planCtx,
			})
		}

		// Chamar o LLM
		compactedMsgs := prompting.CompactMessages(ctx, al.provider, al.config.MaxMessageHistory, al.handler, runMessages)
		resp, err := al.provider.SendMessages(ctx, compactedMsgs, opts)
		if err != nil {
			errMsg := err.Error()
			al.handler.OnMessage("system", i18n.Get("errors.llm_error", i+1)+": "+errMsg)

			// Determinar código de erro tipado
			errCode := loop.ErrToolExecution
			if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "rate") || strings.Contains(errMsg, "Rate") {
				errCode = loop.ErrLLMRateLimit
			} else if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "Unauthorized") {
				errCode = loop.ErrLLMAuth
			}

			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": loop.AgentError{
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
		if pyToolCalls := loop.TryParseToolCode(msg.Content); len(pyToolCalls) > 0 {
			msg.ToolCalls = append(msg.ToolCalls, pyToolCalls...)
		}

		messages = append(messages, msg)
		saveMsgs(messages)

		// Atualiza o plano a partir do conteúdo textual da mensagem do assistente
		loop.UpdatePlannerFromMessage(al.stateManager, msg.Content)

		// Emitir evento de mensagem estruturado com token usage
		al.handler.OnEvent(loop.AgentEvent{
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
			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": loop.AgentError{
						Code:    loop.ErrLLMEmptyResponse,
						Message: i18n.Get("errors.optimizer_blank_response"),
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
				al.handler.OnMessage("system", i18n.Get("errors.abort_consecutive_failures", al.config.MaxConsecutiveFail))
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data:      map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
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
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data:      map[string]interface{}{"reason": "suspended_timer", "total_iterations": i + 1},
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
				if len(plan) > 0 && loop.HasPendingTasks(plan) {
					warning := loop.GeneratePendingTasksWarning(plan)
					al.handler.OnMessage("system", "Plano de trabalho com tarefas pendentes. Solicitando continuação.")
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
			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "finished",
				Iteration: i + 1,
				Data:      map[string]interface{}{"reason": "completed", "total_iterations": i + 1},
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
				errMsg := i18n.Get("errors.tool_not_found", toolID)
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName: toolID,
					Args:     tc.Function.Arguments,
					Success:  false,
					Output:   errMsg,
				})
				al.handler.OnMessage("system", errMsg)
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "error",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"error": loop.AgentError{Code: loop.ErrToolNotFound, Message: errMsg, Details: map[string]interface{}{"tool": toolID}},
					},
				})
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID,
					Content: tooling.FormatToolError(toolID, errMsg),
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
			al.handler.OnEvent(loop.AgentEvent{
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
					tooling.RenderDiffZone(al.handler, workspaceDir, tc.Function.Arguments, toolID)
				}

				approved, authErr := al.permissionManager.Authorize(ctx, toolID, target)
				if authErr != nil || !approved {
					errMsg := fmt.Sprintf("Ação '%s' rejeitada pelo usuário ou pelas políticas de segurança.", toolID)
					iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
						ToolName: toolID,
						Args:     tc.Function.Arguments,
						Success:  false,
						Output:   errMsg,
					})
					al.handler.OnMessage("system", errMsg)
					al.handler.OnEvent(loop.AgentEvent{
						Timestamp: time.Now(),
						Event:     "error",
						Iteration: i + 1,
						Data: map[string]interface{}{
							"error": loop.AgentError{Code: loop.ErrPermissionDenied, Message: errMsg, Details: map[string]interface{}{"tool": toolID, "target": target}},
						},
					})
					messages = append(messages, llm.Message{
						Role: "tool", ToolCallID: tc.ID, Name: toolID,
						Content: tooling.FormatToolError(toolID, errMsg),
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
				errContent := tooling.FormatToolError(toolID, execErr.Error())
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", i18n.Get("errors.tool_execution_failed", toolID)+": "+execErr.Error())

				// Evento estruturado de tool_result com erro
				errCode := loop.ErrToolExecution
				if toolCtx.Err() != nil {
					errCode = loop.ErrToolTimeout
				}
				al.handler.OnEvent(loop.AgentEvent{
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
				al.handler.OnEvent(loop.AgentEvent{
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
				errContent := tooling.FormatToolError(toolID, errMsg)
				if toolID == "terminal_command" {
					errContent = loop.FormatContextualError(errContent)
				}
				messages = append(messages, llm.Message{
					Role: "tool", ToolCallID: tc.ID, Name: toolID, Content: errContent,
				})
				al.handler.OnMessage("system", fmt.Sprintf("Erro na ferramenta %s: %s", toolID, errMsg))

				// Evento estruturado de tool_result com falha lógica
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "tool_result",
					Iteration: i + 1,
					Data: map[string]interface{}{
						"tool_call_id": tc.ID,
						"tool":         toolID,
						"success":      false,
						"error":        errMsg,
						"error_code":   loop.ErrToolExecution,
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
				al.handler.OnMessage("system", i18n.Get("errors.abort_consecutive_failures", al.config.MaxConsecutiveFail))
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "finished",
					Iteration: i + 1,
					Data:      map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
				})
				return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
			}
		} else {
			consecutiveFailures = 0
		}
	}

	al.handler.OnMessage("system", "Limite de iterações atingido.")
	al.handler.OnEvent(loop.AgentEvent{
		Timestamp: time.Now(),
		Event:     "finished",
		Iteration: al.config.MaxIterations,
		Data:      map[string]interface{}{"reason": "max_iterations", "total_iterations": al.config.MaxIterations},
	})
	al.handler.OnStatusChange("idle")
	return fmt.Errorf("limite de %d iterações atingido", al.config.MaxIterations)
}

// truncateStr trunca uma string
func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
