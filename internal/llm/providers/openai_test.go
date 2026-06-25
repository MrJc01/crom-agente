package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
)

func TestOpenAIProvider_SendMessages_RetriesWithoutToolsOnError(t *testing.T) {
	requestsReceived := 0
	var lastRequestBody []byte

	// Mock server que simula o comportamento de um modelo sem suporte a tools
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		lastRequestBody = body

		if requestsReceived == 1 {
			// Retorna erro informando que tools não são suportados pelo modelo
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"message": "tools is not supported by this model"}}`))
			return
		}

		// Retorna resposta de sucesso para o retry
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Estou respondendo sem ferramentas."
				}
			}],
			"usage": {
				"prompt_tokens": 15,
				"completion_tokens": 10,
				"total_tokens": 25
			}
		}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-api-key", "some-cheap-model")
	p.URL = server.URL // Aponta para o mock server

	history := []llm.Message{
		{Role: "user", Content: "Olá"},
	}

	opts := llm.RequestOptions{
		Tools: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.ToolFunctionSchema{
					Name:        "write_file",
					Description: "Escreve arquivo",
				},
			},
		},
	}

	resp, err := p.SendMessages(context.Background(), history, opts)
	if err != nil {
		t.Fatalf("SendMessages falhou: %v", err)
	}

	// Deve ter feito 2 requisições (a primeira falhou e a segunda foi o retry)
	if requestsReceived != 2 {
		t.Fatalf("esperava 2 requisições recebidas, obteve %d", requestsReceived)
	}

	if resp.Message.Content != "Estou respondendo sem ferramentas." {
		t.Errorf("resposta incorreta do retry: %q", resp.Message.Content)
	}

	// Verifica se a segunda requisição realmente não continha a propriedade tools
	var req struct {
		Tools []interface{} `json:"tools"`
	}
	if err := json.Unmarshal(lastRequestBody, &req); err != nil {
		t.Fatalf("erro ao fazer unmarshal do request: %v", err)
	}
	if len(req.Tools) != 0 {
		t.Errorf("esperava 0 tools no request de retry, obteve %d", len(req.Tools))
	}
}
