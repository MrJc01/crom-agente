package loop

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/llm"
)

// TryParseToolCode intercepta blocos Python do tipo `/tool_code` e os converte em chamadas estruturadas de ferramentas.
func TryParseToolCode(content string) []llm.ToolCall {
	var toolCalls []llm.ToolCall

	// Verifica se o conteúdo contém uma seção `/tool_code`
	if !strings.Contains(content, "/tool_code") {
		return nil
	}

	idxCode := strings.Index(content, "/tool_code")
	codePart := content[idxCode:]

	// Limpa o bloco de código de indicadores de markdown
	var cleanLines []string
	lines := strings.Split(codePart, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || trimmed == "/tool_code" {
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	pyCode := strings.Join(cleanLines, "\n")

	// Procura e faz o parse de chamadas `.execute(`
	searchStr := pyCode
	for {
		idx := strings.Index(searchStr, ".execute(")
		if idx == -1 {
			break
		}

		// Rastreia para trás para obter o nome da ferramenta (alfanumérico + _)
		start := idx
		for start > 0 {
			char := searchStr[start-1]
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
				start--
			} else {
				break
			}
		}
		toolName := searchStr[start:idx]

		// Encontra o fim dos argumentos pareando parênteses
		argsStart := idx + len(".execute(")
		parenCount := 1
		inSingleQuote := false
		inDoubleQuote := false
		escaped := false
		argsEnd := -1

		for j := argsStart; j < len(searchStr); j++ {
			char := searchStr[j]
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				continue
			}
			if char == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				continue
			}
			if !inSingleQuote && !inDoubleQuote {
				if char == '(' {
					parenCount++
				} else if char == ')' {
					parenCount--
					if parenCount == 0 {
						argsEnd = j
						break
					}
				}
			}
		}

		if argsEnd != -1 && toolName != "" {
			argsStr := searchStr[argsStart:argsEnd]

			// Converte os argumentos keyword para JSON
			jsonArgs, err := parsePythonKeywordArgs(argsStr)
			if err == nil {
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:   fmt.Sprintf("pycall_%d_%d", time.Now().UnixNano(), len(toolCalls)),
					Type: "function",
					Function: llm.FunctionCall{
						Name:      toolName,
						Arguments: jsonArgs,
					},
				})
			} else {
				log.Printf("[TryParseToolCode] Erro ao parsear argumentos Python: %v", err)
			}

			searchStr = searchStr[argsEnd+1:]
		} else {
			// Evita loop infinito
			searchStr = searchStr[idx+len(".execute("):]
		}
	}

	return toolCalls
}

// TryParsePythonDirectToolCalls detecta chamadas diretas no estilo Python, como `write_file(path="...", content="...")`, e as converte em chamadas estruturadas.
func TryParsePythonDirectToolCalls(content string, validTools map[string]bool) []llm.ToolCall {
	var toolCalls []llm.ToolCall

	for toolName := range validTools {
		searchPattern := toolName + "("
		searchStr := content
		for {
			idx := strings.Index(searchStr, searchPattern)
			if idx == -1 {
				break
			}
			if idx > 0 {
				prevChar := searchStr[idx-1]
				if (prevChar >= 'a' && prevChar <= 'z') || (prevChar >= 'A' && prevChar <= 'Z') || (prevChar >= '0' && prevChar <= '9') || prevChar == '_' || prevChar == '.' {
					searchStr = searchStr[idx+len(searchPattern):]
					continue
				}
			}

			argsStart := idx + len(searchPattern)
			parenCount := 1
			inSingleQuote := false
			inDoubleQuote := false
			escaped := false
			argsEnd := -1

			for j := argsStart; j < len(searchStr); j++ {
				char := searchStr[j]
				if escaped {
					escaped = false
					continue
				}
				if char == '\\' {
					escaped = true
					continue
				}
				if char == '\'' && !inDoubleQuote {
					inSingleQuote = !inSingleQuote
					continue
				}
				if char == '"' && !inSingleQuote {
					inDoubleQuote = !inDoubleQuote
					continue
				}
				if !inSingleQuote && !inDoubleQuote {
					if char == '(' {
						parenCount++
					} else if char == ')' {
						parenCount--
						if parenCount == 0 {
							argsEnd = j
							break
						}
					}
				}
			}

			if argsEnd != -1 {
				argsStr := searchStr[argsStart:argsEnd]
				jsonArgs, err := parsePythonKeywordArgs(argsStr)
				if err == nil && strings.TrimSpace(argsStr) != "" && jsonArgs == "{}" {
					err = fmt.Errorf("argumentos posicionais ou inválidos")
				}
				if err == nil {
					toolCalls = append(toolCalls, llm.ToolCall{
						ID:   fmt.Sprintf("pydirect_%d_%d", time.Now().UnixNano(), len(toolCalls)),
						Type: "function",
						Function: llm.FunctionCall{
							Name:      toolName,
							Arguments: jsonArgs,
						},
					})
					log.Printf("[TryParsePythonDirectToolCalls] Sucesso ao recuperar chamada alucinada Python: %s(%s)", toolName, jsonArgs)
				} else {
					log.Printf("[TryParsePythonDirectToolCalls] Erro ao parsear args de %s: %v", toolName, err)
				}
				searchStr = searchStr[argsEnd+1:]
			} else {
				searchStr = searchStr[idx+len(searchPattern):]
			}
		}
	}

	return toolCalls
}

