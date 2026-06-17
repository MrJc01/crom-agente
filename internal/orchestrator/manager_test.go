package orchestrator

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
)

type testEventHandler struct {
	done chan struct{}
}

func (t *testEventHandler) OnStatusChange(status string) {
	if status == "finished" || status == "idle" || strings.HasPrefix(status, "error:") {
		select {
		case <-t.done:
		default:
			close(t.done)
		}
	}
}

func (t *testEventHandler) OnMessage(string, string) {}

func TestMultiAgentManager_AddWorkspace(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	mgr := NewMultiAgentManager()
	wsDir := t.TempDir()

	// 1. Adiciona workspace
	err := mgr.AddWorkspace("test-proj", wsDir)
	if err != nil {
		t.Fatalf("erro ao adicionar workspace: %v", err)
	}

	// 2. Tenta adicionar novamente (duplicidade)
	err = mgr.AddWorkspace("test-proj", wsDir)
	if err == nil {
		t.Fatal("esperava erro ao adicionar duplicado, obteve nil")
	}

	// 3. Carrega e verifica
	list, err := LoadWorkspaces()
	if err != nil {
		t.Fatalf("erro ao carregar workspaces: %v", err)
	}
	if len(list) != 1 || list[0].Name != "test-proj" {
		t.Fatalf("lista de workspaces incorreta: %+v", list)
	}

	// 4. Remove workspace
	err = mgr.RemoveWorkspace("test-proj")
	if err != nil {
		t.Fatalf("erro ao remover workspace: %v", err)
	}

	listAfter, _ := LoadWorkspaces()
	if len(listAfter) != 0 {
		t.Fatalf("esperado lista vazia, obteve %+v", listAfter)
	}
}

func TestMultiAgentManager_StartStopAgent(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Cria arquivos de config global e .env padrões para evitar erro de inicialização
	gDir, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("erro ao obter global dir: %v", err)
	}
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock" // Usa o provider mock para rodar testes offline sem chaves reais
	_ = config.SaveGlobalConfig(gDir, gCfg)

	env := &config.EnvVars{}
	_ = env.Save(gDir)

	wsDir := t.TempDir()
	// Configura workspace config padrão
	_ = config.SaveWorkspaceConfig(wsDir, config.DefaultWorkspaceConfig("test-proj"))

	mgr := NewMultiAgentManager()
	_ = mgr.AddWorkspace("test-proj", wsDir)

	ctx := context.Background()
	handler := &testEventHandler{done: make(chan struct{})}

	err = mgr.StartAgent(ctx, "test-proj", "", "Tarefa de teste concorrente", handler)
	if err != nil {
		t.Fatalf("erro ao iniciar agente: %v", err)
	}

	// Verifica se o agente está ativo
	running := mgr.ListRunningAgents()
	if len(running) != 1 || running[0].WorkspaceName != "test-proj" {
		t.Fatalf("esperava 1 agente rodando, obteve: %d", len(running))
	}

	// Para o agente
	err = mgr.StopAgent("test-proj")
	if err != nil {
		t.Fatalf("erro ao parar agente: %v", err)
	}

	// Aguarda o status change sinalizar que o loop encerrou
	select {
	case <-handler.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando fim do agente")
	}

	// Aguarda mais um tiquinho para a goroutine sair completamente da pilha
	time.Sleep(50 * time.Millisecond)

	runningAfter := mgr.ListRunningAgents()
	if len(runningAfter) != 0 {
		t.Fatalf("esperava 0 agentes ativos, obteve %d", len(runningAfter))
	}
}
