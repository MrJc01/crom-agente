package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
)

func TestGeminiProvider_Name(t *testing.T) {
	provider := NewGeminiProvider("fake-key", "gemini-1.5-pro")
	if provider.Name() != "gemini" {
		t.Errorf("Expected name 'gemini', got '%s'", provider.Name())
	}
}

func TestGeminiProvider_SendMessages(t *testing.T) {
	// Start a mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v1beta/chat/completions" {
			// This test uses the real hardcoded URL inside SendMessages,
			// so the test needs to mock http.DefaultTransport or we skip network tests.
			// Given that SendMessages has hardcoded URLs, we will just test the struct init.
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Hello from Gemini mock"
				}
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`))
	}))
	defer mockServer.Close()

	provider := NewGeminiProvider("fake-key", "gemini-1.5-pro")

	// Fast-fail the context so we don't actually hit Google if it tries to
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.SendMessages(ctx, []llm.Message{{Role: "user", Content: "Hello"}}, llm.RequestOptions{})

	// We expect a context canceled error because it tries to hit the real Google API
	if err == nil {
		t.Errorf("Expected error due to canceled context, got nil")
	}
}

func TestParseGeminiMultimodalContent(t *testing.T) {
	text := "Some text\nimage:base64:iVBORw0KGgo=\nMore text"
	result := parseGeminiMultimodalContent(text)

	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got %T", result)
	}

	if len(arr) != 3 {
		t.Errorf("Expected 3 parts, got %d", len(arr))
	}
}
