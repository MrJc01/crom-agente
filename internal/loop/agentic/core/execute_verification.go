package core

import (
	"context"
	"fmt"
	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/state"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runAutoTests(workspaceDir string) (bool, string) {
	if workspaceDir == "" {
		return true, ""
	}

	// 1. Detectar se é projeto Go (existe go.mod)
	goMod := filepath.Join(workspaceDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		if _, err := exec.LookPath("go"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "test", "./...")
			cmd.Dir = workspaceDir
			out, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf(i18n.Get("verification.go_test_failed"), string(out))
			}
		}
	}

	// 2. Detectar se existem arquivos Python modificados/criados ou se é projeto Python
	files, err := os.ReadDir(workspaceDir)
	if err != nil {
		return true, ""
	}

	var pyFiles []string
	var testFiles []string
	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			if strings.HasSuffix(name, ".py") {
				if strings.Contains(name, "test") {
					testFiles = append(testFiles, name)
				} else {
					pyFiles = append(pyFiles, name)
				}
			}
		}
	}

	// Se tiver arquivos de teste explícitos (ex: test_solucao.py), rodar com python3
	if len(testFiles) > 0 {
		if _, err := exec.LookPath("python3"); err == nil {
			for _, tf := range testFiles {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "python3", tf)
				cmd.Dir = workspaceDir
				out, err := cmd.CombinedOutput()
				if err != nil {
					return false, fmt.Sprintf("Python unit test '%s' failed:\n%s", tf, string(out))
				}
			}
		}
	}

	// Se tiver arquivos Python regulares, rodar doctest neles se contiverem doctests
	if _, err := exec.LookPath("python3"); err == nil {
		for _, pf := range pyFiles {
			// Ler arquivo para ver se contém ">>>" indicando doctests
			content, errRead := os.ReadFile(filepath.Join(workspaceDir, pf))
			if errRead == nil && strings.Contains(string(content), ">>>") {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				// python3 -m doctest pf
				cmd := exec.CommandContext(ctx, "python3", "-m", "doctest", pf)
				cmd.Dir = workspaceDir
				out, err := cmd.CombinedOutput()
				if err != nil || strings.Contains(string(out), "Failed") {
					return false, fmt.Sprintf("Python doctest in '%s' failed:\n%s", pf, string(out))
				}
			}
		}
	}

	return true, ""
}

func runAutoFormatter(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		if _, err := exec.LookPath("gofmt"); err == nil {
			cmd := exec.Command("gofmt", "-w", path)
			_ = cmd.Run()
		}
	case ".py":
		if _, err := exec.LookPath("black"); err == nil {
			cmd := exec.Command("black", path)
			_ = cmd.Run()
		} else if _, err := exec.LookPath("ruff"); err == nil {
			cmd := exec.Command("ruff", "format", path)
			_ = cmd.Run()
		}
	}
}

func isCompletionResponse(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "tarefa concluída") ||
		strings.Contains(lower, "task is complete") ||
		strings.Contains(lower, "concluí a tarefa") ||
		strings.Contains(lower, "i have completed the task") ||
		strings.Contains(lower, "tudo pronto") ||
		strings.Contains(lower, "finalizei as alterações") ||
		strings.Contains(lower, "com sucesso") ||
		strings.Contains(lower, "foi concluída") ||
		strings.Contains(lower, "foi adicionada") ||
		strings.Contains(lower, "estão completos") ||
		strings.Contains(lower, "foi executad") ||
		strings.Contains(lower, "já concluí") ||
		strings.Contains(lower, "concluído")
}

