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
	"github.com/crom/crom-agente/internal/tools"

	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
	"github.com/crom/crom-agente/internal/loop/agentic/workspace"
)

// Execute roda o loop ReAct completo para a tarefa dada
func (al *AgenticLoop) Execute(ctx context.Context, intent string) error {
	al.textOnlyMode = false
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

	if isSimpleIntent(intent) && len(messages) == 0 {
		resp, err := al.generateSimpleResponse(ctx, intent)
		if err == nil {
			messages = append(messages, llm.Message{Role: "user", Content: intent})
			messages = append(messages, llm.Message{Role: "assistant", Content: resp})
			saveMsgs(messages)

			if al.stateManager != nil {
				_ = al.stateManager.SetOperationalStatus(state.StatusIdle)
			}
			al.handler.OnMessage("assistant", resp)
			al.handler.OnStatusChange("idle")
			return nil
		}
	}

	workspaceDir := ""
	sessionDir := ""
	if al.stateManager != nil {
		workspaceDir = al.stateManager.GetWorkspaceDir()
		sessionDir = filepath.Dir(al.stateManager.FilePath())
	}

	if len(messages) == 0 {
		// 1. Otimização do prompt inicial via camada agêntica
		if al.config == nil || !al.config.DisablePromptOptimization || len(strings.Fields(intent)) > 500 {
			optimized, err := prompting.OptimizePrompt(ctx, al.provider, al.promptManager, al.GetTools(), intent)
			if err == nil && optimized != "" {
				al.handler.OnMessage("system", i18n.Get("system.optimized_prompt_log", optimized))
				intent = optimized
				if al.stateManager != nil {
					_ = al.stateManager.SetActiveTask(intent)
				}
			}
		}

		// 2. Injetar todas as mensagens de sistema *antes* do prompt do usuário
		if workspaceDir != "" {
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

			// 2.1. Detectar stack técnica (Dinâmico)
			stack := workspace.DetectStack(workspaceDir)
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: i18n.Get("system.stack_detected", stack),
			})

			// 2.2. Carregar regras locais
			if localRules := workspace.LoadLocalRules(workspaceDir); localRules != "" {
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: i18n.Get("system.local_rules", localRules),
				})
			}

			// 2.3. Diretório de Sessão para Artefatos, Tasks e Scripts (Dinâmico)
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

			// 2.4. Injetar fase atual (Planning ou Execution)
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
		}

		// 3. Adicionar a intenção original ou otimizada do usuário
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: intent,
		})
		saveMsgs(messages)
	} else {
		// Se já existir histórico, apenas adicionamos a nova intenção à conversação
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: intent,
		})
		saveMsgs(messages)
	}

	consecutiveNoToolCallTurns := 0
	consecutiveFailures := 0
	consecutiveRetryCount := 0
	timerScheduled := false
	lastIterFailed := false
	lastToolWasValidation := false

	for i := 0; ; i++ {
		if al.config.MaxIterations > 0 && i >= al.config.MaxIterations {
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
		askUserCalled := false
		iterLog := state.IterationLog{
			Iteration: i + 1,
			Timestamp: time.Now(),
		}

		// Injetar mensagens pendentes do usuário enviadas em tempo real
		userMsgInjected := false
		al.mu.Lock()
		if len(al.pendingUserMessages) > 0 {
			userMsgInjected = true
			for _, pendingMsg := range al.pendingUserMessages {
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: pendingMsg,
				})
				al.handler.OnMessage("user", pendingMsg)
			}
			al.pendingUserMessages = nil
			saveMsgs(messages)
		}
		al.mu.Unlock()

		// Determinar ModoCognitivo
		modo := state.ModoExecuting // Default
		planIsEmpty := true
		if al.stateManager != nil {
			plan := al.stateManager.GetPlan()
			if len(plan) > 0 {
				planIsEmpty = false
			}
		}

		if userMsgInjected {
			modo = state.ModoInteracting
		} else if planIsEmpty {
			modo = state.ModoPlanning
		} else if lastIterFailed {
			modo = state.ModoDebugging
		} else if lastToolWasValidation {
			modo = state.ModoVerifying
		}

		if al.stateManager != nil {
			_ = al.stateManager.SetCognitiveMode(modo)
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
		copied := false
		if planCtx := loop.SyncPlanToContext(al.stateManager); planCtx != "" {
			// Cria uma cópia rasa para injetar a mensagem do sistema sem corromper o histórico salvo
			runMessages = make([]llm.Message, len(messages))
			copy(runMessages, messages)
			copied = true
			runMessages = append(runMessages, llm.Message{
				Role:    "system",
				Content: planCtx,
			})
		}

		// Injetar diretrizes do ModoCognitivo atual de forma dinâmica
		if al.promptManager != nil {
			if phasePrompt, ok := al.promptManager.GetPrompt("phase_" + modo); ok && phasePrompt.Enabled {
				if !copied {
					runMessages = make([]llm.Message, len(messages))
					copy(runMessages, messages)
					copied = true
				}
				runMessages = append(runMessages, llm.Message{
					Role:    "system",
					Content: phasePrompt.Content,
				})
			}
		}

		// Injetar diretrizes do modo text-only de forma dinâmica se ativado
		if al.textOnlyMode {
			var textOnlyPromptContent string
			if al.promptManager != nil {
				if textOnlyPrompt, ok := al.promptManager.GetPrompt("text_only_mode"); ok && textOnlyPrompt.Enabled {
					textOnlyPromptContent = textOnlyPrompt.Content
				}
			}
			if textOnlyPromptContent == "" {
				textOnlyPromptContent = "[SYSTEM] ATENÇÃO: O modelo/provedor atual não suporta chamadas de função (tool use) nativas. Você deve gerar chamadas de ferramentas no corpo do texto em formato markdown/XML para que sejam parseadas e executadas. Por exemplo, para criar/escrever um arquivo, escreva o seguinte bloco no texto:\n\n```python\n# FILE: caminho/do/arquivo\n# Seu código aqui\n```\nNão tente fazer chamadas de função JSON tradicionais."
			}

			if !copied {
				runMessages = make([]llm.Message, len(messages))
				copy(runMessages, messages)
				copied = true
			}
			runMessages = append(runMessages, llm.Message{
				Role:    "system",
				Content: textOnlyPromptContent,
			})
		}

		// Chamar o LLM
		compactedMsgs := prompting.CompactMessages(ctx, al.provider, al.config.MaxMessageHistory, al.handler, runMessages)
		finalMsgs := FormatMessagesForModel(compactedMsgs, al.provider)
		resp, err := al.provider.SendMessages(ctx, finalMsgs, opts)
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

		if resp.ToolUseDisabled {
			if !al.textOnlyMode {
				al.textOnlyMode = true
				al.handler.OnMessage("system", i18n.Get("system.text_only_mode_activated"))
			}
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
			if al.stateManager != nil {
				_ = al.stateManager.RecordToolCallsFromTextParse(len(pyToolCalls))
			}
		}

		// Interceptar chamadas de ferramentas em formato de bloco de código markdown (modo text-only)
		if al.textOnlyMode {
			if markdownToolCalls := loop.TryParseMarkdownToolCalls(msg.Content); len(markdownToolCalls) > 0 {
				msg.ToolCalls = append(msg.ToolCalls, markdownToolCalls...)
				if al.stateManager != nil {
					_ = al.stateManager.RecordToolCallsFromTextParse(len(markdownToolCalls))
				}
			}
		}

		// Circuit Breaker logic
		if len(msg.ToolCalls) > 0 {
			consecutiveNoToolCallTurns = 0
			if al.stateManager != nil {
				for range msg.ToolCalls {
					_ = al.stateManager.RecordToolCallEmitted()
				}
			}
		} else {
			consecutiveNoToolCallTurns++
		}

		threshold := 3
		if al.config != nil && al.config.MaxConsecutiveFail > 0 {
			threshold = al.config.MaxConsecutiveFail
		}

		if consecutiveNoToolCallTurns >= threshold && taskRequiresFiles(intent) {
			if al.stateManager != nil {
				_ = al.stateManager.SetCircuitBreakerTriggered(true)
			}
			al.handler.OnMessage("system", fmt.Sprintf("⚠️ [CIRCUIT_BREAKER] Abortando execução: O modelo executou %d turnos sem chamadas de ferramentas em uma tarefa que requer criação/edição de arquivos.", consecutiveNoToolCallTurns))
			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "error",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"error": loop.AgentError{
						Code:    loop.ErrToolExecution,
						Message: fmt.Sprintf("circuit breaker triggered: model is unable to use tools after %d turns", consecutiveNoToolCallTurns),
						Details: map[string]interface{}{"consecutive_no_tool_calls": consecutiveNoToolCallTurns},
					},
				},
			})
			al.handler.OnStatusChange("idle")
			if al.stateManager != nil {
				_ = al.stateManager.SaveIterationLog(i+1, iterLog)
			}
			return fmt.Errorf("abortando: o modelo executou %d turnos sem ações em tarefa que requer arquivos", consecutiveNoToolCallTurns)
		}

		messages = append(messages, msg)
		saveMsgs(messages)

		// Atualiza o plano a partir do conteúdo textual da mensagem do assistente
		disablePlanCache := false
		if al.config != nil {
			disablePlanCache = al.config.DisablePlanCacheProtection
		}
		loop.UpdatePlannerFromMessageWithConfig(al.stateManager, msg.Content, disablePlanCache)

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
				if !al.config.ConsecutiveFailureRetry || (al.config.ConsecutiveFailureRetryLimit > 0 && consecutiveRetryCount >= al.config.ConsecutiveFailureRetryLimit) {
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

				consecutiveRetryCount++
				limitStr := "infinito"
				if al.config.ConsecutiveFailureRetryLimit > 0 {
					limitStr = fmt.Sprintf("%d", al.config.ConsecutiveFailureRetryLimit)
				}
				al.handler.OnMessage("system", fmt.Sprintf("Atingido limite de %d falhas consecutivas (LLM vazio). Aguardando %v antes de tentar novamente (retry %d/%s, cancele para interromper)...", al.config.MaxConsecutiveFail, al.failureRetryDelay, consecutiveRetryCount, limitStr))
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "retry",
					Iteration: i + 1,
					Data:      map[string]interface{}{"reason": "consecutive_failures", "delay": al.failureRetryDelay.String(), "error_type": "empty_llm_response", "retry_count": consecutiveRetryCount},
				})
				if al.stateManager != nil {
					_ = al.stateManager.SaveIterationLog(i+1, iterLog)
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
				case <-time.After(al.failureRetryDelay):
				}
				i--
				consecutiveFailures = al.config.MaxConsecutiveFail - 1
				lastIterFailed = iterationHasFailure
				lastToolWasValidation = false
				continue
			}
			if al.stateManager != nil {
				_ = al.stateManager.SaveIterationLog(i+1, iterLog)
			}
			lastIterFailed = iterationHasFailure
			lastToolWasValidation = false
			continue
		} else if len(msg.ToolCalls) == 0 {
			// Scanner de alucinações: detectar menções a ferramentas no texto sem tool calls
			if hallucinatedTools := detectHallucinatedToolCalls(msg.Content, al.tools); len(hallucinatedTools) > 0 {
				warning := fmt.Sprintf("⚠️ [INVALID_TOOL_CALL_FORMAT] Você mencionou as ferramentas %s no texto, mas não emitiu chamadas de ferramenta JSON/XML estruturadas. Emita as chamadas corretamente.",
					strings.Join(hallucinatedTools, ", "))
				al.handler.OnMessage("system", warning)
				messages = append(messages, llm.Message{Role: "system", Content: warning})
				saveMsgs(messages)
				lastIterFailed = false
				lastToolWasValidation = false
				if al.stateManager != nil {
					_ = al.stateManager.SaveIterationLog(i+1, iterLog)
				}
				continue
			}

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

			// Se o plano não estiver vazio e houver tarefas pendentes/em andamento,
			// avisa o agente para continuar executando até terminar.
			hasPendingTasks := false
			if al.stateManager != nil {
				plan := al.stateManager.GetPlan()
				for _, item := range plan {
					if item.Status == "pending" || item.Status == "in_progress" {
						hasPendingTasks = true
						break
					}
				}
				if hasPendingTasks {
					warning := loop.GeneratePendingTasksWarning(plan)
					al.handler.OnMessage("system", "Aviso de tarefas pendentes no plano. Solicitando continuação.")
					messages = append(messages, llm.Message{
						Role:    "system",
						Content: warning,
					})
					saveMsgs(messages)
					lastIterFailed = false
					lastToolWasValidation = false
					if al.stateManager != nil {
						_ = al.stateManager.SaveIterationLog(i+1, iterLog)
					}
					continue
				}
			}

			// Verificação física de arquivos planejados
			expectedFiles := loop.ParseExpectedFiles(messages)
			if len(expectedFiles) > 0 && workspaceDir != "" {
				missingFiles := loop.VerifyExpectedFiles(expectedFiles, workspaceDir)
				if len(missingFiles) > 0 {
					warning := fmt.Sprintf("⚠️ [PHYSICAL_FILE_MISSING] Os seguintes arquivos planejados não existem no disco:\n%s\nCrie os arquivos ausentes antes de encerrar.", strings.Join(missingFiles, "\n"))
					al.handler.OnMessage("system", warning)
					messages = append(messages, llm.Message{Role: "system", Content: warning})
					saveMsgs(messages)
					lastIterFailed = false
					lastToolWasValidation = false
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

			// Interceptar se for um subagente especialista adaptado
			var isAgent bool
			var priorSummary string
			var rawArgs = tc.Function.Arguments
			if _, ok := tool.(*tools.AgentToolAdapter); ok {
				isAgent = true
				if al.stateManager != nil {
					priorSummary = al.stateManager.GetSummaryForAgent(toolID)
				}
				// Injeta o prior_summary se não fornecido pelo LLM
				var argsMap map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err == nil {
					if _, ok := argsMap["prior_summary"]; !ok || argsMap["prior_summary"] == "" {
						argsMap["prior_summary"] = priorSummary
						if newArgs, errMarshal := json.Marshal(argsMap); errMarshal == nil {
							rawArgs = string(newArgs)
						}
					}
				}
			}

			// Executar com timeout
			toolStartTime := time.Now()
			toolCtx, cancel := context.WithTimeout(ctx, time.Duration(al.config.ToolTimeoutSeconds)*time.Second)
			result, execErr := tool.Execute(toolCtx, json.RawMessage(rawArgs))
			cancel()
			toolDuration := time.Since(toolStartTime).Milliseconds()

			if execErr != nil {
				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       rawArgs,
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
				// Validação pós-criação/edição de arquivos (Fase 7)
				if toolID == "write_file" || toolID == "diff_replace" {
					var argsPath struct {
						Path string `json:"path"`
					}
					if err := json.Unmarshal([]byte(rawArgs), &argsPath); err == nil && argsPath.Path != "" {
						filePath := argsPath.Path
						if !filepath.IsAbs(filePath) && workspaceDir != "" {
							filePath = filepath.Join(workspaceDir, filePath)
						}

						if al.stateManager != nil {
							_ = al.stateManager.RecordFileValidated()
						}
						valid, errMsg := loop.ValidateCreatedFile(filePath, "")
						if !valid {
							feedbackMsg := fmt.Sprintf("⚠️ [VALIDATION_ERROR]: O arquivo %s contém erros de sintaxe/compilação:\n%s\nPor favor, corrija os erros identificados.", argsPath.Path, errMsg)

							iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
								ToolName:   toolID,
								Args:       rawArgs,
								Success:    false,
								Output:     feedbackMsg,
								DurationMs: toolDuration,
							})

							messages = append(messages, llm.Message{
								Role:       "tool",
								ToolCallID: tc.ID,
								Name:       toolID,
								Content:    feedbackMsg,
							})

							al.handler.OnMessage("system", fmt.Sprintf("Validação falhou para %s: %s", argsPath.Path, errMsg))

							al.handler.OnEvent(loop.AgentEvent{
								Timestamp: time.Now(),
								Event:     "tool_result",
								Iteration: i + 1,
								Data: map[string]interface{}{
									"tool_call_id": tc.ID,
									"tool":         toolID,
									"success":      false,
									"error":        feedbackMsg,
								},
							})

							iterationHasFailure = true
							continue
						}
						if al.stateManager != nil {
							_ = al.stateManager.RecordFileCreated()
						}
					}
				}

				if isAgent {
					var agentRes struct {
						Output         string `json:"output"`
						ContextSummary string `json:"context_summary"`
					}
					if err := json.Unmarshal([]byte(result.Data), &agentRes); err == nil {
						if al.stateManager != nil {
							_ = al.stateManager.UpdateSummaryForAgent(toolID, agentRes.ContextSummary)
						}
						// Oculta a estrutura interna de ContextSummary do prompt do Supervisor
						result.Data = agentRes.Output
					}
				}

				iterLog.ToolsCalled = append(iterLog.ToolsCalled, state.ToolTrace{
					ToolName:   toolID,
					Args:       rawArgs,
					Success:    true,
					Output:     result.Data,
					DurationMs: toolDuration,
				})
				if toolID == "schedule_timer" {
					timerScheduled = true
				}
				if toolID == "ask_user" {
					askUserCalled = true
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

		if askUserCalled {
			al.handler.OnMessage("system", "Pergunta enviada ao usuário. Suspendendo execução para aguardar resposta no chat.")
			if al.stateManager != nil {
				_ = al.stateManager.SetStatus("waiting_user_input")
				_ = al.stateManager.AddLog("Suspenso aguardando resposta do usuário")
			}
			al.handler.OnEvent(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "finished",
				Iteration: i + 1,
				Data:      map[string]interface{}{"reason": "waiting_user_input", "total_iterations": i + 1},
			})
			al.handler.OnStatusChange("waiting_user_input")
			return nil
		}

		if iterationHasFailure {
			consecutiveFailures++
			if consecutiveFailures >= al.config.MaxConsecutiveFail {
				if !al.config.ConsecutiveFailureRetry || (al.config.ConsecutiveFailureRetryLimit > 0 && consecutiveRetryCount >= al.config.ConsecutiveFailureRetryLimit) {
					al.handler.OnMessage("system", i18n.Get("errors.abort_consecutive_failures", al.config.MaxConsecutiveFail))
					al.handler.OnEvent(loop.AgentEvent{
						Timestamp: time.Now(),
						Event:     "finished",
						Iteration: i + 1,
						Data:      map[string]interface{}{"reason": "consecutive_failures", "total_iterations": i + 1},
					})
					return fmt.Errorf("abortando: %d falhas consecutivas", al.config.MaxConsecutiveFail)
				}

				consecutiveRetryCount++
				limitStr := "infinito"
				if al.config.ConsecutiveFailureRetryLimit > 0 {
					limitStr = fmt.Sprintf("%d", al.config.ConsecutiveFailureRetryLimit)
				}
				al.handler.OnMessage("system", fmt.Sprintf("Atingido limite de %d falhas consecutivas (execução). Aguardando %v antes de tentar novamente (retry %d/%s, cancele para interromper)...", al.config.MaxConsecutiveFail, al.failureRetryDelay, consecutiveRetryCount, limitStr))
				al.handler.OnEvent(loop.AgentEvent{
					Timestamp: time.Now(),
					Event:     "retry",
					Iteration: i + 1,
					Data:      map[string]interface{}{"reason": "consecutive_failures", "delay": al.failureRetryDelay.String(), "error_type": "tool_failure", "retry_count": consecutiveRetryCount},
				})
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
				case <-time.After(al.failureRetryDelay):
				}
				i--
				consecutiveFailures = al.config.MaxConsecutiveFail - 1
			}
		} else {
			consecutiveFailures = 0
			consecutiveRetryCount = 0
		}

		lastIterFailed = iterationHasFailure
		lastToolWasValidation = false
		if len(msg.ToolCalls) > 0 {
			lastTc := msg.ToolCalls[len(msg.ToolCalls)-1]
			lastToolWasValidation = isValidationAction(lastTc.Function.Name, lastTc.Function.Arguments)
		}
	}
}

