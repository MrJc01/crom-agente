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
	if len(messages) <= 2 && workspaceDir != "" {
		// 0.5. Identidade Agêntica (Lembrar o modelo de suas ferramentas e capacidade de alterar o sistema)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM AGENTIC IDENTITY] Você é um agente autônomo de IA com acesso completo ao sistema do usuário. Você pode ler/escrever arquivos locais (usando 'read_file', 'write_file', 'diff_replace'), executar comandos de terminal ('terminal_command'), e navegar na internet, realizar ações complexas ou tirar prints de websites usando o navegador ('browser_action' ou 'browser_subagent'). Você não necessita que o diretório atual seja um repositório Git para funcionar — trabalhe normalmente mesmo sem Git. Você NUNCA deve alegar ao usuário que é apenas um modelo de linguagem e que não pode criar/editar arquivos, rodar comandos, acessar a internet ou tirar screenshots. Use as ferramentas disponíveis imediatamente para executar o pedido do usuário.",
		})

		// 1. Detectar stack técnica
		stack := al.detectStack(workspaceDir)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: fmt.Sprintf("[SYSTEM STACK DETECTED] A stack técnica deste projeto foi identificada como: %s. Priorize comandos e validações desta stack.", stack),
		})

		// 1.5. Instrução de Conflito de Portas (Address already in use)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM PORT CONFLICT HANDLING] Se você tentar iniciar um servidor (ex: npm run dev, go run, etc.) e ele falhar com um erro como 'Address already in use', 'port already in use', ou 'listen tcp :8080: bind: address already in use', você deve identificar imediatamente a porta conflitante, tentar utilizar uma porta alternativa (ex: 8081, 8082, 3001, etc.) ou configurar a porta via variáveis de ambiente/parâmetros do comando, e continuar a execução sem desistir.",
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
			Content: "[SYSTEM PLANNING REQUIREMENT] Se a tarefa solicitada pelo usuário for complexa ou envolver múltiplos passos, você deve descrever e listar um plano de execução detalhado em formato de checklist markdown no início de sua resposta. Use o formato:\n- [ ] Nome da tarefa\n\nÀ medida que progredir, atualize o status das tarefas:\n- [/] Tarefa em andamento\n- [x] Tarefa concluída\n\nVocê deve sempre incluir o plano de trabalho atualizado no início de seu conteúdo de texto (content) em todas as respostas (mesmo quando estiver chamando ferramentas) para que o usuário possa acompanhar o progresso de forma estruturada. Se houver dúvidas cruciais, ambiguidades técnicas ou decisões de design arquitetural importantes, identifique-as e liste-as sob uma seção clara chamada '**Questões de Alinhamento / Clarificações**' no início de sua resposta. Se a tarefa for simples (como uma saudação 'oi' ou conversa rápida), responda diretamente e de forma amigável sem criar um plano.",
		})

		// 3.5. Exigência de Uso de Ferramentas para Escrita de Arquivos (Evitar responder apenas com markdown)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM TOOL USAGE REQUIREMENT] IMPORTANTE: Responder apenas com blocos de código markdown no chat NÃO cria, altera ou escreve arquivos no workspace do usuário. Se o seu plano envolve criar, editar ou excluir arquivos, você deve OBRIGATORIAMENTE chamar as ferramentas apropriadas (como 'write_file', 'diff_replace', etc.) para realizar essas ações no disco. Nunca marque uma tarefa de criação/modificação de código como concluída (- [x]) a menos que você tenha efetivamente executado a chamada de ferramenta correspondente com sucesso.",
		})

		// 3.5.5. Planejamento de Impacto de Arquivos (Proposed Changes)
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM FILE IMPACT PLANNING] Antes de realizar modificações ou criar novos arquivos no disco, você deve descrever um plano de impacto de arquivos contendo uma seção 'Proposed Changes' listando explicitamente os arquivos que serão criados (NEW), modificados (MODIFY) ou excluídos (DELETE).",
		})

		// 3.6. Exigência de Uso do parâmetro 'path' para Capturas de Tela
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: "[SYSTEM SCREENSHOT PATH REQUIREMENT] IMPORTANTE: Ao tirar capturas de tela (screenshots) usando a ferramenta 'browser_action' (com o parâmetro 'action' definido como 'screenshot') ou a ferramenta 'computer_control' (com o parâmetro 'action' definido como 'screenshot'), você deve OBRIGATORIAMENTE fornecer o parâmetro 'path' (ex: 'screenshot.png') se o usuário solicitou salvar a imagem. Se você não fornecer o 'path', a imagem NÃO será salva no disco e você não conseguirá salvá-la posteriormente usando a ferramenta 'write_file'. Não tente chamar uma ferramenta com o nome 'screenshot' (ela não existe); você deve chamar 'browser_action' ou 'computer_control' com 'action' definido como 'screenshot'.",
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

		// 5. Injetar fase atual (Planning ou Execution) no contexto da sessão
		phase := GetCurrentPhase(al.stateManager)
		if phase == PhasePlanning {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "[SYSTEM PHASE: PLANNING] Esta é a fase de PLANEJAMENTO. Analise cuidadosamente o pedido, gere o checklist detalhado de tarefas necessárias usando o formato `- [ ] Tarefa` (e a seção de 'Questões de Alinhamento / Clarificações' se houver dúvidas), e comece a executar IMEDIATAMENTE o plano chamando pelo menos uma ferramenta (como 'read_file', 'list_dir', 'write_file', 'terminal_command') na mesma resposta. Você NUNCA deve retornar uma resposta puramente de texto com o plano sem invocar nenhuma ferramenta, pois isso fará com que o loop seja suspenso com erro de tarefas incompletas.",
			})
		} else {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "[SYSTEM PHASE: EXECUTION] Esta é a fase de EXECUÇÃO. O plano já foi definido. Concentre-se em executar as tarefas pendentes `[ ]` e em andamento `[/]`, atualizando o checklist com `[x]` à medida que conclui cada item. NÃO repita itens já concluídos `[x]`.",
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
				errMsg := result.Error
				if errMsg == "" && result.Data != "" {
					errMsg = result.Data
				}
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



