package main

import (
	"context"
	"fmt"
	"os"

	"github.com/crom/crom-agente/internal/llm"
)

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Println("Erro: OPENROUTER_API_KEY não definida")
		os.Exit(1)
	}

	fmt.Println("Testando OpenRouter com api key...")
	p := llm.NewOpenAIProvider(apiKey, "google/gemini-2.5-flash-lite")
	p.URL = "https://openrouter.ai/api/v1/chat/completions"

	messages := []llm.Message{
		{Role: "user", Content: "Olá! Navegue no site Google e tire um screenshot."},
	}

	// Criando definição de uma ferramenta
	toolDef := llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunctionSchema{
			Name:        "browser_action",
			Description: "Navega e executa ações no browser",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type": "string",
						"enum": []string{"navigate", "click", "type", "screenshot"},
					},
					"url": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []interface{}{"action"},
			},
		},
	}

	opts := llm.RequestOptions{
		Tools:      []llm.ToolDefinition{toolDef},
		ToolChoice: "auto",
	}

	resp, err := p.SendMessages(context.Background(), messages, opts)
	if err != nil {
		fmt.Printf("Erro ao chamar SendMessages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Sucesso!\n")
	fmt.Printf("Mensagem Retornada: %q\n", resp.Message.Content)
	fmt.Printf("Número de ToolCalls: %d\n", len(resp.Message.ToolCalls))
	for idx, tc := range resp.Message.ToolCalls {
		fmt.Printf("ToolCall[%d]: ID=%s, Name=%s, Args=%s\n", idx, tc.ID, tc.Function.Name, tc.Function.Arguments)
	}
}
