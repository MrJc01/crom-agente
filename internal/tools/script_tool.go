package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScriptTool representa um script externo carregado dinamicamente no workspace
type ScriptTool struct {
	path string
	name string
}

// NewScriptTool cria uma nova instância de ScriptTool
func NewScriptTool(path, name string) *ScriptTool {
	return &ScriptTool{
		path: path,
		name: name,
	}
}

// ID retorna o identificador único da ferramenta
func (s *ScriptTool) ID() string {
	return s.name
}

// Description retorna a descrição legível da ferramenta
func (s *ScriptTool) Description() string {
	return fmt.Sprintf("Executa o script local %s com os argumentos fornecidos. Requer aprovação do usuário.", s.name)
}

// ParametersSchema retorna o JSON Schema dos parâmetros
func (s *ScriptTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"arguments": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Lista de argumentos de linha de comando para passar ao script"
			}
		},
		"required": ["arguments"]
	}`)
}

// RequiresApproval indica se a execução deve ser aprovada
func (s *ScriptTool) RequiresApproval() bool {
	return true
}

// Execute executa o script localmente e retorna a saída combinada de stdout e stderr
func (s *ScriptTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Arguments []string `json:"arguments"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, err
	}

	cmd := exec.CommandContext(ctx, s.path, input.Arguments...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("erro ao executar o script: %v. Saída: %s", err, string(output)),
		}, nil
	}

	return Result{
		Success: true,
		Data:    string(output),
	}, nil
}

// LoadScriptsFromDir lê todos os arquivos em um diretório e retorna como ferramentas dinâmicas
func LoadScriptsFromDir(dir string) ([]Tool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var loaded []Tool
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		name := entry.Name()
		toolName := strings.Split(name, ".")[0]
		toolName = strings.ReplaceAll(toolName, "-", "_")
		toolName = strings.ReplaceAll(toolName, " ", "_")

		// Ignora arquivos ocultos ou temporários
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		loaded = append(loaded, NewScriptTool(path, toolName))
	}
	return loaded, nil
}