// TryParseJSONStructuredToolCalls detecta e converte chamadas estruturadas JSON (estilo OpenAI/Tauri com "name" e "parameters" ou "arguments").
func TryParseJSONStructuredToolCalls(content string, validTools map[string]bool) []llm.ToolCall {
	var toolCalls []llm.ToolCall

	// Vamos procurar por blocos JSON no formato { ... }
	searchStr := content
	for {
		idx := strings.Index(searchStr, "{")
		if idx == -1 {
			break
		}

		// Procura o fechamento correspondente pareando chaves
		braceCount := 0
		inSingleQuote := false
		inDoubleQuote := false
		escaped := false
		endIdx := -1

		for j := idx; j < len(searchStr); j++ {
			char := searchStr[j]
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				continue
			}
			if char == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				continue
			}
			if !inSingleQuote && !inDoubleQuote {
				if char == '{' {
					braceCount++
				} else if char == '}' {
					braceCount--
					if braceCount == 0 {
						endIdx = j
						break
					}
				}
			}
		}

		if endIdx == -1 {
			// Sem chaves correspondentes
			searchStr = searchStr[idx+1:]
			continue
		}

		jsonStr := searchStr[idx : endIdx+1]
		searchStr = searchStr[endIdx+1:]

		// Tenta decodificar o JSON estruturado
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
			continue
		}

		// Checa se tem o campo Name/Tool/Function
		var toolName string
		if nameVal, ok := obj["name"].(string); ok {
			toolName = nameVal
		} else if toolVal, ok := obj["tool"].(string); ok {
			toolName = toolVal
		} else if funcObj, ok := obj["function"].(map[string]interface{}); ok {
			if fName, ok := funcObj["name"].(string); ok {
				toolName = fName
			}
		}

		// Se o nome não é uma ferramenta válida, ignore
		if toolName == "" || !validTools[toolName] {
			continue
		}

		// Extrai os argumentos
		var argsMap map[string]interface{}
		if params, ok := obj["parameters"].(map[string]interface{}); ok {
			argsMap = params
		} else if arguments, ok := obj["arguments"].(map[string]interface{}); ok {
			argsMap = arguments
		} else if funcObj, ok := obj["function"].(map[string]interface{}); ok {
			if fArgs, ok := funcObj["arguments"].(map[string]interface{}); ok {
				argsMap = fArgs
			} else if fArgsStr, ok := funcObj["arguments"].(string); ok {
				var parsedArgs map[string]interface{}
				if err := json.Unmarshal([]byte(fArgsStr), &parsedArgs); err == nil {
					argsMap = parsedArgs
				}
			}
		} else {
			// Se o JSON contiver diretamente os argumentos no próprio objeto sem aninhar (ex: {"name": "write_file", "path": "...", "content": "..."})
			argsMap = make(map[string]interface{})
			for k, v := range obj {
				if k != "name" && k != "tool" && k != "type" && k != "function" {
					argsMap[k] = v
				}
			}
		}

		if argsMap == nil {
			argsMap = make(map[string]interface{})
		}

		// Serializa os argumentos em string JSON
		argsBytes, err := json.Marshal(argsMap)
		if err != nil {
			continue
		}

		log.Printf("[TryParseJSONStructuredToolCalls] Sucesso ao recuperar chamada alucinada JSON: %s(%s)", toolName, string(argsBytes))
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:   fmt.Sprintf("jsoncall_%d_%d", time.Now().UnixNano(), len(toolCalls)),
			Type: "function",
			Function: llm.FunctionCall{
				Name:      toolName,
				Arguments: string(argsBytes),
			},
		})
	}

	return toolCalls
}

