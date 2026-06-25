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

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				inBlock = true
				blockLines = nil
				detectedPath = ""

				// Tenta detectar o arquivo procurando na linha imediatamente anterior
				if i > 0 {
					prevLine := strings.TrimSpace(lines[i-1])
					detectedPath = parseFilePathFromText(prevLine)
				}
			} else {
				inBlock = false
				// Processa o bloco finalizado
				if len(blockLines) > 0 {
					// Verifica se o caminho está na primeira linha do bloco como comentário
					firstLine := strings.TrimSpace(blockLines[0])
					pathInFirstLine := parseFilePathFromComment(firstLine)
					if pathInFirstLine != "" {
						detectedPath = pathInFirstLine
						blockLines = blockLines[1:] // remove a linha de comentário do path
					}

					if detectedPath != "" {
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
