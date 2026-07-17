package write_file

import (
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
		panic("falha ao carregar metadados de write_file: " + err.Error())
	}
}

// WriteFileTool grava ou sobrescreve arquivos dentro do workspace
type WriteFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewWriteFileTool cria a ferramenta write_file
func NewWriteFileTool(workspaceRoot string, jail bool) *WriteFileTool {
	return &WriteFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *WriteFileTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição legível
func (t *WriteFileTool) Description() string {
	return metadata.Description
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *WriteFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo para gravar"
			},
			"content": {
				"type": "string",
				"description": "Conteúdo textual completo a ser gravado no arquivo"
			}
		},
		"required": ["path", "content"]
	}`)
}

// RequiresApproval define se a ferramenta precisa de HITL
func (t *WriteFileTool) RequiresApproval() bool {
	return true
}

// Execute roda a escrita do arquivo
func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	// Em projeto Godot, editar .tscn como texto conflita com a edição da cena
	// viva pelas ferramentas godot_* (gera nós órfãos, unique_id inválido, script
	// errado na raiz). Bloqueia e orienta a usar as ferramentas do editor.
	if strings.HasSuffix(strings.ToLower(targetFile), ".tscn") {
		if _, e := os.Stat(filepath.Join(t.workspaceRoot, "project.godot")); e == nil {
			return tools.Result{Success: false, Error: "Não edite arquivos .tscn com write_file num projeto Godot — isso corrompe a cena. Use as ferramentas do editor: godot_create_scene, godot_add_node, godot_set_node_property, godot_connect_signal, godot_save_scene."}, nil
		}
	}

	if err := tools.EnsureDir(filepath.Dir(targetFile)); err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	if err := os.WriteFile(targetFile, []byte(input.Content), 0644); err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao gravar arquivo: %s", err.Error())}, nil
	}

	return tools.Result{Success: true, Data: "Arquivo gravado com sucesso."}, nil
}
