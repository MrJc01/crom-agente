package core

import (
	"encoding/json"
	"strings"
)

// FixUnescapedQuotesInJSON attempts to fix invalid JSON caused by unescaped double quotes inside string values.
// This is a common hallucination from LLMs in native tool calls, e.g. {"command": "grep "foo" file"}
func FixUnescapedQuotesInJSON(raw string) string {
	if json.Valid([]byte(raw)) {
		return raw
	}

	// Very simple heuristic: if it's a flat JSON object {"key": "value"} where value has unescaped quotes.
	// We'll iterate through all occurrences of ": \"" (or ":\"") and find the matching ending quote.
	// Any double quote in between will be escaped.

	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
		return raw
	}

	var sb strings.Builder
	inString := false
	inKey := false
	var lastChar rune

	// A more robust but simple state machine for flat key-value pairs
	// LLMs usually fail on {"command": "awk '{print "hello"}'"}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '"' && lastChar != '\\' {
			// Is this quote starting/ending a key or value?
			// Let's see context
			prev := strings.TrimSpace(raw[:i])
			next := strings.TrimSpace(raw[i+1:])

			isStartOrEndOfKeyVal := false
			if strings.HasSuffix(prev, "{") || strings.HasSuffix(prev, ",") || strings.HasSuffix(prev, ":") {
				isStartOrEndOfKeyVal = true
			}
			if strings.HasPrefix(next, ":") || strings.HasPrefix(next, ",") || strings.HasPrefix(next, "}") {
				isStartOrEndOfKeyVal = true
			}

			if isStartOrEndOfKeyVal {
				// Valid structural quote
			} else {
				// Invalid internal quote! Escape it.
				sb.WriteByte('\\')
			}
		}
		sb.WriteByte(c)
		lastChar = rune(c)

		// Avoid unused variable warnings
		_ = inString
		_ = inKey
	}

	fixed := sb.String()
	if json.Valid([]byte(fixed)) {
		return fixed
	}
	return raw
}
