package edit_file_by_line

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
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
		panic("falha ao carregar metadados de edit_file_by_line: " + err.Error())
	}
}

type EditFileByLineTool struct {
	workspaceRoot string
	jail          bool
}

func NewEditFileByLineTool(workspaceRoot string, jail bool) *EditFileByLineTool {
	return &EditFileByLineTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *EditFileByLineTool) ID() string {
	return metadata.ID
}

func (t *EditFileByLineTool) Description() string {
	return metadata.Description
}

func (t *EditFileByLineTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo"
			},
			"start_line": {
				"type": "integer",
				"description": "Linha inicial a ser substituída (1-indexed, inclusiva)"
			},
			"end_line": {
				"type": "integer",
				"description": "Linha final a ser substituída (1-indexed, inclusiva)"
			},
			"replacement_content": {
				"type": "string",
				"description": "O novo conteúdo que vai entrar no lugar do bloco de linhas"
			}
		},
		"required": ["path", "start_line", "end_line", "replacement_content"]
	}`)
}

func (t *EditFileByLineTool) RequiresApproval() bool {
	return false
}

func (t *EditFileByLineTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path               string `json:"path"`
		StartLine          int    `json:"start_line"`
		EndLine            int    `json:"end_line"`
		ReplacementContent string `json:"replacement_content"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.StartLine < 1 || input.EndLine < input.StartLine {
		return tools.Result{Success: false, Error: "intervalo de linhas inválido. start_line deve ser >= 1 e end_line >= start_line"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo: %v", err)}, nil
	}

	// Backup antes da substituição para suportar o rollback contextual
	_ = os.WriteFile(targetFile+".bak", data, 0644)

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	if input.StartLine > totalLines {
		return tools.Result{Success: false, Error: fmt.Sprintf("start_line (%d) é maior que o total de linhas do arquivo (%d)", input.StartLine, totalLines)}, nil
	}

	if input.EndLine > totalLines {
		input.EndLine = totalLines
	}

	var newContent strings.Builder
	for i := 0; i < input.StartLine-1; i++ {
		newContent.WriteString(lines[i])
		newContent.WriteString("\n")
	}

	newContent.WriteString(input.ReplacementContent)
	// Add newline se o replacement não tiver e não for o fim do arquivo, ou simplesmente injetamos o replacement literal.
	// Vamos forçar que a formatação não quebre arquivos
	if len(input.ReplacementContent) > 0 && !strings.HasSuffix(input.ReplacementContent, "\n") {
		if input.EndLine < totalLines {
			newContent.WriteString("\n")
		}
	}

	for i := input.EndLine; i < totalLines; i++ {
		newContent.WriteString(lines[i])
		if i < totalLines-1 {
			newContent.WriteString("\n")
		}
	}

	err = os.WriteFile(targetFile, []byte(newContent.String()), 0644)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao escrever no arquivo: %v", err)}, nil
	}

	return tools.Result{Success: true, Data: fmt.Sprintf("substituição por linha efetuada com sucesso em '%s'", input.Path)}, nil
}
