package sdk

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crom/crom-agente/internal/orchestrator"
	pkgconfig "github.com/crom/crom-agente/pkg/config"
)

// AgentConfig define a parametrizacao do agente no SDK
type AgentConfig struct {
	AgentID     string   `json:"agent_id"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	Temperature float64  `json:"temperature"`
	SystemRules []string `json:"system_rules"`
}

// Manager coordena os agentes criados programaticamente
type Manager struct {
	cfg      pkgconfig.Config
	internal *orchestrator.MultiAgentManager
}

// NewManager cria uma nova instancia do Manager do SDK
func NewManager(cfg pkgconfig.Config) *Manager {
	return &Manager{
		cfg:      cfg,
		internal: orchestrator.NewMultiAgentManager(),
	}
}

// CreateAgent instancia um novo Agent programatico
func (m *Manager) CreateAgent(agentCfg AgentConfig) (*Agent, error) {
	workspaceName := agentCfg.AgentID
	workspacePath := m.cfg.WorkspaceRoot

	// Registra o workspace no MultiAgentManager
	_ = m.internal.AddWorkspace(workspaceName, workspacePath)

	// Garante que o diretorio do agente existe
	storageDir := filepath.Join(workspacePath, ".crom", "agents", agentCfg.AgentID)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("falha ao criar storage dir do agente: %w", err)
	}

	return &Agent{
		agentID:   agentCfg.AgentID,
		config:    agentCfg,
		manager:   m,
		workspace: workspaceName,
		path:      workspacePath,
	}, nil
}
