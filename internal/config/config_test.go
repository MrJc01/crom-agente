package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Testes da Configuração Global ---

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("esperado provider 'openai', obteve '%s'", cfg.DefaultProvider)
	}
	if cfg.MaxIterationsDefault != 0 {
		t.Fatalf("esperado 0 iterações (ilimitado), obteve %d", cfg.MaxIterationsDefault)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("esperado log level 'info', obteve '%s'", cfg.LogLevel)
	}
}

func TestLoadGlobalConfig_CreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig falhou: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("esperado provider 'openai', obteve '%s'", cfg.DefaultProvider)
	}

	// Arquivo JSON deve existir
	cfgPath := filepath.Join(dir, GlobalConfigFile)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatalf("arquivo global.json não foi criado")
	}
}

func TestSaveAndLoadGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &GlobalConfig{
		DefaultProvider:           "anthropic",
		DefaultModel:              "claude-sonnet-4-20250514",
		MaxIterationsDefault:      25,
		MaxConsecutiveFailDefault: 5,
		MaxTokensPerTaskDefault:   200000,
		ToolTimeoutSecondsDefault: 60,
		MaxMessageHistoryDefault:  60,
		LogLevel:                  "debug",
	}

	if err := SaveGlobalConfig(dir, cfg); err != nil {
		t.Fatalf("SaveGlobalConfig falhou: %v", err)
	}

	loaded, err := LoadGlobalConfig(dir)
	if err != nil {
		t.Fatalf("LoadGlobalConfig falhou: %v", err)
	}

	if loaded.DefaultProvider != "anthropic" {
		t.Fatalf("esperado 'anthropic', obteve '%s'", loaded.DefaultProvider)
	}
	if loaded.MaxIterationsDefault != 25 {
		t.Fatalf("esperado 25, obteve %d", loaded.MaxIterationsDefault)
	}
	if loaded.LogLevel != "debug" {
		t.Fatalf("esperado 'debug', obteve '%s'", loaded.LogLevel)
	}
}

// --- Testes do .env ---

func TestLoadEnvVars_EmptyWhenMissing(t *testing.T) {
	dir := t.TempDir()
	env, err := LoadEnvVars(dir)
	if err != nil {
		t.Fatalf("LoadEnvVars falhou: %v", err)
	}
	if len(env.All()) != 0 {
		t.Fatalf("esperado 0 variáveis, obteve %d", len(env.All()))
	}
}

func TestEnvVars_SetGetSave(t *testing.T) {
	dir := t.TempDir()
	env := &EnvVars{vars: make(map[string]string)}
	env.Set("OPENAI_API_KEY", "sk-test-123456789")
	env.Set("GEMINI_API_KEY", "AIza-test-key")

	if err := env.Save(dir); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	// Recarregar
	loaded, err := LoadEnvVars(dir)
	if err != nil {
		t.Fatalf("LoadEnvVars falhou: %v", err)
	}

	if loaded.Get("OPENAI_API_KEY") != "sk-test-123456789" {
		t.Fatalf("esperado 'sk-test-123456789', obteve '%s'", loaded.Get("OPENAI_API_KEY"))
	}
	if loaded.Get("GEMINI_API_KEY") != "AIza-test-key" {
		t.Fatalf("esperado 'AIza-test-key', obteve '%s'", loaded.Get("GEMINI_API_KEY"))
	}
}

func TestEnvVars_IgnoresCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	envContent := `# Este é um comentário
OPENAI_API_KEY=sk-real

# Outro comentário
GEMINI_API_KEY=AIza-real
`
	envPath := filepath.Join(dir, EnvFile)
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("falha ao gravar .env: %v", err)
	}

	env, err := LoadEnvVars(dir)
	if err != nil {
		t.Fatalf("LoadEnvVars falhou: %v", err)
	}

	all := env.All()
	if len(all) != 2 {
		t.Fatalf("esperado 2 variáveis, obteve %d: %v", len(all), all)
	}
	if env.Get("OPENAI_API_KEY") != "sk-real" {
		t.Fatalf("valor incorreto para OPENAI_API_KEY")
	}
}

func TestEnvVars_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	env := &EnvVars{vars: map[string]string{"SECRET": "value"}}
	if err := env.Save(dir); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, EnvFile))
	if err != nil {
		t.Fatalf("Stat falhou: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("esperado permissão 0600, obteve %o", perm)
	}
}

func TestMaskedValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "***"},
		{"12345678", "***"},
		{"sk-1234567890abcdef", "sk-1***********cdef"},
	}

	for _, tt := range tests {
		got := MaskedValue(tt.input)
		if got != tt.expected {
			t.Errorf("MaskedValue(%q) = %q, esperado %q", tt.input, got, tt.expected)
		}
	}
}

// --- Testes da Configuração de Workspace ---

func TestLoadWorkspaceConfig_CreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkspaceConfig falhou: %v", err)
	}

	if cfg.PermissionMode != "scoped" {
		t.Fatalf("esperado permission_mode 'scoped', obteve '%s'", cfg.PermissionMode)
	}
	if !cfg.WorkspaceJail {
		t.Fatal("esperado workspace_jail=true")
	}

	// Arquivo deve existir
	cfgPath := filepath.Join(dir, WorkspaceConfigDir, WorkspaceConfigFile)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatalf("arquivo config.json do workspace não foi criado")
	}
}

