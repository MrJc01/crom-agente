package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// DefaultGlobalDir é o diretório global padrão (~/.crom/)
	DefaultGlobalDir = ".crom"

	// GlobalConfigFile é o arquivo JSON de configuração global
	GlobalConfigFile = "global.json"

	// EnvFile é o arquivo .env de segredos
	EnvFile = ".env"

	// WorkspacesFile lista os workspaces registrados
	WorkspacesFile = "workspaces.json"

	// WorkspaceConfigDir é o diretório .crom dentro de cada workspace
	WorkspaceConfigDir = ".crom"

	// WorkspaceConfigFile é o arquivo de config por workspace
	WorkspaceConfigFile = "config.json"
)

// MCPServerConfig descreve a configuração de um servidor MCP externo
// Pode ser um subprocesso (via Command+Args) ou um servidor remoto SSE (via URL)
type MCPServerConfig struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"` // ex: "npx", "python"
	Args    []string `json:"args,omitempty"`    // ex: ["-y", "@modelcontextprotocol/server-filesystem"]
	URL     string   `json:"url,omitempty"`     // URL do servidor SSE remoto (alternativa ao Command)
	Env     []string `json:"env,omitempty"`     // Variáveis de ambiente extras: ["KEY=VALUE"]
}

// GlobalConfig contém configurações globais compartilhadas por todos os workspaces
type GlobalConfig struct {
	DefaultProvider                  string            `json:"default_provider"`
	DefaultModel                     string            `json:"default_model"`
	MaxIterationsDefault             int               `json:"max_iterations_default"`
	MaxConsecutiveFailDefault        int               `json:"max_consecutive_failures_default"`
	MaxTokensPerTaskDefault          int               `json:"max_tokens_per_task_default"`
	ToolTimeoutSecondsDefault        int               `json:"tool_timeout_seconds_default"`
	MaxMessageHistoryDefault         int               `json:"max_message_history_default"`
	LogLevel                         string            `json:"log_level"`
	TelemetryEnabled                 bool              `json:"telemetry_enabled"`
	DisablePromptOptimizationDefault bool              `json:"disable_prompt_optimization_default"`
	MCPServers                       []MCPServerConfig `json:"mcp_servers,omitempty"` // Servidores MCP globais
}

// EnvVars contém variáveis carregadas do .env (segredos)
type EnvVars struct {
	mu   sync.RWMutex
	vars map[string]string
}

// WorkspaceConfig contém configurações específicas de um workspace
type WorkspaceConfig struct {
	WorkspaceName             string   `json:"workspace_name"`
	Provider                  string   `json:"provider,omitempty"`
	Model                     string   `json:"model,omitempty"`
	MaxIterations             *int     `json:"max_iterations,omitempty"`
	MaxConsecutiveFail        *int     `json:"max_consecutive_failures,omitempty"`
	MaxTokensPerTask          *int     `json:"max_tokens_per_task,omitempty"`
	ToolTimeoutSeconds        *int     `json:"tool_timeout_seconds,omitempty"`
	MaxMessageHistory         *int     `json:"max_message_history,omitempty"`
	PermissionMode            string   `json:"permission_mode"`
	WorkspaceJail             bool     `json:"workspace_jail"`
	AllowedTools              []string `json:"allowed_tools,omitempty"`
	BlockedCommands           []string `json:"blocked_commands,omitempty"`
	AutoVerify                bool     `json:"auto_verify"`
	AutoSelfCheck             bool     `json:"auto_self_check"`
	BrowserHeadless           *bool    `json:"browser_headless,omitempty"`
	DisablePromptOptimization *bool    `json:"disable_prompt_optimization,omitempty"`
}

// ResolvedConfig é o resultado do merge de todas as camadas de configuração
type ResolvedConfig struct {
	Provider                  string
	Model                     string
	MaxIterations             int
	MaxConsecutiveFail        int
	MaxTokensPerTask          int
	ToolTimeoutSeconds        int
	MaxMessageHistory         int
	PermissionMode            string
	WorkspaceJail             bool
	AutoVerify                bool
	AutoSelfCheck             bool
	AllowedTools              []string
	BlockedCommands           []string
	LogLevel                  string
	BrowserHeadless           bool
	DisablePromptOptimization bool
}


// CLIFlags contém flags passados via linha de comando (prioridade máxima)
type CLIFlags struct {
	Provider                  string
	Model                     string
	MaxIterations             *int
	MaxConsecutiveFail        *int
	ToolTimeoutSeconds        *int
	MaxMessageHistory         *int
	PermissionMode            string
	DisablePromptOptimization *bool
}

