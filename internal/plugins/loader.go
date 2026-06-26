// Package plugins fornece a infraestrutura para carregar ferramentas compiladas
// separadamente como plugins Go (usando o pacote plugin nativo do Go).
//
// Para criar um plugin, compile um arquivo .so usando:
//
//	go build -buildmode=plugin -o meu_plugin.so meu_plugin.go
//
// O plugin deve exportar uma função chamada NewTool que retorne tools.Tool.
package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

// PluginLoader gerencia o carregamento de plugins externos de ferramentas
type PluginLoader struct {
	pluginDir string
}

// NewPluginLoader cria um novo carregador de plugins a partir de um diretório
func NewPluginLoader(pluginDir string) *PluginLoader {
	return &PluginLoader{pluginDir: pluginDir}
}

// LoadAll carrega todos os plugins .so do diretório configurado
func (pl *PluginLoader) LoadAll() ([]tools.Tool, error) {
	var loaded []tools.Tool

	entries, err := os.ReadDir(pl.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Sem plugins, sem erro
		}
		return nil, fmt.Errorf("falha ao ler diretório de plugins: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".so") {
			continue
		}

		path := filepath.Join(pl.pluginDir, entry.Name())
		tool, err := pl.loadPlugin(path)
		if err != nil {
			return nil, fmt.Errorf("falha ao carregar plugin %s: %w", entry.Name(), err)
		}

		loaded = append(loaded, tool)
	}

	return loaded, nil
}

// loadPlugin carrega um único plugin .so e extrai a ferramenta
func (pl *PluginLoader) loadPlugin(path string) (tools.Tool, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir plugin %s: %w", path, err)
	}

	sym, err := p.Lookup("NewTool")
	if err != nil {
		return nil, fmt.Errorf("plugin %s não exporta a função NewTool: %w", path, err)
	}

	newToolFn, ok := sym.(func() tools.Tool)
	if !ok {
		return nil, fmt.Errorf("plugin %s: NewTool tem assinatura incorreta, esperado func() tools.Tool", path)
	}

	tool := newToolFn()
	if tool == nil {
		return nil, fmt.Errorf("plugin %s: NewTool retornou nil", path)
	}

	return tool, nil
}

// LoadPluginsForWorkspace carrega plugins do diretório .crom/plugins/ dentro do workspace
func LoadPluginsForWorkspace(workspacePath string) ([]tools.Tool, error) {
	pluginDir := filepath.Join(workspacePath, ".crom", "plugins")
	loader := NewPluginLoader(pluginDir)
	return loader.LoadAll()
}