func TestSaveAndLoadWorkspaceConfig(t *testing.T) {
	dir := t.TempDir()
	maxIter := 30
	cfg := &WorkspaceConfig{
		WorkspaceName:   "meu-projeto",
		Provider:        "gemini",
		Model:           "gemini-pro",
		MaxIterations:   &maxIter,
		PermissionMode:  "total_access",
		WorkspaceJail:   false,
		AutoVerify:      true,
		AllowedTools:    []string{"read_file", "write_file"},
		BlockedCommands: []string{"rm -rf /"},
	}

	if err := SaveWorkspaceConfig(dir, cfg); err != nil {
		t.Fatalf("SaveWorkspaceConfig falhou: %v", err)
	}

	loaded, err := LoadWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkspaceConfig falhou: %v", err)
	}

	if loaded.WorkspaceName != "meu-projeto" {
		t.Fatalf("esperado 'meu-projeto', obteve '%s'", loaded.WorkspaceName)
	}
	if loaded.Provider != "gemini" {
		t.Fatalf("esperado 'gemini', obteve '%s'", loaded.Provider)
	}
	if *loaded.MaxIterations != 30 {
		t.Fatalf("esperado 30, obteve %d", *loaded.MaxIterations)
	}
	if loaded.PermissionMode != "total_access" {
		t.Fatalf("esperado 'total_access', obteve '%s'", loaded.PermissionMode)
	}
}

// --- Testes do Resolve (Merge de Precedência) ---

func TestResolve_GlobalOnlyDefaults(t *testing.T) {
	global := DefaultGlobalConfig()
	resolved := Resolve(global, nil, CLIFlags{})

	if resolved.Provider != "openai" {
		t.Fatalf("esperado 'openai', obteve '%s'", resolved.Provider)
	}
	if resolved.MaxIterations != 0 {
		t.Fatalf("esperado 0, obteve %d", resolved.MaxIterations)
	}
	if resolved.PermissionMode != "scoped" {
		t.Fatalf("esperado 'scoped', obteve '%s'", resolved.PermissionMode)
	}
}

func TestResolve_WorkspaceOverridesGlobal(t *testing.T) {
	global := DefaultGlobalConfig()
	maxIter := 25
	workspace := &WorkspaceConfig{
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-20250514",
		MaxIterations: &maxIter,
	}

	resolved := Resolve(global, workspace, CLIFlags{})

	if resolved.Provider != "anthropic" {
		t.Fatalf("esperado workspace override 'anthropic', obteve '%s'", resolved.Provider)
	}
	if resolved.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("esperado workspace override model, obteve '%s'", resolved.Model)
	}
	if resolved.MaxIterations != 25 {
		t.Fatalf("esperado workspace override 25, obteve %d", resolved.MaxIterations)
	}
	// Campos não overridden devem manter global defaults
	if resolved.MaxConsecutiveFail != 0 {
		t.Fatalf("esperado global default 0, obteve %d", resolved.MaxConsecutiveFail)
	}
}

func TestResolve_CLIFlagsOverrideAll(t *testing.T) {
	global := DefaultGlobalConfig()
	maxIter := 25
	workspace := &WorkspaceConfig{
		Provider:      "anthropic",
		MaxIterations: &maxIter,
	}
	cliMaxIter := 50
	flags := CLIFlags{
		Provider:      "ollama",
		MaxIterations: &cliMaxIter,
	}

	resolved := Resolve(global, workspace, flags)

	if resolved.Provider != "ollama" {
		t.Fatalf("esperado CLI override 'ollama', obteve '%s'", resolved.Provider)
	}
	if resolved.MaxIterations != 50 {
		t.Fatalf("esperado CLI override 50, obteve %d", resolved.MaxIterations)
	}
}

func TestResolve_FullHierarchy(t *testing.T) {
	// Global: openai, 15 iterações
	global := DefaultGlobalConfig()

	// Workspace: anthropic, 25 iterações, timeout 60
	wsIter := 25
	wsTimeout := 60
	workspace := &WorkspaceConfig{
		Provider:           "anthropic",
		MaxIterations:      &wsIter,
		ToolTimeoutSeconds: &wsTimeout,
		PermissionMode:     "total_access",
	}

	// CLI: apenas model override
	flags := CLIFlags{
		Model: "custom-model",
	}

	resolved := Resolve(global, workspace, flags)

	// Provider: workspace (anthropic) vence global (openai), CLI não setou
	if resolved.Provider != "anthropic" {
		t.Fatalf("esperado 'anthropic', obteve '%s'", resolved.Provider)
	}
	// Model: CLI (custom-model) vence todos
	if resolved.Model != "custom-model" {
		t.Fatalf("esperado 'custom-model', obteve '%s'", resolved.Model)
	}
	// MaxIterations: workspace (25), CLI não setou
	if resolved.MaxIterations != 25 {
		t.Fatalf("esperado 25, obteve %d", resolved.MaxIterations)
	}
	// ToolTimeout: workspace (60)
	if resolved.ToolTimeoutSeconds != 60 {
		t.Fatalf("esperado 60, obteve %d", resolved.ToolTimeoutSeconds)
	}
	// MaxConsecutiveFail: somente global (0)
	if resolved.MaxConsecutiveFail != 0 {
		t.Fatalf("esperado 0, obteve %d", resolved.MaxConsecutiveFail)
	}
	// PermissionMode: workspace (total_access)
	if resolved.PermissionMode != "total_access" {
		t.Fatalf("esperado 'total_access', obteve '%s'", resolved.PermissionMode)
	}
}

// --- Teste de JSON roundtrip ---

func TestGlobalConfig_JSONRoundtrip(t *testing.T) {
	cfg := DefaultGlobalConfig()
	cfg.DefaultProvider = "gemini"
	cfg.MaxIterationsDefault = 42

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("Marshal falhou: %v", err)
	}

	var loaded GlobalConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal falhou: %v", err)
	}

	if loaded.DefaultProvider != "gemini" || loaded.MaxIterationsDefault != 42 {
		t.Fatalf("roundtrip falhou: %+v", loaded)
	}
}
