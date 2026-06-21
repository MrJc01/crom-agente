package config

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

//go:embed assets/default_prompts.json
var defaultPromptsJSON []byte

// PromptTemplate represents a single prompt configuration
type PromptTemplate struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

// PromptsConfig is the struct matching prompts.json
type PromptsConfig struct {
	Version   string                    `json:"version"`
	Prompts   map[string]PromptTemplate `json:"prompts"`
	Overrides map[string]struct {
		Content string `json:"content"`
	} `json:"overrides"`
}

// PromptManager handles loading and merging prompts
type PromptManager struct {
	mu            sync.RWMutex
	globalPath    string
	workspacePath string
	config        PromptsConfig
}

// NewPromptManager creates a PromptManager and loads configurations
func NewPromptManager(workspaceDir string) *PromptManager {
	home, _ := os.UserHomeDir()
	pm := &PromptManager{
		globalPath:    filepath.Join(home, ".crom", "prompts.json"),
		workspacePath: filepath.Join(workspaceDir, ".crom", "prompts.json"),
		config: PromptsConfig{
			Version:   "1.0",
			Prompts:   make(map[string]PromptTemplate),
			Overrides: make(map[string]struct {
				Content string `json:"content"`
			}),
		},
	}
	pm.loadDefaults()
	pm.Load()
	return pm
}

func (pm *PromptManager) loadDefaults() {
	if err := json.Unmarshal(defaultPromptsJSON, &pm.config); err != nil {
		// Log erro de fallback
	}
	
	// Garantir que os mapas existem mesmo se o JSON estiver vazio
	if pm.config.Prompts == nil {
		pm.config.Prompts = make(map[string]PromptTemplate)
	}
	if pm.config.Overrides == nil {
		pm.config.Overrides = make(map[string]struct{ Content string `json:"content"` })
	}
}

// Load reads the prompts from disk
func (pm *PromptManager) Load() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Load global overrides
	pm.loadFile(pm.globalPath)
	// Load workspace overrides
	if pm.workspacePath != "" {
		pm.loadFile(pm.workspacePath)
	}
}

func (pm *PromptManager) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var fileConfig PromptsConfig
	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return
	}

	for k, v := range fileConfig.Prompts {
		pm.config.Prompts[k] = v
	}

	for k, v := range fileConfig.Overrides {
		if p, ok := pm.config.Prompts[k]; ok {
			p.Content = v.Content
			pm.config.Prompts[k] = p
		}
	}
}

// GetPrompt returns a specific prompt by key
func (pm *PromptManager) GetPrompt(key string) (PromptTemplate, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.config.Prompts[key]
	return p, ok
}

// GetAllEnabled returns all enabled default prompts
func (pm *PromptManager) GetAllEnabled() []PromptTemplate {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var enabled []PromptTemplate
	keys := []string{"agentic_identity", "port_conflict", "planning_requirement", "tool_usage", "file_impact", "screenshot_path"}
	for _, k := range keys {
		if p, ok := pm.config.Prompts[k]; ok && p.Enabled {
			enabled = append(enabled, p)
		}
	}
	
	// Add any custom prompts
	for k, p := range pm.config.Prompts {
		found := false
		for _, defaultKey := range keys {
			if k == defaultKey {
				found = true
				break
			}
		}
		if !found && p.Enabled && k != "phase_planning" && k != "phase_execution" {
			enabled = append(enabled, p)
		}
	}
	return enabled
}
