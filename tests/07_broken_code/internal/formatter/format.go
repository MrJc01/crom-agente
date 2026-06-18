package formatter

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// FormatOutput é uma função intencionalmente complexa demais (complexidade ciclomática > 20).
// O agente deve refatorá-la em funções menores e mais legíveis.
func FormatOutput(input string, format string, uppercase bool, trim bool, maxLen int, prefix string, suffix string, addTimestamp bool, timestampFmt string, wrapWidth int, padChar rune, padSide string, replaceSpaces bool, replaceWith string, removeNonAlpha bool, addLineNumbers bool) string {
	result := input

	// Trim
	if trim {
		result = strings.TrimSpace(result)
	}

	// Uppercase
	if uppercase {
		result = strings.ToUpper(result)
	}

	// Replace spaces
	if replaceSpaces && replaceWith != "" {
		result = strings.ReplaceAll(result, " ", replaceWith)
	}

	// Remove non-alpha
	if removeNonAlpha {
		var filtered []rune
		for _, r := range result {
			if unicode.IsLetter(r) || unicode.IsSpace(r) {
				filtered = append(filtered, r)
			}
		}
		result = string(filtered)
	}

	// Max length
	if maxLen > 0 && len(result) > maxLen {
		result = result[:maxLen]
	}

	// Prefix e suffix
	if prefix != "" {
		result = prefix + result
	}
	if suffix != "" {
		result = result + suffix
	}

	// Timestamp
	if addTimestamp {
		ts := time.Now()
		if timestampFmt == "" {
			timestampFmt = "2006-01-02 15:04:05"
		}
		result = fmt.Sprintf("[%s] %s", ts.Format(timestampFmt), result)
	}

	// Padding
	if wrapWidth > 0 && len(result) < wrapWidth {
		pad := wrapWidth - len(result)
		padStr := strings.Repeat(string(padChar), pad)
		if padSide == "left" {
			result = padStr + result
		} else if padSide == "right" {
			result = result + padStr
		} else if padSide == "center" {
			leftPad := pad / 2
			rightPad := pad - leftPad
			result = strings.Repeat(string(padChar), leftPad) + result + strings.Repeat(string(padChar), rightPad)
		}
	}

	// Word wrap
	if wrapWidth > 0 {
		var lines []string
		words := strings.Fields(result)
		currentLine := ""
		for _, word := range words {
			if len(currentLine)+len(word)+1 > wrapWidth {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				if currentLine != "" {
					currentLine += " "
				}
				currentLine += word
			}
		}
		if currentLine != "" {
			lines = append(lines, currentLine)
		}
		result = strings.Join(lines, "\n")
	}

	// Line numbers
	if addLineNumbers {
		lines := strings.Split(result, "\n")
		var numbered []string
		for i, line := range lines {
			numbered = append(numbered, fmt.Sprintf("%3d | %s", i+1, line))
		}
		result = strings.Join(numbered, "\n")
	}

	// Format
	switch format {
	case "json":
		result = fmt.Sprintf(`{"output": %q}`, result)
	case "xml":
		result = fmt.Sprintf("<output>%s</output>", result)
	case "markdown":
		result = fmt.Sprintf("```\n%s\n```", result)
	case "html":
		result = fmt.Sprintf("<pre>%s</pre>", result)
	case "csv":
		result = strings.ReplaceAll(result, "\n", ",")
	}

	return result
}