// parsePythonKeywordArgs converte argumentos keyword do Python "k1=v1, k2=v2" em um JSON "{\"k1\": v1, \"k2\": v2}"
func parsePythonKeywordArgs(argsStr string) (string, error) {
	argsList := splitTopLevel(argsStr, ',')
	var jsonPairs []string
	for _, arg := range argsList {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		kv := splitTopLevel(arg, '=')
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		valJSON, err := pyExprToJSON(val)
		if err != nil {
			return "", err
		}

		keyBytes, _ := json.Marshal(key)
		jsonPairs = append(jsonPairs, fmt.Sprintf("%s: %s", string(keyBytes), valJSON))
	}
	return "{" + strings.Join(jsonPairs, ", ") + "}", nil
}

// pyExprToJSON converte uma expressão literal simples do Python (strings, bools, dicts, lists, numbers) em JSON.
func pyExprToJSON(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "null", nil
	}

	// 1. Booleanos e nulos
	if expr == "True" {
		return "true", nil
	}
	if expr == "False" {
		return "false", nil
	}
	if expr == "None" {
		return "null", nil
	}

	// 2. Strings literais
	if (strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'")) ||
		(strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"")) {
		val, err := parsePythonString(expr)
		if err != nil {
			return "", err
		}
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}

	// 3. Listas
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		inner := expr[1 : len(expr)-1]
		if strings.TrimSpace(inner) == "" {
			return "[]", nil
		}
		parts := splitTopLevel(inner, ',')
		var jsonParts []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			jsonPart, err := pyExprToJSON(part)
			if err != nil {
				return "", err
			}
			jsonParts = append(jsonParts, jsonPart)
		}
		return "[" + strings.Join(jsonParts, ", ") + "]", nil
	}

	// 4. Dicionários (Dicts)
	if strings.HasPrefix(expr, "{") && strings.HasSuffix(expr, "}") {
		inner := expr[1 : len(expr)-1]
		if strings.TrimSpace(inner) == "" {
			return "{}", nil
		}
		parts := splitTopLevel(inner, ',')
		var jsonPairs []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kv := splitTopLevel(part, ':')
			if len(kv) != 2 {
				return "", fmt.Errorf("par de dicionário inválido: %s", part)
			}
			keyJSON, err := pyExprToJSON(kv[0])
			if err != nil {
				return "", err
			}
			valJSON, err := pyExprToJSON(kv[1])
			if err != nil {
				return "", err
			}
			jsonPairs = append(jsonPairs, keyJSON+": "+valJSON)
		}
		return "{" + strings.Join(jsonPairs, ", ") + "}", nil
	}

	// 5. Números
	isNumber := true
	for _, r := range expr {
		if !((r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E') {
			isNumber = false
			break
		}
	}
	if isNumber && len(expr) > 0 {
		return expr, nil
	}

	// Fallback padrão: tratar como string bruta
	jsonBytes, _ := json.Marshal(expr)
	return string(jsonBytes), nil
}

// splitTopLevel divide uma string por um delimitador apenas no nível mais externo (fora de aspas e parênteses/colchetes/chaves).
func splitTopLevel(s string, delimiter rune) []string {
	var parts []string
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	parenNesting := 0   // ( )
	bracketNesting := 0 // [ ]
	braceNesting := 0   // { }

	for i := 0; i < len(s); i++ {
		char := rune(s[i])

		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			current.WriteRune(char)
			escaped = true
			continue
		}
		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteRune(char)
			continue
		}
		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteRune(char)
			continue
		}

		if !inSingleQuote && !inDoubleQuote {
			switch char {
			case '(':
				parenNesting++
			case ')':
				parenNesting--
			case '[':
				bracketNesting++
			case ']':
				bracketNesting--
			case '{':
				braceNesting++
			case '}':
				braceNesting--
			}

			if char == delimiter && parenNesting == 0 && bracketNesting == 0 && braceNesting == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(char)
	}
	parts = append(parts, current.String())
	return parts
}

