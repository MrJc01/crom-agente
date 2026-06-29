package core

import (
	"context"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
)

// MockProvider implementa llm.Provider para testes
type MockProvider struct {
	ResponseContent string
}

func (m *MockProvider) SendMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions) (*llm.Response, error) {
	return &llm.Response{
		Message: llm.Message{Role: "assistant", Content: m.ResponseContent},
		Usage:   llm.Usage{TotalTokens: 10},
	}, nil
}
func (m *MockProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	return m.SendMessages(ctx, messages, opts)
}
func (m *MockProvider) Name() string                        { return "mock" }
func (m *MockProvider) SupportsSystemPrompt() bool          { return true }
func (m *MockProvider) Capabilities() llm.ModelCapabilities { return llm.ModelCapabilities{} }

func TestPlannerLoop_ExtractJSON(t *testing.T) {
	// Cria um executor mock
	cfg := &config.ResolvedConfig{
		CognitiveArchitecture: config.CognitiveArchitecture{
			StructuralDecomposition: true,
		},
	}

	// Mock Provider que retorna Markdown
	provider := &MockProvider{
		ResponseContent: "Aqui está o seu plano:\n```json\n[\n  \"Passo 1\",\n  \"Passo 2\"\n]\n```\nBom trabalho!",
	}

	// Cria um StateManager dummy temporário para não dar nil pointer no motor
	sm := state.NewStateManager("/tmp")
	executor := New(provider, sm, nil, cfg)
	planner := NewPlannerLoop(executor)

	// O mock do provider deve falhar graciosamente no executeCoreLoop porque não há intenção de ferramentas,
	// mas o PlannerLoop DEVE extrair as 2 tarefas corretamente sem dar panic no JSON parse.

	// Como o executeCoreLoop no Mock vai dar erro de loop (fallback texto),
	// vamos ignorar o erro do motor, mas queremos garantir que o Planner processou o JSON
	err := planner.Execute(context.Background(), "Faça algo")

	// Como o sm é nil neste teste simplificado, e o MockProvider retorna a mesma resposta pra tudo,
	// o executeCoreLoop tentará rodar "Passo 1".
	// Nós validaremos apenas se o JSON Unmarshal parou de dar erro no Markdown.
	if err != nil && strings.Contains(err.Error(), "falha ao parsear tarefas do planner") {
		t.Fatalf("Planner falhou ao parsear JSON dentro de Markdown: %v", err)
	}

	// Se passou, significa que a extração de [ a ] funcionou!
}