// --- Defaults ---

// DefaultGlobalConfig retorna uma configuração global com valores padrão sensatos
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		DefaultProvider:                  "openai",
		DefaultModel:                     "gpt-4o",
		MaxIterationsDefault:             15,
		MaxConsecutiveFailDefault:        3,
		MaxTokensPerTaskDefault:          100000,
		ToolTimeoutSecondsDefault:        30,
		MaxMessageHistoryDefault:         40,
		LogLevel:                         "info",
		TelemetryEnabled:                 false,
		DisablePromptOptimizationDefault: false,
	}
}

// DefaultWorkspaceConfig retorna uma configuração de workspace com valores padrão
func DefaultWorkspaceConfig(name string) *WorkspaceConfig {
	return &WorkspaceConfig{
		WorkspaceName:  name,
		PermissionMode: "scoped",
		WorkspaceJail:  true,
		AutoVerify:     true,
		AutoSelfCheck:  true,
	}
}

// --- Loaders ---

// GlobalDir retorna o caminho do diretório global (~/.crom/)
func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("erro ao obter home dir: %w", err)
	}
	return filepath.Join(home, DefaultGlobalDir), nil
}

// LoadGlobalConfig carrega ou cria a configuração global
func LoadGlobalConfig(globalDir string) (*GlobalConfig, error) {
	cfgPath := filepath.Join(globalDir, GlobalConfigFile)

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultGlobalConfig()
			if saveErr := SaveGlobalConfig(globalDir, cfg); saveErr != nil {
				return nil, fmt.Errorf("erro ao criar config global padrão: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("erro ao ler config global: %w", err)
	}

	cfg := DefaultGlobalConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("erro ao parsear config global: %w", err)
	}
	return cfg, nil
}

// SaveGlobalConfig persiste a configuração global no disco
func SaveGlobalConfig(globalDir string, cfg *GlobalConfig) error {
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório global: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config global: %w", err)
	}

	cfgPath := filepath.Join(globalDir, GlobalConfigFile)
	return os.WriteFile(cfgPath, data, 0644)
}

// LoadEnvVars carrega variáveis do arquivo .env
func LoadEnvVars(globalDir string) (*EnvVars, error) {
	envPath := filepath.Join(globalDir, EnvFile)
	env := &EnvVars{vars: make(map[string]string)}

	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return env, nil // .env ainda não existe, retorna vazio
		}
		return nil, fmt.Errorf("erro ao ler .env: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			env.vars[key] = value
		}
	}

	return env, nil
}

// Get retorna o valor de uma variável do .env
func (e *EnvVars) Get(key string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.vars[key]
}

// Set define uma variável no .env
func (e *EnvVars) Set(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vars[key] = value
}

// All retorna todas as variáveis (cópia)
func (e *EnvVars) All() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make(map[string]string, len(e.vars))
	for k, v := range e.vars {
		cp[k] = v
	}
	return cp
}

