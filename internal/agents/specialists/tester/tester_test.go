package tester_test

import (
	"context"
	"os"
	"testing"

	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/llm/providers"

	// Importa o pacote tester para que o init() seja disparado e registre o agente
	_ "github.com/crom/crom-agente/internal/agents/specialists/tester"
)

func TestTesterAgent_MetadataAndProperties(t *testing.T) {
	mockProvider := providers.NewMockProvider(providers.MockTextResponse("OK", 5))

	cfg := agents.Config{
		WorkspacePath:   t.TempDir(),
		LLMProvider:     mockProvider,
		BrowserHeadless: true,
	}

	agentInst, ok := agents.GetAgentInst("tester", cfg)
	if !ok {
		t.Fatalf("esperava encontrar agente 'tester' registrado")
	}

	if agentInst.Name() != "tester" {
		t.Errorf("nome do agente esperado 'tester', obtido: %s", agentInst.Name())
	}

	if agentInst.Description() == "" {
		t.Errorf("descrição do agente não deveria ser vazia")
	}

	if agentInst.SystemPrompt() == "" {
		t.Errorf("system prompt do agente não deveria ser vazio")
	}

	// Verifica se as ferramentas de teste fundamentais estão na lista de permitidas
	toolIDs := agentInst.ToolIDs()
	expectedTools := map[string]bool{
		"terminal_command": true,
		"run_tests":        true,
		"read_file":        true,
		"write_file":       true,
	}

	for _, toolID := range toolIDs {
		delete(expectedTools, toolID)
	}

	if len(expectedTools) > 0 {
		t.Errorf("ferramentas esperadas não encontradas na lista de permitidas: %v", expectedTools)
	}
}

func TestTesterAgent_ExecuteMocked(t *testing.T) {
	// Cria diretório temporário para workspace
	tempDir := t.TempDir()

	// Cria estrutura .crom necessária
	err := os.MkdirAll(tempDir+"/.crom", 0755)
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária .crom: %v", err)
	}

	// Mock para responder a interações do loop
	mockProvider := providers.NewMockProvider(
		providers.MockTextResponse("Eu vou rodar os testes e reportar sucesso.", 20),
	)

	cfg := agents.Config{
		WorkspacePath:   tempDir,
		LLMProvider:     mockProvider,
		BrowserHeadless: true,
	}

	agentInst, ok := agents.GetAgentInst("tester", cfg)
	if !ok {
		t.Fatalf("esperava agente tester registrado")
	}

	ctx := context.Background()
	result, err := agentInst.Execute(ctx, "rode os testes unitários do projeto", "")
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !result.Success {
		t.Errorf("esperava resultado com sucesso")
	}

	if result.Output == "" {
		t.Errorf("esperava output preenchido")
	}
}
