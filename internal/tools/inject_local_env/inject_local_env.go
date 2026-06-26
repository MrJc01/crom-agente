package inject_local_env

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de inject_local_env: " + err.Error())
	}
}

type InjectLocalEnvTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewInjectLocalEnvTool(workspaceRoot string, workspaceJail bool) *InjectLocalEnvTool {
	return &InjectLocalEnvTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *InjectLocalEnvTool) ID() string { return metadata.ID }

func (t *InjectLocalEnvTool) Description() string { return metadata.Description }

func (t *InjectLocalEnvTool) RequiresApproval() bool { return false }

func (t *InjectLocalEnvTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"envs": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				},
				"description": "Dicionário de variáveis de ambiente chave-valor a injetar"
			},
			"env_file": {
				"type": "string",
				"description": "Caminho relativo ou absoluto para um arquivo contendo variáveis no formato KEY=VALUE"
			}
		},
		"required": []
	}`)
}

func (t *InjectLocalEnvTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Envs    map[string]string `json:"envs"`
		EnvFile string            `json:"env_file"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	injected := 0

	// 1. Injeta dicionário direto
	for k, v := range input.Envs {
		if err := os.Setenv(k, v); err == nil {
			injected++
		}
	}

	// 2. Injeta a partir do arquivo .env
	if input.EnvFile != "" {
		filePath := input.EnvFile
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(t.workspaceRoot, filePath)
		}

		if t.workspaceJail {
			rel, err := filepath.Rel(t.workspaceRoot, filePath)
			if err != nil || strings.HasPrefix(rel, "..") {
				return tools.Result{Success: false, Error: "acesso negado: fora do workspace"}, nil
			}
		}

		file, err := os.Open(filePath)
		if err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("falha ao abrir arquivo env: %s", err)}, nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				if err := os.Setenv(k, v); err == nil {
					injected++
				}
			}
		}
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("✓ Injetadas %d variáveis de ambiente com sucesso.", injected),
	}, nil
}