// parsePythonString extrai o valor de uma string literal com aspas simples ou duplas e resolve sequências de escape.
func parsePythonString(s string) (string, error) {
	if len(s) < 2 {
		return "", fmt.Errorf("literal de string inválido: %s", s)
	}
	quoteChar := s[0]
	if quoteChar != '\'' && quoteChar != '"' {
		return "", fmt.Errorf("não é uma literal de string: %s", s)
	}
	if s[len(s)-1] != quoteChar {
		return "", fmt.Errorf("literal de string não terminada: %s", s)
	}

	var sb strings.Builder
	escaped := false
	for i := 1; i < len(s)-1; i++ {
		char := s[i]
		if escaped {
			switch char {
			case 'n':
				sb.WriteRune('\n')
			case 'r':
				sb.WriteRune('\r')
			case 't':
				sb.WriteRune('\t')
			case '\\':
				sb.WriteRune('\\')
			case '\'':
				sb.WriteRune('\'')
			case '"':
				sb.WriteRune('"')
			default:
				sb.WriteRune('\\')
				sb.WriteRune(rune(char))
			}
			escaped = false
		} else if char == '\\' {
			escaped = true
		} else {
			sb.WriteRune(rune(char))
		}
	}
	return sb.String(), nil
}

// TryParseMarkdownToolCalls tenta extrair chamadas de ferramentas de blocos de código markdown.
func TryParseMarkdownToolCalls(content string) []llm.ToolCall {
	var toolCalls []llm.ToolCall

	// Encontra blocos de código markdown
	lines := strings.Split(content, "\n")

	inBlock := false
	var blockLines []string
	var detectedPath string
	var blockLang string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				inBlock = true
				blockLines = nil
				detectedPath = ""
				blockLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				// remove any parameters after spaces
				if idx := strings.Index(blockLang, " "); idx != -1 {
					blockLang = blockLang[:idx]
				}

				// Tenta detectar o arquivo procurando na linha imediatamente anterior
				if i > 0 {
					prevLine := strings.TrimSpace(lines[i-1])
					detectedPath = parseFilePathFromText(prevLine)
				}
			} else {
				inBlock = false
				// Processa o bloco finalizado
				if len(blockLines) > 0 {
					// Verifica se o caminho está nas primeiras linhas do bloco como comentário (Item 36)
					for lIdx := 0; lIdx < len(blockLines) && lIdx < 3; lIdx++ {
						lineVal := strings.TrimSpace(blockLines[lIdx])
						pathVal := parseFilePathFromComment(lineVal)
						if pathVal != "" {
							detectedPath = pathVal
							// Remove a linha de comentário do path
							blockLines = append(blockLines[:lIdx], blockLines[lIdx+1:]...)
							lIdx--
						}
					}

					// Fallback: se ainda não detectou o caminho do arquivo, tenta buscar um arquivo único no texto com a extensão correspondente
					if detectedPath == "" {
						detectedPath = findUniqueFileInTextWithExtension(content, blockLang)
					}

					if detectedPath != "" {
						// Limpeza de linhas em branco extras no início/fim
						for len(blockLines) > 0 && strings.TrimSpace(blockLines[0]) == "" {
							blockLines = blockLines[1:]
						}
						for len(blockLines) > 0 && strings.TrimSpace(blockLines[len(blockLines)-1]) == "" {
							blockLines = blockLines[:len(blockLines)-1]
						}

						fileContent := strings.Join(blockLines, "\n")
						// Cria argumentos JSON para a ferramenta write_file
						argsMap := map[string]string{
							"path":    detectedPath,
							"content": fileContent,
						}
						argsBytes, err := json.Marshal(argsMap)
						if err == nil {
							toolCalls = append(toolCalls, llm.ToolCall{
								ID:   fmt.Sprintf("mdcall_%d_%d", time.Now().UnixNano(), len(toolCalls)),
								Type: "function",
								Function: llm.FunctionCall{
									Name:      "write_file",
									Arguments: string(argsBytes),
								},
							})
						}
					}
				}
			}
			continue
		}

		if inBlock {
			blockLines = append(blockLines, line)
		}
	}

	return toolCalls
}

