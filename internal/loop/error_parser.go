package loop

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Padrão comum de erro Go, JS, Rust e C: file.go:12:3 ou file.js:12:3
	compileErrorRegex = regexp.MustCompile(`(?m)^([^:\s#]+):([0-9]+):(?:[0-9]+:)?\s+(.*)$`)
)

// ParsedError representa um erro estruturado de compilação/teste extraído dos logs
type ParsedError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ParseTerminalErrors analisa uma saída de texto bruta e extrai uma lista de erros de compilação
func ParseTerminalErrors(output string) []ParsedError {
	var results []ParsedError
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		matches := compileErrorRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			var lineNum int
			_, _ = fmt.Sscanf(matches[2], "%d", &lineNum)

			results = append(results, ParsedError{
				File:    strings.TrimSpace(matches[1]),
				Line:    lineNum,
				Message: strings.TrimSpace(matches[3]),
			})
		}
	}

	return results
}

// FormatContextualError cria uma string amigável para o prompt se erros estruturados forem identificados
func FormatContextualError(output string) string {
	errors := ParseTerminalErrors(output)
	if len(errors) == 0 {
		return output
	}

	var sb strings.Builder
	sb.WriteString(output)
	sb.WriteString("\n\n🔍 [ANÁLISE DE ERROS ESTRUTURADA]:\n")
	for _, err := range errors {
		sb.WriteString(fmt.Sprintf("📍 Arquivo: %s | Linha: %d | Erro: %s\n", err.File, err.Line, err.Message))
	}
	return sb.String()
}