// truncateStr trunca uma string
func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// FormatMessagesForModel formata as mensagens para o LLM. Se o provedor não suportar
// System Prompt, todas as mensagens de role "system" serão mescladas no início da primeira
// mensagem de role "user".
func FormatMessagesForModel(messages []llm.Message, provider llm.Provider) []llm.Message {
	if provider.SupportsSystemPrompt() {
		return messages
	}

	var systemContents []string
	var otherMessages []llm.Message

	for _, msg := range messages {
		if msg.Role == "system" {
			if msg.Content != "" {
				systemContents = append(systemContents, msg.Content)
			}
		} else {
			otherMessages = append(otherMessages, msg)
		}
	}

	if len(systemContents) > 0 && len(otherMessages) > 0 {
		// Encontra a primeira mensagem user para mesclar
		for i, msg := range otherMessages {
			if msg.Role == "user" {
				// Mescla os conteúdos
				instructions := strings.Join(systemContents, "\n\n")
				merged := fmt.Sprintf("=== INSTRUÇÕES DO SISTEMA ===\n%s\n=============================\n\n%s", instructions, msg.Content)
				otherMessages[i].Content = merged
				break
			}
		}
	}

	return otherMessages
}

func isValidationAction(toolID string, rawArgs string) bool {
	if toolID == "terminal_command" {
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err == nil {
			cmd := strings.ToLower(args.Command)
			for _, word := range []string{"test", "lint", "vet", "compile", "build"} {
				if strings.Contains(cmd, word) {
					return true
				}
			}
		}
	}
	if strings.Contains(toolID, "test") || strings.Contains(toolID, "lint") || strings.Contains(toolID, "vet") {
		return true
	}
	return false
}

