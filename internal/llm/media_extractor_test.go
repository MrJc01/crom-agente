package llm

import (
	"context"
	"strings"
	"testing"
)

func TestStripMultimodalPayloads(t *testing.T) {
	// 1. Test string stripping
	contentStr := "Algum texto inicial\nimage:base64:SGVsbG8=\naudio:base64:V29ybGQ=\nOutro texto"
	strippedStr := StripMultimodalPayloads(contentStr)
	resStr, ok := strippedStr.(string)
	if !ok {
		t.Fatalf("esperava string, obteve %T", strippedStr)
	}
	if strings.Contains(resStr, "image:base64:") || strings.Contains(resStr, "audio:base64:") {
		t.Errorf("esperava que os prefixos de mídia fossem removidos, obteve: %q", resStr)
	}
	if !strings.Contains(resStr, "[Mídia nativa omitida para compatibilidade]") {
		t.Errorf("esperava placeholder de substituição, obteve: %q", resStr)
	}

	// 2. Test slice stripping (OpenAI structure)
	contentParts := []interface{}{
		map[string]interface{}{"type": "text", "text": "Instrução textual"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,..."}},
	}
	strippedParts := StripMultimodalPayloads(contentParts)
	parts, ok := strippedParts.([]interface{})
	if !ok {
		t.Fatalf("esperava []interface{}, obteve %T", strippedParts)
	}
	if len(parts) != 1 {
		t.Fatalf("esperava 1 item após a remoção, obteve %d", len(parts))
	}
	partMap, ok := parts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("esperava map, obteve %T", parts[0])
	}
	if partMap["type"] != "text" || partMap["text"] != "Instrução textual" {
		t.Errorf("parte textual incorreta: %v", partMap)
	}
}

func TestExtractAndInjectMediaContext(t *testing.T) {
	// Simula mensagens com base64
	messages := []Message{
		{
			Role:    "user",
			Content: "Verifique esta imagem:\nimage:base64:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAAAAAA6fptVAAAACklEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", // PNG pixel
		},
		{
			Role:    "assistant",
			Content: "Olhando imagem...",
		},
		{
			Role:    "user",
			Content: "Mensagem com contexto já processado:\n[Layer 2 Estrutura Predefinida - Imagem]: Tipo: PNG\nimage:base64:iVBORw0KGgoAAA==",
		},
	}

	// Como a API Key está em branco, o MediaExtractor usará a estrutura predefinida (Layer 2 local)
	injected := ExtractAndInjectMediaContext(context.Background(), messages, "mock", "", "")

	if len(injected) != len(messages) {
		t.Fatalf("tamanho de mensagens alterado de %d para %d", len(messages), len(injected))
	}

	// Mensagem 1: Deve conter o contexto injetado
	if !strings.Contains(injected[0].Content, "[Layer 2 Estrutura Predefinida - Imagem]") {
		t.Errorf("esperava injeção de Layer 2 na primeira mensagem, obteve: %q", injected[0].Content)
	}

	// Mensagem 2 (assistant): Deve permanecer intocada
	if injected[1].Content != "Olhando imagem..." {
		t.Errorf("mensagem do assistant alterada: %q", injected[1].Content)
	}

	// Mensagem 3: Já tinha "[Layer 2", portanto não deve ser re-processada ou duplicada
	if strings.Count(injected[2].Content, "[Layer 2") != 1 {
		t.Errorf("esperava que a terceira mensagem mantivesse apenas a injeção original única, obteve: %q", injected[2].Content)
	}
}
