package error_histogram

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de error_histogram: " + err.Error())
	}
}

type ErrorHistogramTool struct {
	workspaceRoot string
	stateManager  *state.StateManager
}

func NewErrorHistogramTool(workspaceRoot string, sm *state.StateManager) *ErrorHistogramTool {
	return &ErrorHistogramTool{
		workspaceRoot: workspaceRoot,
		stateManager:  sm,
	}
}

func (t *ErrorHistogramTool) ID() string { return metadata.ID }

func (t *ErrorHistogramTool) Description() string { return metadata.Description }

func (t *ErrorHistogramTool) RequiresApproval() bool { return false }

func (t *ErrorHistogramTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

type FileHistogram struct {
	FilePath string `json:"file_path"`
	Errors   int    `json:"errors"`
	Type     string `json:"type"` // "compile", "assert", "unknown"
}

func (t *ErrorHistogramTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if t.stateManager == nil {
		return tools.Result{Success: false, Error: "StateManager não configurado nesta ferramenta"}, nil
	}

	messages := t.stateManager.GetMessages()
	if len(messages) == 0 {
		return tools.Result{Success: true, Data: "[]\n(Nenhuma mensagem de erro registrada no histórico da sessão)"}, nil
	}

	fileCounts := make(map[string]int)
	fileTypes := make(map[string]string)

	// Regexes para extrair nomes de arquivos com erro
	// Ex: "validation error: filename.go", "fail: filename_test.go", "filename.go:123"
	goFileRegex := regexp.MustCompile(`([a-zA-Z0-9_\-\/]+\.go)`)
	pyFileRegex := regexp.MustCompile(`([a-zA-Z0-9_\-\/]+\.py)`)
	jsFileRegex := regexp.MustCompile(`([a-zA-Z0-9_\-\/]+\.ts|[a-zA-Z0-9_\-\/]+\.js)`)

	for _, msg := range messages {
		// Analisa saídas de erros de compilação ou validação (geralmente enviadas por tool ou system)
		if msg.Role == "tool" || msg.Role == "system" {
			content := msg.Content
			if strings.Contains(content, "VALIDATION_ERROR") ||
				strings.Contains(content, "TEST_FAILURE") ||
				strings.Contains(content, "fail") ||
				strings.Contains(content, "error") ||
				strings.Contains(content, "panic") {

				// Detecta tipo
				errType := "assert"
				if strings.Contains(content, "syntax") || strings.Contains(content, "compile") || strings.Contains(content, "undefined") {
					errType = "compile"
				}

				// Encontra arquivos mencionados
				foundFiles := make(map[string]bool)
				for _, match := range goFileRegex.FindAllString(content, -1) {
					foundFiles[match] = true
				}
				for _, match := range pyFileRegex.FindAllString(content, -1) {
					foundFiles[match] = true
				}
				for _, match := range jsFileRegex.FindAllString(content, -1) {
					foundFiles[match] = true
				}

				for file := range foundFiles {
					// Ignora arquivos de teste se quisermos focar nos arquivos modificados
					if strings.Contains(file, "vendor/") || strings.Contains(file, "node_modules/") {
						continue
					}
					fileCounts[file]++
					fileTypes[file] = errType
				}
			}
		}
	}

	var histogram []FileHistogram
	for file, count := range fileCounts {
		histogram = append(histogram, FileHistogram{
			FilePath: file,
			Errors:   count,
			Type:     fileTypes[file],
		})
	}

	data, _ := json.MarshalIndent(histogram, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
