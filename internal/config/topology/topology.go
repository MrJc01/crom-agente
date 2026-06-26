package topology

import (
	"encoding/json"
	"os"
)

// SpecialistConfig define a configuração de um subagente especialista
type SpecialistConfig struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // "native", "mcp", "external"
	Description  string   `json:"description,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	ExecPath     string   `json:"exec_path,omitempty"` // para "external" ou "mcp"
	Args         []string `json:"args,omitempty"`
	URL          string   `json:"url,omitempty"` // para "mcp" SSE
	Model        string   `json:"model,omitempty"` // opcional se quiser sobrescrever o modelo padrão
	ToolIDs      []string `json:"tool_ids,omitempty"`
}

// SupervisorConfig define a configuração do Supervisor principal
type SupervisorConfig struct {
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// TopologyConfig representa a estrutura de definição dos agentes no CROM-Agente
type TopologyConfig struct {
	Supervisor  SupervisorConfig   `json:"supervisor"`
	Specialists []SpecialistConfig `json:"specialists"`
}

// LoadTopology carrega a topologia a partir de um JSON no disco, expandindo variáveis de ambiente
func LoadTopology(path string) (*TopologyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expandir variáveis de ambiente
	expanded := os.ExpandEnv(string(data))

	var topo TopologyConfig
	if err := json.Unmarshal([]byte(expanded), &topo); err != nil {
		return nil, err
	}

	return &topo, nil
}

// GetSpecialists retorna a lista de especialistas configurados
func (t *TopologyConfig) GetSpecialists() []SpecialistConfig {
	return t.Specialists
}

// GetDefaultTopology retorna uma topologia padrão fallback caso não exista configuração local
func GetDefaultTopology() *TopologyConfig {
	return &TopologyConfig{
		Supervisor: SupervisorConfig{
			Model: "",
		},
		Specialists: []SpecialistConfig{
			{
				Name:        "browser",
				Type:        "native",
				Description: "Especialista em navegação web e automação visual de navegadores",
				ToolIDs:     []string{"scraper", "http_client"},
			},
			{
				Name:        "spawn",
				Type:        "native",
				Description: "Especialista em executar tarefas assíncronas isoladas e scripts locais",
				ToolIDs:     []string{"terminal_command", "read_file", "write_file"},
			},
			{
				Name:        "reasoning",
				Type:        "native",
				Description: "Especialista em raciocínio lógico avançado e loop de pensamento sequencial puro (no-tools)",
				ToolIDs:     []string{},
			},
		},
	}
}

// SaveTopology persiste a topologia no disco em formato JSON
func SaveTopology(path string, topo *TopologyConfig) error {
	data, err := json.MarshalIndent(topo, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}