func isSimpleIntent(intent string) bool {
	clean := strings.TrimSpace(strings.ToLower(intent))
	clean = strings.TrimRight(clean, ".!?")
	greetings := []string{
		"oi", "olá", "ola", "bom dia", "boa tarde", "boa noite", "hello", "hi", "test", "teste",
		"hey", "opa", "tudo bem", "tudo bem?", "como vai?", "tchau", "bye", "flw",
		"good morning", "good afternoon", "good evening",
	}
	for _, g := range greetings {
		if clean == g {
			return true
		}
	}
	replies := []string{
		"sim", "não", "nao", "yes", "no", "ok", "confirmar", "confirma", "cancelar", "cancela", "fechar", "rejeitar", "aprovar", "aprovado", "rejeitado", "obrigado", "obrigada", "valeu", "thanks",
		"start", "stop", "iniciar", "parar", "status", "ready", "pronto", "go",
	}
	for _, r := range replies {
		if clean == r {
			return true
		}
	}
	return false
}

func (al *AgenticLoop) generateSimpleResponse(ctx context.Context, intent string) (string, error) {
	clean := strings.TrimSpace(strings.ToLower(intent))

	// 10.3. Verificar cache local com TTL
	al.fastPathCacheMu.Lock()
	entry, found := al.fastPathCache[clean]
	al.fastPathCacheMu.Unlock()

	if found && time.Now().Before(entry.expiresAt) {
		return entry.response, nil
	}

	// 10.4. Definir timeout curto específico de 5 segundos
	fastCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	prompt := []llm.Message{
		{
			Role:    "system",
			Content: "Você é um Agente, um assistente de IA. Responda à saudação, agradecimento ou resposta curta do usuário de forma extremamente amigável, natural e muito curta (máximo 1 frase).",
		},
		{
			Role:    "user",
			Content: intent,
		},
	}
	resp, err := al.provider.SendMessages(fastCtx, prompt, llm.RequestOptions{})
	if err != nil {
		return "", err
	}
	if al.stateManager != nil {
		_ = al.stateManager.RecordTokens(resp.Usage.TotalTokens)
	}

	resContent := resp.Message.Content

	// Salvar no cache com TTL de 5 minutos
	al.fastPathCacheMu.Lock()
	al.fastPathCache[clean] = fastPathCacheEntry{
		response:  resContent,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	al.fastPathCacheMu.Unlock()

	return resContent, nil
}

func detectHallucinatedToolCalls(content string, toolsMap map[string]tools.Tool) []string {
	if content == "" {
		return nil
	}

	// Remover blocos de código para evitar falsos positivos em comentários legítimos
	cleanedContent := stripCodeBlocks(content)

	var found []string
	seen := make(map[string]bool)

	for name := range toolsMap {
		if seen[name] {
			continue
		}

		// Padrões diretos (legado)
		directPatterns := []string{
			name + "(",
			name + " {",
			name + ": {",
			"tool_call: " + name,
			"tool: " + name,
			"call: " + name,
		}
		for _, pat := range directPatterns {
			if strings.Contains(cleanedContent, pat) {
				found = append(found, name)
				seen[name] = true
				break
			}
		}
		if seen[name] {
			continue
		}

		// Padrões narrativos (modelos 3B/8B frequentemente emitem esses formatos)
		lowerContent := strings.ToLower(cleanedContent)
		lowerName := strings.ToLower(name)
		narrativePatterns := []string{
			"[chamando " + lowerName + "]",
			"[chamando ferramenta " + lowerName + "]",
			"[calling " + lowerName + "]",
			"[calling tool " + lowerName + "]",
			"executar " + lowerName,
			"execute " + lowerName,
			"usar ferramenta " + lowerName,
			"using tool " + lowerName,
			"invocar " + lowerName,
			"invoke " + lowerName,
			"chamar " + lowerName,
			"vou usar " + lowerName,
			"vou chamar " + lowerName,
			"i'll call " + lowerName,
			"i will call " + lowerName,
			"running " + lowerName,
			"executando " + lowerName,
		}
		for _, pat := range narrativePatterns {
			if strings.Contains(lowerContent, pat) {
				found = append(found, name)
				seen[name] = true
				break
			}
		}
		if seen[name] {
			continue
		}

		// Padrão de JSON inline não-estruturado: {"tool": "name", ...} ou {"name": "tool_name", ...}
		jsonPatterns := []string{
			`"tool": "` + name + `"`,
			`"name": "` + name + `"`,
			`"function": "` + name + `"`,
			`"tool_name": "` + name + `"`,
			`"action": "` + name + `"`,
		}
		for _, pat := range jsonPatterns {
			if strings.Contains(cleanedContent, pat) {
				found = append(found, name)
				seen[name] = true
				break
			}
		}
	}
	return found
}

// stripCodeBlocks remove blocos de código markdown e /tool_code do conteúdo para que
// menções a ferramentas dentro de código legítimo não sejam tratadas como alucinações.
func stripCodeBlocks(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false
	inToolCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detectar blocos /tool_code
		if trimmed == "/tool_code" {
			inToolCode = !inToolCode
			continue
		}
		if inToolCode {
			continue
		}

		// Detectar blocos de código markdown
		if strings.HasPrefix(trimmed, "```") {
			inBlock = !inBlock
			continue
		}
		if inBlock {
			continue
		}

		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func taskRequiresFiles(intent string) bool {
	lower := strings.ToLower(intent)
	keywords := []string{
		"crie", "salve", "escreva", "código", "arquivo", "create", "write", "save", "code", "file", "organize", "generat", "gerar",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
