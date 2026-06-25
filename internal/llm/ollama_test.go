package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProvider_SendMessages_SanitizesDeepSeek(t *testing.T) {
	var capturedBody []byte

	// Mock server do Ollama
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		capturedBody = body

		// Responde com mensagem de sucesso
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"message": {
				"role": "assistant",
				"content": "Executado com sucesso localmente"
			},
			"done": true,
			"prompt_eval_count": 10,
			"eval_count": 5
		}`))
	}))
	defer server.Close()

	// 1. Testa modelo DeepSeek (deve sanitizar histórico e desativar tools)
	pDeepSeek := NewOllamaProvider(server.URL, "deepseek-r1:8b")

	history := []Message{
		{Role: "user", Content: "Rode a tarefa"},
		{
			Role:    "assistant",
			Content: "Vou rodar.",
			ToolCalls: []ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"a.txt","content":"ok"}`,
					},
				},
			},
		},
		{Role: "tool", Name: "write_file", ToolCallID: "call-1", Content: "Sucesso"},
	}

	opts := RequestOptions{
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: ToolFunctionSchema{
					Name:        "write_file",
					Description: "Escreve arquivo",
				},
			},
		},
	}

	_, err := pDeepSeek.SendMessages(context.Background(), history, opts)
	if err != nil {
		t.Fatalf("SendMessages falhou: %v", err)
	}

	// Analisa o body enviado para o DeepSeek
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role      string      `json:"role"`
			Content   string      `json:"content"`
			ToolCalls interface{} `json:"tool_calls"`
		} `json:"messages"`
		Tools []interface{} `json:"tools"`
	}

	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("erro ao unmarshal captured request: %v", err)
	}

	// Verifica se as tools foram removidas do request (DeepSeek)
	if len(req.Tools) != 0 {
		t.Errorf("esperava 0 tools para deepseek, obteve %d", len(req.Tools))
	}

	// Verifica se a mensagem de "tool" foi convertida para "user"
	if req.Messages[2].Role != "user" {
		t.Errorf("esperava role 'user' para resposta de ferramenta no DeepSeek, obteve %q", req.Messages[2].Role)
	}
	if !strings.Contains(req.Messages[2].Content, "[Retorno da ferramenta write_file: Sucesso]") {
		t.Errorf("conteúdo da ferramenta não sanitizado corretamente: %q", req.Messages[2].Content)
	}

	// Verifica se a mensagem de assistant com tool_call teve a info anexada ao texto
	if !strings.Contains(req.Messages[1].Content, "[Chamando ferramentas: write_file") {
		t.Errorf("chamada de ferramenta não mesclada no conteúdo do assistente: %q", req.Messages[1].Content)
	}

	// 2. Testa modelo normal (deve enviar tool_calls normalmente)
	pNormal := NewOllamaProvider(server.URL, "qwen2.5-coder:7b")

	_, err = pNormal.SendMessages(context.Background(), history, opts)
	if err != nil {
		t.Fatalf("SendMessages falhou: %v", err)
	}

	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("erro ao unmarshal captured request: %v", err)
	}

	// Qwen deve receber a ferramenta normalmente
	if len(req.Tools) != 1 {
		t.Errorf("esperava 1 tool para qwen2.5-coder, obteve %d", len(req.Tools))
	}
}
