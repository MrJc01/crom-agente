package security

import (
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "OpenAI Key",
			input:    "my key is sk-1234567890abcdef1234567890abcdef1234567890abcdef and it is secret",
			expected: "my key is sk-***REDACTED*** and it is secret",
		},
		{
			name:     "Anthropic Key",
			input:    "anthropic api key: sk-ant-sid01-1234567890abcdef1234567890abcdef1234567890abcdef-12345 please keep safe",
			expected: "anthropic api key: sk-ant-***REDACTED*** please keep safe",
		},
		{
			name:     "Gemini Key",
			input:    "gemini key: AIzaSyA1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6Q and that's it",
			expected: "gemini key: AIzaSy***REDACTED*** and that's it",
		},
		{
			name:     "PostgreSQL Connection URI",
			input:    "connecting to postgres://admin:secretPass123@localhost:5432/my_database?sslmode=disable...",
			expected: "connecting to postgres://admin:***REDACTED***@localhost:5432/my_database?sslmode=disable...",
		},
		{
			name:     "Normal Text",
			input:    "no secrets here, just some golang code",
			expected: "no secrets here, just some golang code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.input)
			if got != tt.expected {
				t.Errorf("Redact() = %q, expected %q", got, tt.expected)
			}
		})
	}
}
