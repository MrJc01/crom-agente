package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/agents"
	agentscore "github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/loop"
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

func (t *testEventHandler) OnMessage(string, string)      {}
func (t *testEventHandler) OnEvent(event loop.AgentEvent) {}

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
	case <-time.After(10 * time.Second):
		t.Fatal("timeout aguardando fim do agente")
	}

	// Aguarda mais um tiquinho para a goroutine sair completamente da pilha
	time.Sleep(50 * time.Millisecond)

	runningAfter := mgr.ListRunningAgents()
	if len(runningAfter) != 0 {
		t.Fatalf("esperava 0 agentes ativos, obteve %d", len(runningAfter))
	}
}

func TestMultiAgentManager_EnvOverride(t *testing.T) {
	wsDir := t.TempDir()

	// 1. Cria .env na raiz
	rootEnvPath := filepath.Join(wsDir, ".env")
	err := os.WriteFile(rootEnvPath, []byte("API_KEY=root_key\nOTHER_KEY=root_other"), 0644)
	if err != nil {
		t.Fatalf("erro ao criar .env raiz: %v", err)
	}

	// 2. Cria .crom/.env
	cromDir := filepath.Join(wsDir, ".crom")
	_ = os.MkdirAll(cromDir, 0755)
	cromEnvPath := filepath.Join(cromDir, ".env")
	err = os.WriteFile(cromEnvPath, []byte("API_KEY=crom_key\nNEW_KEY=crom_new"), 0644)
	if err != nil {
		t.Fatalf("erro ao criar .crom/.env: %v", err)
	}

	// 3. Simula a logica do manager
	env, _ := config.LoadEnvVars(wsDir)

	// Carrega de .crom/
	if localCromEnv, err := config.LoadEnvVars(filepath.Join(wsDir, ".crom")); err == nil {
		for k, v := range localCromEnv.All() {
			env.Set(k, v)
		}
	}

	// Verifica se a chave foi sobrescrita
	if env.Get("API_KEY") != "crom_key" {
		t.Errorf("esperava 'crom_key', obteve '%s'", env.Get("API_KEY"))
	}
	// Verifica se a chave da raiz foi mantida
	if env.Get("OTHER_KEY") != "root_other" {
		t.Errorf("esperava 'root_other', obteve '%s'", env.Get("OTHER_KEY"))
	}
	// Verifica se a nova chave de crom existe
	if env.Get("NEW_KEY") != "crom_new" {
		t.Errorf("esperava 'crom_new', obteve '%s'", env.Get("NEW_KEY"))
	}
}

func TestMultiAgentManager_TopologyAgents(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	gDir, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("erro ao obter global dir: %v", err)
	}
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)

	env := &config.EnvVars{}
	_ = env.Save(gDir)

	wsDir := t.TempDir()
	_ = config.SaveWorkspaceConfig(wsDir, config.DefaultWorkspaceConfig("test-topo-proj"))

	cromDir := filepath.Join(wsDir, ".crom")
	_ = os.MkdirAll(cromDir, 0755)

	topoJSON := `{
		"supervisor": {
			"name": "supervisor",
			"provider": "mock",
			"model": "mock-model"
		},
		"specialists": [
			{
				"name": "custom_topo_specialist",
				"type": "native"
			}
		]
	}`
	err = os.WriteFile(filepath.Join(cromDir, "crom_agents.json"), []byte(topoJSON), 0644)
	if err != nil {
		t.Fatalf("erro ao salvar crom_agents.json: %v", err)
	}

	agents.RegisterAgent("custom_topo_specialist", func(cfg agents.Config) agentscore.Agent {
		return &mockAgent{name: "custom_topo_specialist"}
	})

	mgr := NewMultiAgentManager()
	_ = mgr.AddWorkspace("test-topo-proj", wsDir)

	ctx := context.Background()
	handler := &testEventHandler{done: make(chan struct{})}

	err = mgr.StartAgent(ctx, "test-topo-proj", "", "Test task", handler)
	if err != nil {
		t.Fatalf("erro ao iniciar agente: %v", err)
	}

	running := mgr.ListRunningAgents()
	if len(running) != 1 {
		t.Fatalf("esperava 1 agente rodando, obteve: %d", len(running))
	}

	loopTools := running[0].Loop.GetTools()
	found := false
	for _, tool := range loopTools {
		if tool.ID() == "custom_topo_specialist" {
			found = true
			break
		}
	}

	// Para o agente e aguarda finalização antes do cleanup do TempDir
	_ = mgr.StopAgent("test-topo-proj")
	select {
	case <-handler.done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout aguardando fim do agente")
	}
	time.Sleep(50 * time.Millisecond)

	if !found {
		t.Error("esperava encontrar a ferramenta custom_topo_specialist registrada no loop")
	}
}

type mockAgent struct {
	name string
}

func (m *mockAgent) Name() string { return m.name }
func (m *mockAgent) Description() string { return "Mock description" }
func (m *mockAgent) SystemPrompt() string { return "Mock prompt" }
func (m *mockAgent) ToolIDs() []string { return nil }
func (m *mockAgent) Execute(ctx context.Context, prompt string, prior string) (agentscore.AgentResult, error) {
	return agentscore.AgentResult{Success: true, Output: "Mock Output"}, nil
}

func TestMultiAgentManager_InjectUserMessage(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	gDir, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("erro ao obter global dir: %v", err)
	}
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)

	env := &config.EnvVars{}
	_ = env.Save(gDir)

	wsDir := t.TempDir()
	_ = config.SaveWorkspaceConfig(wsDir, config.DefaultWorkspaceConfig("test-inject-proj"))

	mgr := NewMultiAgentManager()
	_ = mgr.AddWorkspace("test-inject-proj", wsDir)

	ctx := context.Background()
	handler := &testEventHandler{done: make(chan struct{})}

	err = mgr.StartAgent(ctx, "test-inject-proj", "", "Tarefa de teste", handler)
	if err != nil {
		t.Fatalf("erro ao iniciar agente: %v", err)
	}

	// Tenta injetar mensagem do usuário
	err = mgr.InjectUserMessage("test-inject-proj", "Nova instrução em tempo real")
	if err != nil {
		t.Fatalf("erro ao injetar mensagem de usuário: %v", err)
	}

	// Se tentarmos injetar em um workspace inexistente, deve falhar
	err = mgr.InjectUserMessage("inexistente", "Oi")
	if err == nil {
		t.Fatal("esperava erro ao injetar em workspace inexistente, obteve nil")
	}

	_ = mgr.StopAgent("test-inject-proj")
	select {
	case <-handler.done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
	time.Sleep(50 * time.Millisecond)
}
