package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/state"
)

func TestAgenticLoop_SpawnSubagent_Success(t *testing.T) {
	ws := t.TempDir()

	// Respostas sequenciais para o MockProvider:
	// 1. Pai chama spawn_subagent
	// 2. Filho responde com texto concluído (sem tool calls)
	// 3. Pai responde com texto final concluído
	provider := llm.NewMockProvider(
		llm.MockToolCallResponse("spawn_subagent", `{"task": "subtask task"}`, 10),
		llm.MockTextResponse("filho concluído", 10),
		llm.MockTextResponse("pai verificado", 10),
		llm.MockTextResponse("pai concluído", 10),
	)

	sm := state.NewStateManager(filepath.Join(ws, ".crom"))
	_ = sm.LoadState()

	al := New(provider, sm, nil)
	al.RegisterSpawnSubagentTool()

	err := al.Execute(context.Background(), "tarefa pai")
	if err != nil {
		t.Fatalf("erro ao executar loop pai: %v", err)
	}

	// Verifica se o subagente foi registrado na pasta agents/
	agentsDir := filepath.Join(ws, ".crom", "agents")
	files, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("erro ao ler pasta de subagentes: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("esperava exatamente 1 subagente registrado, obteve %d", len(files))
	}
}

func TestAgenticLoop_SpawnSubagent_GitRollback(t *testing.T) {
	ws := t.TempDir()

	// 1. Inicializa repositório Git temporário real
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = ws
	_ = cmdInit.Run()

	cmdUser := exec.Command("git", "config", "user.name", "Test")
	cmdUser.Dir = ws
	_ = cmdUser.Run()
	cmdEmail := exec.Command("git", "config", "user.email", "test@test.com")
	cmdEmail.Dir = ws
	_ = cmdEmail.Run()

	// 2. Cria arquivo base
	initFile := filepath.Join(ws, "main.go")
	_ = os.WriteFile(initFile, []byte("versao 1"), 0644)

	cmdAdd := exec.Command("git", "add", "main.go")
	cmdAdd.Dir = ws
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "init")
	cmdCommit.Dir = ws
	_ = cmdCommit.Run()

	// 3. Altera arquivo fisicamente (modificação não comitada)
	_ = os.WriteFile(initFile, []byte("versao modificada"), 0644)

	// 4. Configura mock LLM para falhar
	provider := llm.NewMockProvider(
		llm.MockToolCallResponse("spawn_subagent", `{"task": "subtask falha"}`, 10),
		llm.MockErrorResponse("simulated LLM error inside subagent"),
	)

	sm := state.NewStateManager(filepath.Join(ws, ".crom"))
	_ = sm.LoadState()

	al := New(provider, sm, nil)
	al.RegisterSpawnSubagentTool()

	// Executa (deve falhar e disparar rollback)
	_ = al.Execute(context.Background(), "tarefa pai")

	// 5. Verifica se o git rollback reverteu o arquivo
	data, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo após rollback: %v", err)
	}
	if string(data) != "versao 1" {
		t.Fatalf("esperava rollback para 'versao 1', obteve %q", string(data))
	}
}