func findUniqueFileInText(content string) string {
	words := strings.Fields(content)
	fileMap := make(map[string]bool)
	for _, word := range words {
		word = strings.Trim(word, "\"`'*(),.:;!?")
		if hasCommonExtension(word) {
			fileMap[word] = true
		}
	}
	if len(fileMap) == 1 {
		for f := range fileMap {
			return f
		}
	}
	return ""
}

func findUniqueFileInTextWithExtension(content string, lang string) string {
	ext := ""
	switch strings.ToLower(lang) {
	case "python", "py":
		ext = ".py"
	case "go":
		ext = ".go"
	case "json":
		ext = ".json"
	case "html", "htm":
		ext = ".html"
	case "js", "javascript":
		ext = ".js"
	case "ts", "typescript":
		ext = ".ts"
	case "sh", "bash":
		ext = ".sh"
	case "md":
		ext = ".md"
	case "yml", "yaml":
		ext = ".yml"
	default:
		// Se não houver mapeamento direto, busca qualquer extensão comum
		return findUniqueFileInText(content)
	}

	words := strings.Fields(content)
	fileMap := make(map[string]bool)
	for _, word := range words {
		word = strings.Trim(word, "\"`'*(),.:;!?")
		if strings.HasSuffix(strings.ToLower(word), ext) {
			fileMap[word] = true
		}
	}
	if len(fileMap) == 1 {
		for f := range fileMap {
			return f
		}
	}
	return ""
}

func parseFilePathFromComment(line string) string {
	// Remove prefixes comuns de comentários
	line = strings.TrimPrefix(line, "#")
	line = strings.TrimPrefix(line, "//")
	line = strings.TrimPrefix(line, "/*")
	line = strings.TrimSuffix(line, "*/")
	line = strings.TrimPrefix(line, "<!--")
	line = strings.TrimSuffix(line, "-->")
	line = strings.TrimSpace(line)

	// Padrões como "FILE: caminho", "File: caminho", "caminho"
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "file:") {
		return strings.TrimSpace(line[5:])
	}
	if strings.HasPrefix(lower, "caminho:") {
		return strings.TrimSpace(line[8:])
	}
	if strings.HasPrefix(lower, "arquivo:") {
		return strings.TrimSpace(line[8:])
	}
	if strings.HasPrefix(lower, "path:") {
		return strings.TrimSpace(line[5:])
	}

	// Se tiver extensão de arquivo comum, assume que é o caminho do arquivo
	if hasCommonExtension(line) {
		return line
	}
	return ""
}

func parseFilePathFromText(line string) string {
	lower := strings.ToLower(line)
	// Remove caracteres especiais no final como ":" ou "."
	line = strings.TrimRight(line, ":. \t\r\n")

	// Se o texto contém "file:" ou "arquivo:" ou "path:"
	if idx := strings.Index(lower, "file:"); idx != -1 {
		return strings.TrimSpace(line[idx+5:])
	}
	if idx := strings.Index(lower, "arquivo:"); idx != -1 {
		return strings.TrimSpace(line[idx+8:])
	}
	if idx := strings.Index(lower, "path:"); idx != -1 {
		return strings.TrimSpace(line[idx+5:])
	}

	// Tenta extrair a última palavra se for um caminho com extensão
	words := strings.Fields(line)
	if len(words) > 0 {
		lastWord := words[len(words)-1]
		lastWord = strings.Trim(lastWord, "\"`'*")
		if hasCommonExtension(lastWord) {
			return lastWord
		}
	}
	return ""
}

func hasCommonExtension(s string) bool {
	exts := []string{
		".go", ".py", ".js", ".ts", ".html", ".css", ".json", ".php", ".txt", ".sh", ".md", ".yml", ".yaml", ".sql", ".ini",
	}
	s = strings.ToLower(s)
	for _, ext := range exts {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}
