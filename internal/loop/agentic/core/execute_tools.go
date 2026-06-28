package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/tools"
	"regexp"
	"strings"
	"time"
)

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

func (al *AgenticLoop) parseAndInterceptToolCalls(msg *llm.Message, consecutiveNoToolCallTurns int) {
	if pyToolCalls := loop.TryParseToolCode(msg.Content); len(pyToolCalls) > 0 {
		msg.ToolCalls = append(msg.ToolCalls, pyToolCalls...)
		if al.stateManager != nil {
			_ = al.stateManager.RecordToolCallsFromTextParse(len(pyToolCalls))
		}
	}

	if len(msg.ToolCalls) == 0 && (al.textOnlyMode || consecutiveNoToolCallTurns >= 2) {
		validToolsMap := make(map[string]bool)
		for name := range al.tools {
			validToolsMap[name] = true
		}
		if pyDirectCalls := loop.TryParsePythonDirectToolCalls(msg.Content, validToolsMap); len(pyDirectCalls) > 0 {
			msg.ToolCalls = append(msg.ToolCalls, pyDirectCalls...)
			if al.stateManager != nil {
				_ = al.stateManager.RecordToolCallsFromTextParse(len(pyDirectCalls))
			}
		}
	}

	if len(msg.ToolCalls) == 0 && (al.textOnlyMode || consecutiveNoToolCallTurns >= 2) {
		validToolsMap := make(map[string]bool)
		for name := range al.tools {
			validToolsMap[name] = true
		}
		if jsonStructuredCalls := loop.TryParseJSONStructuredToolCalls(msg.Content, validToolsMap); len(jsonStructuredCalls) > 0 {
			msg.ToolCalls = append(msg.ToolCalls, jsonStructuredCalls...)
			if al.stateManager != nil {
				_ = al.stateManager.RecordToolCallsFromTextParse(len(jsonStructuredCalls))
			}
		}
	}

	if al.textOnlyMode || (len(msg.ToolCalls) == 0 && consecutiveNoToolCallTurns >= 2) {
		if markdownToolCalls := loop.TryParseMarkdownToolCalls(msg.Content); len(markdownToolCalls) > 0 {
			msg.ToolCalls = append(msg.ToolCalls, markdownToolCalls...)
			if al.stateManager != nil {
				_ = al.stateManager.RecordToolCallsFromTextParse(len(markdownToolCalls))
			}
		}
	}
}

// normalizeToolCallName ajusta nomes de ferramentas legadas ou confundidas pelo LLM (ex: "screenshot")
func (al *AgenticLoop) normalizeToolCallName(tc *llm.ToolCall) string {
	toolID := tc.Function.Name
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
	return toolID
}

// prepareSubagentArgs intercepta chamadas de subagentes especialistas adaptados e injeta prior_summary
func (al *AgenticLoop) prepareSubagentArgs(tc *llm.ToolCall, tool tools.Tool, rawArgs string) (bool, string) {
	var isAgent bool
	if _, ok := tool.(*tools.AgentToolAdapter); ok {
		isAgent = true
		var priorSummary string
		if al.stateManager != nil {
			priorSummary = al.stateManager.GetSummaryForAgent(tool.ID())
		}
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
	return isAgent, rawArgs
}

// processSubagentResult atualiza o sumário de contexto retornado pelo subagente
func (al *AgenticLoop) processSubagentResult(toolID string, isAgent bool, resultData string) string {
	if isAgent {
		var agentRes struct {
			Output         string `json:"output"`
			ContextSummary string `json:"context_summary"`
		}
		if err := json.Unmarshal([]byte(resultData), &agentRes); err == nil {
			if al.stateManager != nil {
				_ = al.stateManager.UpdateSummaryForAgent(toolID, agentRes.ContextSummary)
			}
			return agentRes.Output
		}
	}
	return resultData
}

func DetectCommandLoop(messages []llm.Message) bool {
	type cmdTrace struct {
		cmd string
		out string
	}
	var traces []cmdTrace
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == "terminal_command" || tc.Function.Name == "run_command" {
					var out string
					for j := i + 1; j < len(messages); j++ {
						if messages[j].Role == "tool" && messages[j].ToolCallID == tc.ID {
							out = messages[j].Content
							break
						}
					}
					traces = append(traces, cmdTrace{
						cmd: tc.Function.Arguments,
						out: out,
					})
				}
			}
		}
	}
	if len(traces) < 3 {
		return false
	}
	if traces[0].cmd == traces[1].cmd && traces[0].out == traces[1].out &&
		traces[1].cmd == traces[2].cmd && traces[1].out == traces[2].out {
		return true
	}
	return false
}

func countConsecutiveEmptyResponses(messages []llm.Message) int {
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" {
			if m.Content == "" && len(m.ToolCalls) == 0 {
				count++
			} else {
				break
			}
		}
	}
	return count
}

func countConsecutiveReadOnlyTurns(messages []llm.Message) int {
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" {
			hasWriteOrExec := false
			for _, tc := range m.ToolCalls {
				name := tc.Function.Name
				if name == "write_file" || name == "edit_file" || name == "terminal_command" || name == "run_command" {
					hasWriteOrExec = true
					break
				}
			}
			if !hasWriteOrExec {
				count++
			} else {
				break
			}
		}
	}
	return count
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