// Save persiste as variáveis no arquivo .env
func (e *EnvVars) Save(globalDir string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório global: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("# crom-agente environment variables\n")
	builder.WriteString("# Este arquivo contém segredos — NÃO versione!\n\n")
	for key, value := range e.vars {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	envPath := filepath.Join(globalDir, EnvFile)
	return os.WriteFile(envPath, []byte(builder.String()), 0600) // 0600: apenas o dono pode ler
}

// MaskedValue retorna o valor mascarado de uma variável (para exibição segura)
func MaskedValue(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

// LoadWorkspaceConfig carrega ou cria a configuração de um workspace
func LoadWorkspaceConfig(workspacePath string) (*WorkspaceConfig, error) {
	cfgDir := filepath.Join(workspacePath, WorkspaceConfigDir)
	cfgPath := filepath.Join(cfgDir, WorkspaceConfigFile)

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			name := filepath.Base(workspacePath)
			cfg := DefaultWorkspaceConfig(name)
			
			// Tenta ler o global config para preencher o provider e model iniciais
			if gDir, gErr := GlobalDir(); gErr == nil {
				if gCfg, gCfgErr := LoadGlobalConfig(gDir); gCfgErr == nil && gCfg != nil {
					cfg.Provider = gCfg.DefaultProvider
					cfg.Model = gCfg.DefaultModel
				}
			}
			
			if saveErr := SaveWorkspaceConfig(workspacePath, cfg); saveErr != nil {
				return nil, fmt.Errorf("erro ao criar config workspace padrão: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("erro ao ler config workspace: %w", err)
	}

	cfg := &WorkspaceConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("erro ao parsear config workspace: %w", err)
	}
	return cfg, nil
}

// SaveWorkspaceConfig persiste a configuração de um workspace no disco
func SaveWorkspaceConfig(workspacePath string, cfg *WorkspaceConfig) error {
	cfgDir := filepath.Join(workspacePath, WorkspaceConfigDir)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório de workspace: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar config workspace: %w", err)
	}

	cfgPath := filepath.Join(cfgDir, WorkspaceConfigFile)
	return os.WriteFile(cfgPath, data, 0644)
}

// --- Resolve ---

// Resolve aplica a hierarquia de precedência para gerar a configuração efetiva:
// CLI Flags > Workspace config > Global config > Hardcoded defaults
func Resolve(global *GlobalConfig, workspace *WorkspaceConfig, flags CLIFlags) *ResolvedConfig {
	resolved := &ResolvedConfig{
		// Base: defaults globais
		Provider:                  global.DefaultProvider,
		Model:                     global.DefaultModel,
		MaxIterations:             global.MaxIterationsDefault,
		MaxConsecutiveFail:        global.MaxConsecutiveFailDefault,
		MaxTokensPerTask:          global.MaxTokensPerTaskDefault,
		ToolTimeoutSeconds:        global.ToolTimeoutSecondsDefault,
		MaxMessageHistory:         global.MaxMessageHistoryDefault,
		LogLevel:                  global.LogLevel,
		PermissionMode:            "scoped",
		WorkspaceJail:             true,
		AutoVerify:                true,
		AutoSelfCheck:             true,
		BrowserHeadless:           true, // por padrão roda de fundo
		DisablePromptOptimization: global.DisablePromptOptimizationDefault,
	}

	// Camada 2: Workspace overrides
	if workspace != nil {
		if workspace.Provider != "" {
			resolved.Provider = workspace.Provider
		}
		if workspace.Model != "" {
			resolved.Model = workspace.Model
		}
		if workspace.MaxIterations != nil {
			resolved.MaxIterations = *workspace.MaxIterations
		}
		if workspace.MaxConsecutiveFail != nil {
			resolved.MaxConsecutiveFail = *workspace.MaxConsecutiveFail
		}
		if workspace.MaxTokensPerTask != nil {
			resolved.MaxTokensPerTask = *workspace.MaxTokensPerTask
		}
		if workspace.ToolTimeoutSeconds != nil {
			resolved.ToolTimeoutSeconds = *workspace.ToolTimeoutSeconds
		}
		if workspace.MaxMessageHistory != nil {
			resolved.MaxMessageHistory = *workspace.MaxMessageHistory
		}
		if workspace.PermissionMode != "" {
			resolved.PermissionMode = workspace.PermissionMode
		}
		resolved.WorkspaceJail = workspace.WorkspaceJail
		resolved.AutoVerify = workspace.AutoVerify
		resolved.AutoSelfCheck = workspace.AutoSelfCheck
		resolved.AllowedTools = workspace.AllowedTools
		resolved.BlockedCommands = workspace.BlockedCommands
		if workspace.BrowserHeadless != nil {
			resolved.BrowserHeadless = *workspace.BrowserHeadless
		}
		if workspace.DisablePromptOptimization != nil {
			resolved.DisablePromptOptimization = *workspace.DisablePromptOptimization
		}
	}


	// Camada 3: CLI Flags (prioridade máxima)
	if flags.Provider != "" {
		resolved.Provider = flags.Provider
	}
	if flags.Model != "" {
		resolved.Model = flags.Model
	}
	if flags.MaxIterations != nil {
		resolved.MaxIterations = *flags.MaxIterations
	}
	if flags.MaxConsecutiveFail != nil {
		resolved.MaxConsecutiveFail = *flags.MaxConsecutiveFail
	}
	if flags.ToolTimeoutSeconds != nil {
		resolved.ToolTimeoutSeconds = *flags.ToolTimeoutSeconds
	}
	if flags.MaxMessageHistory != nil {
		resolved.MaxMessageHistory = *flags.MaxMessageHistory
	}
	if flags.PermissionMode != "" {
		resolved.PermissionMode = flags.PermissionMode
	}
	if flags.DisablePromptOptimization != nil {
		resolved.DisablePromptOptimization = *flags.DisablePromptOptimization
	}

	return resolved
}