// GetLastIterationExecutionStatus analisa as últimas mensagens para ver o status da última execução de ferramenta
func (al *AgenticLoop) GetLastIterationExecutionStatus(messages []llm.Message) string {
	// Acha o índice da última mensagem do assistant
	lastAssistantIdx := -1
	for j := len(messages) - 1; j >= 0; j-- {
		if messages[j].Role == "assistant" {
			lastAssistantIdx = j
			break
		}
	}

	if lastAssistantIdx == -1 {
		return "📋 [STATUS DA ÚLTIMA EXECUÇÃO DE FERRAMENTAS]: Nenhuma ferramenta foi executada ainda."
	}

	// Procura ferramentas executadas em resposta a essa última mensagem do assistant
	var executedTools []string
	var failedTools []string

	for j := lastAssistantIdx + 1; j < len(messages); j++ {
		if messages[j].Role == "tool" {
			name := messages[j].Name
			if strings.HasPrefix(messages[j].Content, "⚠️ Erro") || strings.HasPrefix(messages[j].Content, "Error:") {
				failedTools = append(failedTools, name)
			} else {
				executedTools = append(executedTools, name)
			}
		}
	}

	if len(executedTools) > 0 || len(failedTools) > 0 {
		var parts []string
		if len(executedTools) > 0 {
			parts = append(parts, fmt.Sprintf(i18n.Get("verification.exec_success"), strings.Join(executedTools, ", ")))
		}
		if len(failedTools) > 0 {
			parts = append(parts, fmt.Sprintf(i18n.Get("verification.exec_failed"), strings.Join(failedTools, ", ")))
		}
		return fmt.Sprintf(getPromptText(al, "system_last_tool_status", "📋 [STATUS DA ÚLTIMA EXECUÇÃO DE FERRAMENTAS]: Na última iteração, você %s."), strings.Join(parts, " e "))
	}

	// Se não tem mensagens de tool executadas, mas o assistente tinha ToolCalls na sua mensagem
	astMsg := messages[lastAssistantIdx]
	if len(astMsg.ToolCalls) > 0 {
		var toolNames []string
		for _, tc := range astMsg.ToolCalls {
			toolNames = append(toolNames, tc.Function.Name)
		}
		return fmt.Sprintf(getPromptText(al, "system_last_tool_missing", "⚠️ [STATUS DA ÚLTIMA EXECUÇÃO DE FERRAMENTAS]: Você solicitou a execução de %s, mas NENHUMA ferramenta foi executada (talvez porque a chamada continha argumentos inválidos, ou foi recusada)."), strings.Join(toolNames, ", "))
	}

	// Se não tinha ToolCalls, mas o texto contém padrões de tentativa de chamada de ferramenta
	lowerContent := strings.ToLower(astMsg.Content)
	if strings.Contains(lowerContent, "{") || strings.Contains(lowerContent, "write_file") || strings.Contains(lowerContent, "terminal_command") || strings.Contains(lowerContent, "edit_file") {
		return "⚠️ [STATUS DA ÚLTIMA EXECUÇÃO DE FERRAMENTAS]: NENHUMA ferramenta foi executada na última iteração. Percebi que você escreveu código JSON, comandos ou chamadas de função no corpo do texto. Lembre-se de que responder com texto contendo JSON NÃO executa ferramentas no sistema do usuário. Você DEVE usar a chamada de ferramenta nativa (Tool Calling) fornecida pela API do modelo."
	}

	return "📋 [STATUS DA ÚLTIMA EXECUÇÃO DE FERRAMENTAS]: Nenhuma ferramenta foi solicitada ou executada na última iteração."
}

func (al *AgenticLoop) verifyWorkspaceState(messages *[]llm.Message, workspaceDir string, i int, iterLog state.IterationLog, lastIterFailed *bool, lastToolWasValidation *bool) bool {
	expectedFiles := loop.ParseExpectedFiles(*messages)
	if len(expectedFiles) > 0 && workspaceDir != "" {
		missingFiles := loop.VerifyExpectedFiles(expectedFiles, workspaceDir)
		if len(missingFiles) > 0 {
			warning := fmt.Sprintf(getPromptText(al, "system_physical_file_missing", "⚠️ [PHYSICAL_FILE_MISSING] Os seguintes arquivos planejados não existem no disco:\n%s\nCrie os arquivos ausentes antes de encerrar."), strings.Join(missingFiles, "\n"))
			al.handler.OnMessage("system", warning)
			*messages = append(*messages, llm.Message{Role: "system", Content: warning})
			if al.stateManager != nil {
				_ = al.stateManager.SetMessages(*messages)
				_ = al.stateManager.SaveIterationLog(i+1, iterLog)
			}
			*lastIterFailed = false
			*lastToolWasValidation = false
			return true
		}
	}

	if workspaceDir != "" && al.stateManager != nil {
		st := al.stateManager.GetState()
		if st.FilesCreated > 0 || st.FilesValidated > 0 {
			if ok, testErrMsg := runAutoTests(workspaceDir); !ok {
				warning := fmt.Sprintf(getPromptText(al, "system_test_failure", "⚠️ [TEST_FAILURE]: A execução de testes unitários ou doctests locais detectou falhas no workspace:\n%s\nPor favor, corrija os erros identificados antes de encerrar."), testErrMsg)
				al.handler.OnMessage("system", "Testes unitários ou doctests locais falharam. Solicitando correção.")
				*messages = append(*messages, llm.Message{Role: "system", Content: warning})
				if al.stateManager != nil {
					_ = al.stateManager.SetMessages(*messages)
				}
				*lastIterFailed = true
				*lastToolWasValidation = true

				plan := al.stateManager.GetPlan()
				hasCorrectionTask := false
				for _, item := range plan {
					if strings.HasPrefix(item.Title, "Corrigir falhas") || strings.Contains(strings.ToLower(item.Title), "corrigir") {
						hasCorrectionTask = true
						break
					}
				}
				if !hasCorrectionTask {
					newPlan := append(plan, state.TaskItem{
						Title:  "Corrigir falhas detectadas na suíte de testes",
						Status: "in_progress",
					})
					_ = al.stateManager.SetPlan(newPlan)
				}

				if al.stateManager != nil {
					_ = al.stateManager.SaveIterationLog(i+1, iterLog)
				}
				return true
			}
		}
	}
	return false
}
