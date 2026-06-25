package ask_user_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/ask_user"
)

func TestAskUserTool(t *testing.T) {
	tool := ask_user.NewAskUserTool("/tmp")

	if tool.ID() != "ask_user" {
		t.Errorf("esperava ID 'ask_user', obteve '%s'", tool.ID())
	}

	if tool.RequiresApproval() {
		t.Errorf("RequiresApproval deveria ser falso")
	}

	// 1. Caso com JSON inválido
	resErr, _ := tool.Execute(context.Background(), json.RawMessage("{invalid"))
	if resErr.Success {
		t.Errorf("esperava falha para JSON inválido")
	}

	// 2. Caso com pergunta vazia
	resEmpty, _ := tool.Execute(context.Background(), json.RawMessage(`{"question": ""}`))
	if resEmpty.Success {
		t.Errorf("esperava falha para pergunta vazia")
	}

	// 3. Caso padrão com pergunta
	inputNormal := `{"question": "Qual banco de dados usar?"}`
	resNormal, err := tool.Execute(context.Background(), json.RawMessage(inputNormal))
	if err != nil {
		t.Fatalf("Execute falhou com erro Go: %v", err)
	}
	if !resNormal.Success {
		t.Errorf("esperava sucesso para pergunta normal, erro: %s", resNormal.Error)
	}
	if !strings.Contains(resNormal.Data, "Qual banco de dados usar?") {
		t.Errorf("Data de retorno esperada não contém a pergunta")
	}

	// 4. Caso estruturado com opções recomendadas e allow_custom
	inputOptions := `{
		"question": "Qual banco de dados usar?",
		"options": ["Postgres", "MySQL", "SQLite"],
		"allow_custom": true
	}`
	resOptions, err := tool.Execute(context.Background(), json.RawMessage(inputOptions))
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if !resOptions.Success {
		t.Errorf("esperava sucesso com opções, erro: %s", resOptions.Error)
	}
}
