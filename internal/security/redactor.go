package security

import (
	"regexp"
)

var (
	openaiRegex    = regexp.MustCompile(`(?i)sk-[a-zA-Z0-9]{48}`)
	anthropicRegex = regexp.MustCompile(`(?i)sk-ant-[a-zA-Z0-9_-]{50,}`)
	geminiRegex    = regexp.MustCompile(`(?i)AIzaSy[a-zA-Z0-9_-]{33}`)
	connStrRegex   = regexp.MustCompile(`(?i)(postgres|mysql|mongodb|mongodb\+srv)?://[^:\s]+:[^@\s]+@[^:\s]+:[0-9]+/[^\s?]*`)
)

// Redact substitui segredos e chaves de API conhecidas por um marcador de segurança
func Redact(text string) string {
	if text == "" {
		return text
	}

	// 1. Mascarar OpenAI API Keys
	text = openaiRegex.ReplaceAllString(text, "sk-***REDACTED***")

	// 2. Mascarar Anthropic API Keys
	text = anthropicRegex.ReplaceAllString(text, "sk-ant-***REDACTED***")

	// 3. Mascarar Gemini API Keys
	text = geminiRegex.ReplaceAllString(text, "AIzaSy***REDACTED***")

	// 4. Mascarar senhas em strings de conexão do banco de dados
	text = connStrRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Substituir a parte da senha (entre o usuário ":" e o host "@")
		subparts := regexp.MustCompile(`://([^:\s]+):([^@\s]+)@`).FindStringSubmatch(match)
		if len(subparts) == 3 {
			user := subparts[1]
			// Substitui a senha por asteriscos na string original
			oldAuth := user + ":" + subparts[2] + "@"
			newAuth := user + ":***REDACTED***@"
			return regexp.MustCompile(regexp.QuoteMeta(oldAuth)).ReplaceAllString(match, newAuth)
		}
		return match
	})

	return text
}
