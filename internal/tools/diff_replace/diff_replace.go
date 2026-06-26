package diff_replace

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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
		panic("falha ao carregar metadados de diff_replace: " + err.Error())
	}
}

// DiffReplaceTool substitui um trecho específico de texto por outro em um arquivo
type DiffReplaceTool struct {
	workspaceRoot string
	jail          bool
}

// NewDiffReplaceTool cria a ferramenta diff_replace
func NewDiffReplaceTool(workspaceRoot string, jail bool) *DiffReplaceTool {
	return &DiffReplaceTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *DiffReplaceTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *DiffReplaceTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *DiffReplaceTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo a ser modificado"
			},
			"start_line": {
				"type": "integer",
				"description": "Linha inicial estimada para busca (1-indexed, opcional)"
			},
			"end_line": {
				"type": "integer",
				"description": "Linha final estimada para busca (1-indexed, opcional)"
			},
			"target_content": {
				"type": "string",
				"description": "Texto exato a ser substituído. Deve corresponder precisamente ao conteúdo do arquivo."
			},
			"replacement_content": {
				"type": "string",
				"description": "Novo texto que substituirá o target_content."
			}
		},
		"required": ["path", "target_content", "replacement_content"]
	}`)
}

// RequiresApproval indica que esta ferramenta requer aprovação HITL (nível Alta)
func (t *DiffReplaceTool) RequiresApproval() bool {
	return true
}

// Execute executa a substituição do conteúdo
func (t *DiffReplaceTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path               string `json:"path"`
		StartLine          int    `json:"start_line"`
		EndLine            int    `json:"end_line"`
		TargetContent      string `json:"target_content"`
		ReplacementContent string `json:"replacement_content"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo: %s", err.Error())}, nil
	}

	content := string(data)

	// Procurar ocorrências do TargetContent (Tenta exato primeiro)
	var matches [][]int
	searchIndex := 0
	for {
		idx := strings.Index(content[searchIndex:], input.TargetContent)
		if idx == -1 {
			break
		}
		actualIdx := searchIndex + idx
		matches = append(matches, []int{actualIdx, actualIdx + len(input.TargetContent)})
		searchIndex = actualIdx + len(input.TargetContent)
		if len(input.TargetContent) == 0 { // Prevenir loop infinito se target estiver vazio
			break
		}
	}

	// Se não achou exato, tenta fuzzy match (Item 32)
	if len(matches) == 0 && len(strings.TrimSpace(input.TargetContent)) > 0 {
		re, errRegex := makeFuzzyRegex(input.TargetContent)
		if errRegex == nil {
			matches = re.FindAllStringIndex(content, -1)
		}
	}

	if len(matches) == 0 {
		return tools.Result{Success: false, Error: "target_content não foi encontrado no arquivo"}, nil
	}

	// Filtrar correspondências baseando-se nas linhas especificadas (se informadas)
	var validMatches [][]int
	for _, matchRange := range matches {
		matchStart := matchRange[0]
		// Determinar em qual linha (1-indexed) a correspondência começa
		lineNum := 1
		for i := 0; i < matchStart; i++ {
			if content[i] == '\n' {
				lineNum++
			}
		}

		// Filtrar se start_line e end_line forem informados e válidos
		if input.StartLine > 0 && lineNum < input.StartLine {
			continue
		}
		if input.EndLine > 0 && lineNum > input.EndLine {
			continue
		}

		validMatches = append(validMatches, matchRange)
	}

	if len(validMatches) == 0 {
		return tools.Result{Success: false, Error: "target_content encontrado, mas fora do intervalo de linhas especificado"}, nil
	}

	if len(validMatches) > 1 {
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("substituição ambígua: encontradas %d ocorrências de target_content no intervalo de busca especificado (Item 33)", len(validMatches)),
		}, nil
	}

	// Exatamente uma correspondência válida encontrada
	matchRange := validMatches[0]
	startIdx := matchRange[0]
	endIdx := matchRange[1]
	newContent := content[:startIdx] + input.ReplacementContent + content[endIdx:]

	// Gravar de volta no arquivo
	err = os.WriteFile(targetFile, []byte(newContent), 0644)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao gravar alterações no arquivo: %s", err.Error())}, nil
	}

	return tools.Result{Success: true, Data: fmt.Sprintf("substituição efetuada com sucesso no arquivo %s", input.Path)}, nil
}

func makeFuzzyRegex(target string) (*regexp.Regexp, error) {
	words := strings.Fields(target)
	if len(words) == 0 {
		return nil, fmt.Errorf("target content is empty")
	}
	var patternParts []string
	for _, w := range words {
		patternParts = append(patternParts, regexp.QuoteMeta(w))
	}
	pattern := "(?s)" + strings.Join(patternParts, `\s+`)
	return regexp.Compile(pattern)
}
