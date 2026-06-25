package core_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	_ "github.com/crom/crom-agente/internal/agents/specialists/spawn"
	"github.com/crom/crom-agente/internal/llm/providers"
	agenticcore "github.com/crom/crom-agente/internal/loop/agentic/core"
	"github.com/crom/crom-agente/internal/state"
)

func TestAgenticLoop_SpawnSubagent_Success(t *testing.T) {
	ws := t.TempDir()

	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("spawn_subagent", `{"task": "subtask task"}`, 10),
		providers.MockTextResponse("filho concluído", 10),
		providers.MockTextResponse("pai verificado", 10),
		providers.MockTextResponse("pai concluído", 10),
	)

	sm := state.NewStateManager(filepath.Join(ws, ".crom"))
	_ = sm.LoadState()

	al := agenticcore.New(provider, sm, nil)
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

	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = ws
	_ = cmdInit.Run()

	cmdUser := exec.Command("git", "config", "user.name", "Test")
	cmdUser.Dir = ws
	_ = cmdUser.Run()
	cmdEmail := exec.Command("git", "config", "user.email", "test@test.com")
	cmdEmail.Dir = ws
	_ = cmdEmail.Run()

	initFile := filepath.Join(ws, "main.go")
	_ = os.WriteFile(initFile, []byte("versao 1"), 0644)

	cmdAdd := exec.Command("git", "add", "main.go")
	cmdAdd.Dir = ws
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "init")
	cmdCommit.Dir = ws
	_ = cmdCommit.Run()

	_ = os.WriteFile(initFile, []byte("versao modificada"), 0644)

	provider := providers.NewMockProvider(
		providers.MockToolCallResponse("spawn_subagent", `{"task": "subtask falha"}`, 10),
		providers.MockErrorResponse("simulated LLM error inside subagent"),
	)

	sm := state.NewStateManager(filepath.Join(ws, ".crom"))
	_ = sm.LoadState()

	al := agenticcore.New(provider, sm, nil)
	al.RegisterSpawnSubagentTool()

	_ = al.Execute(context.Background(), "tarefa pai")

	data, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo após rollback: %v", err)
	}
	if string(data) != "versao 1" {
		t.Fatalf("esperava rollback para 'versao 1', obteve %q", string(data))
	}
}
