package sdk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/tools"
	pkgconfig "github.com/crom/crom-agente/pkg/config"
)

type dummyTool struct{}

func (d *dummyTool) ID() string {
	return "dummy"
}

func (d *dummyTool) Description() string {
	return "ferramenta dummy para teste"
}

func (d *dummyTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (d *dummyTool) RequiresApproval() bool {
	return false
}

func (d *dummyTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Success: true, Data: "dummy success"}, nil
}

func TestSDK_E2E(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Inicializa os arquivos de config globais necessarios
	gDir, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("erro ao obter global dir: %v", err)
	}
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock" // utiliza provider mock offline
	_ = config.SaveGlobalConfig(gDir, gCfg)

	env := &config.EnvVars{}
	_ = env.Save(gDir)

	wsDir := t.TempDir()
	// Configura o workspace config
	_ = config.SaveWorkspaceConfig(wsDir, config.DefaultWorkspaceConfig("meu-agente"))

	// 1. Inicializa o Manager do SDK
	sdkConfig := pkgconfig.Config{
		WorkspaceRoot:  wsDir,
		StoragePath:    filepath.Join(wsDir, ".crom"),
		PermissionMode: pkgconfig.ModeTotalAccess,
	}
	manager := NewManager(sdkConfig)

	// 2. Cria o agente
	agentConfig := AgentConfig{
		AgentID:  "meu-agente",
		Provider: "mock",
		Model:    "test-model",
	}

	agent, err := manager.CreateAgent(agentConfig)
	if err != nil {
		t.Fatalf("erro ao criar agente: %v", err)
	}

	// 3. Registra ferramenta customizada
	agent.RegisterTool(&dummyTool{})

	// 4. Executa tarefa
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := agent.ExecuteTask(ctx, "Ola")
	if err != nil {
		t.Fatalf("erro ao executar tarefa: %v", err)
	}

	if result.Status != "finished" && result.Status != "idle" {
		t.Errorf("esperava status finished ou idle, obteve: %q", result.Status)
	}

	// Aguarda a goroutine desregistrar do manager para evitar conflito na remoção do diretório temporário
	for i := 0; i < 50; i++ {
		if len(manager.internal.ListRunningAgents()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSDK_Session(t *testing.T) {
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
	_ = config.SaveWorkspaceConfig(wsDir, config.DefaultWorkspaceConfig("meu-agente"))

	sdkConfig := pkgconfig.Config{
		WorkspaceRoot:  wsDir,
		StoragePath:    filepath.Join(wsDir, ".crom"),
		PermissionMode: pkgconfig.ModeTotalAccess,
	}
	manager := NewManager(sdkConfig)

	agentConfig := AgentConfig{
		AgentID:  "meu-agente",
		Provider: "mock",
		Model:    "test-model",
	}

	agent, err := manager.CreateAgent(agentConfig)
	if err != nil {
		t.Fatalf("erro ao criar agente: %v", err)
	}

	// Define o nome da sessão no SDK Agent
	sessionName := "sdk-session-abc"
	agent.SessionName = sessionName

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = agent.ExecuteTask(ctx, "Ola Session")
	if err != nil {
		t.Fatalf("erro ao executar tarefa: %v", err)
	}

	// Verifica se a sessão correspondente foi salva
	sessionFile := filepath.Join(wsDir, ".crom", "sessions", sessionName, "session.json")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Fatalf("esperava arquivo de sessão criado em %s, mas não existe", sessionFile)
	}

	// Aguarda a goroutine desregistrar do manager para evitar conflito na remoção do diretório temporário
	for i := 0; i < 50; i++ {
		if len(manager.internal.ListRunningAgents()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