// isConversationalResponse detecta se a resposta do modelo é uma conversa simples
// (saudação, agradecimento, etc.) que não deveria ter gerado um plano de tarefas.
// Isso evita que o loop de auto-continuação rode infinitamente quando o modelo
// gera checkboxes desnecessárias para respostas conversacionais.
func isConversationalResponse(response string, originalIntent string) bool {
	// Se o intent original era simples, qualquer resposta sem tool calls é conversacional
	if isSimpleIntent(originalIntent) {
		return true
	}

	// Verificar se a resposta é curta e parece conversacional
	clean := strings.TrimSpace(response)
	// Remove checkboxes markdown para avaliar o conteúdo real
	lines := strings.Split(clean, "\n")
	var contentLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Pula linhas de checkbox
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [/]") {
			continue
		}
		if trimmed != "" {
			contentLines = append(contentLines, trimmed)
		}
	}

	// Se o conteúdo real (sem checkboxes) é curto e tem padrões de conversa
	realContent := strings.Join(contentLines, " ")
	if len(contentLines) <= 3 && len(realContent) < 200 {
		conversationalPatterns := []string{
			"como posso ajudar", "como posso te ajudar", "em que posso ajudar",
			"olá", "oi!", "tudo bem", "como vai", "prazer",
			"hello", "hi!", "how can i help", "how may i help",
			"obrigado", "de nada", "até logo",
		}
		lowerContent := strings.ToLower(realContent)
		for _, p := range conversationalPatterns {
			if strings.Contains(lowerContent, p) {
				return true
			}
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

	// 10.4. Definir timeout para resposta rápida (15s para acomodar latência de gateways remotos + retries)
	fastCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
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
	al.recordCostForResponse(resp)

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

func (al *AgenticLoop) taskRequiresFiles(intent string) bool {
	lower := strings.ToLower(intent)
	keywords := []string{
		"crie", "salve", "escreva", "código", "arquivo", "create", "write", "save", "code", "file", "organize", "generat", "gerar",
	}
	if al != nil && al.promptManager != nil {
		if pm, ok := al.promptManager.GetPrompt("file_creation_keywords"); ok && pm.Enabled && pm.Content != "" {
			keywords = strings.Split(pm.Content, ",")
		}
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func extractEntryPointFromPrompt(intent string) string {
	// Padrão 1: entry_point: has_close_elements ou entry_point = 'has_close_elements' ou similar
	re1 := regexp.MustCompile(`(?i)entry_point["'\s:=]+([\w_]+)`)
	matches1 := re1.FindStringSubmatch(intent)
	if len(matches1) > 1 {
		return matches1[1]
	}

	// Padrão 2: "def target_func(" ou "def target_func ("
	re2 := regexp.MustCompile(`def\s+([\w_]+)\s*\(`)
	matches2 := re2.FindStringSubmatch(intent)
	if len(matches2) > 1 {
		name := matches2[1]
		if name != "check" && name != "candidate" && name != "solve" {
			return name
		}
	}
	return ""
}

// extractTargetFromArgs extrai o alvo (path, command) dos argumentos JSON de uma chamada de ferramenta
func extractTargetFromArgs(rawArgs string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "unknown"
	}
	// Tentar campos comuns
	for _, key := range []string{"path", "file", "command", "query", "url"} {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				if len(s) > 100 {
					return s[:100]
				}
				return s
			}
		}
	}
	return "unknown"
}

func truncateTraceback(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 20 {
		return s
	}
	lower := strings.ToLower(s)
	isError := strings.Contains(lower, "traceback") || strings.Contains(lower, "error") || strings.Contains(lower, "fail") || strings.Contains(lower, "exception") || strings.Contains(lower, "undefined")
	if isError {
		firstFive := lines[:5]
		lastTen := lines[len(lines)-10:]
		return strings.Join(firstFive, "\n") + "\n\n... [TRUNCATED LOGS / TRACEBACKS FOR CONTEXT SIZE (Item 46)] ...\n\n" + strings.Join(lastTen, "\n")
	}
	return s
}

func (al *AgenticLoop) recordCostForResponse(resp *llm.Response) {
	if al.stateManager == nil || resp == nil {
		return
	}
	model := strings.ToLower(al.config.Model)
	var promptPriceUSD, completionPriceUSD float64
	switch {
	case strings.Contains(model, "gpt-4o-mini"):
		promptPriceUSD = 0.150
		completionPriceUSD = 0.600
	case strings.Contains(model, "gpt-4o"):
		promptPriceUSD = 5.00
		completionPriceUSD = 15.00
	case strings.Contains(model, "claude-3-5-sonnet") || strings.Contains(model, "sonnet"):
		promptPriceUSD = 3.00
		completionPriceUSD = 15.00
	case strings.Contains(model, "gemini-2.5-pro") || strings.Contains(model, "pro"):
		promptPriceUSD = 1.25
		completionPriceUSD = 5.00
	case strings.Contains(model, "gemini-2.5-flash") || strings.Contains(model, "flash"):
		promptPriceUSD = 0.075
		completionPriceUSD = 0.30
	default:
		promptPriceUSD = 5.00
		completionPriceUSD = 15.00
	}
	promptTokens := resp.Usage.PromptTokens
	completionTokens := resp.Usage.CompletionTokens
	if promptTokens == 0 && completionTokens == 0 {
		promptTokens = int(float64(resp.Usage.TotalTokens) * 0.8)
		completionTokens = resp.Usage.TotalTokens - promptTokens
	}
	costUSD := (float64(promptTokens)/1000000.0)*promptPriceUSD + (float64(completionTokens)/1000000.0)*completionPriceUSD
	_ = al.stateManager.RecordCost(costUSD)
}
