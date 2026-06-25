package topology_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crom/crom-agente/internal/config/topology"
)

func TestLoadTopology_Valid(t *testing.T) {
	tempDir := t.TempDir()
	jsonContent := `{
		"supervisor": {
			"model": "gpt-4"
		},
		"specialists": [
			{
				"name": "tester",
				"type": "native",
				"description": "Runs tests",
				"tool_ids": ["run_tests"]
			}
		]
	}`

	path := filepath.Join(tempDir, "crom_agents.json")
	if err := os.WriteFile(path, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("falha ao criar arquivo de teste: %v", err)
	}

	topo, err := topology.LoadTopology(path)
	if err != nil {
		t.Fatalf("LoadTopology falhou inesperadamente: %v", err)
	}

	if topo.Supervisor.Model != "gpt-4" {
		t.Errorf("esperava model gpt-4, obteve '%s'", topo.Supervisor.Model)
	}

	if len(topo.Specialists) != 1 || topo.Specialists[0].Name != "tester" {
		t.Errorf("lista de especialistas incorreta: %+v", topo.Specialists)
	}
}

func TestLoadTopology_EnvExpansion(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("TEST_AGENT_MODEL", "gemini-flash")
	defer os.Unsetenv("TEST_AGENT_MODEL")

	jsonContent := `{
		"supervisor": {
			"model": "$TEST_AGENT_MODEL"
		},
		"specialists": []
	}`

	path := filepath.Join(tempDir, "crom_agents.json")
	if err := os.WriteFile(path, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("falha ao criar arquivo de teste: %v", err)
	}

	topo, err := topology.LoadTopology(path)
	if err != nil {
		t.Fatalf("LoadTopology falhou: %v", err)
	}

	if topo.Supervisor.Model != "gemini-flash" {
		t.Errorf("esperava expansao para 'gemini-flash', obteve '%s'", topo.Supervisor.Model)
	}
}

func TestLoadTopology_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "crom_agents.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("falha ao criar arquivo de teste: %v", err)
	}

	_, err := topology.LoadTopology(path)
	if err == nil {
		t.Error("esperava erro ao carregar JSON malformado")
	}
}

func TestGetDefaultTopology(t *testing.T) {
	topo := topology.GetDefaultTopology()
	if len(topo.Specialists) < 2 {
		t.Errorf("esperava pelo menos 2 especialistas na topologia default")
	}
}
