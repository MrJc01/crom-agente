package read_file

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
		panic("falha ao carregar metadados de read_file: " + err.Error())
	}
}

// ReadFileTool lê arquivos de texto dentro do workspace
type ReadFileTool struct {
	workspaceRoot string
	jail          bool
}

// NewReadFileTool cria a ferramenta read_file
func NewReadFileTool(workspaceRoot string, jail bool) *ReadFileTool {
	return &ReadFileTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *ReadFileTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição legível
func (t *ReadFileTool) Description() string {
	return metadata.Description
}

// ParametersSchema define a assinatura JSON Schema da ferramenta
func (t *ReadFileTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo a ser lido"
			}
		},
		"required": ["path"]
	}`)
}

// RequiresApproval define se a ferramenta precisa de HITL
func (t *ReadFileTool) RequiresApproval() bool {
	return false
}

// Execute roda a leitura do arquivo
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	// Task 9.6: Evitar leitura de arquivos binários ou que quebram o context window
	ext := strings.ToLower(filepath.Ext(targetFile))
	blockedExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".svg": true, ".webp": true,
		".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".mp4": true, ".mp3": true, ".wav": true,
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".pyc": true, ".pyo": true,
		".class": true, ".jar": true, ".war": true, ".ttf": true, ".woff": true, ".woff2": true, ".eot": true,
	}
	if blockedExts[ext] {
		return tools.Result{Success: false, Error: fmt.Sprintf("Você não pode ler o arquivo '%s' porque ele tem uma extensão binária bloqueada (%s).", input.Path, ext)}, nil
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo: %s", err.Error())}, nil
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 800 {
		return tools.Result{
			Success: false, 
			Error: fmt.Sprintf("O arquivo '%s' possui %d linhas (maior que o limite de 800 linhas). O uso indiscriminado do `read_file` em arquivos gigantes quebra o contexto. Por favor, use `grep` ou leia o código fonte do repositório em blocos menores.", input.Path, len(lines)),
		}, nil
	}

	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	cleanedData := strings.Join(lines, "\n")

	return tools.Result{Success: true, Data: cleanedData}, nil
}
