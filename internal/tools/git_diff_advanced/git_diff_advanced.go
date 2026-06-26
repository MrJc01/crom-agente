package git_diff_advanced

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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
		panic("falha ao carregar metadados de git_diff_advanced: " + err.Error())
	}
}

type GitDiffAdvancedTool struct {
	workspaceRoot string
}

func NewGitDiffAdvancedTool(workspaceRoot string) *GitDiffAdvancedTool {
	return &GitDiffAdvancedTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *GitDiffAdvancedTool) ID() string { return metadata.ID }

func (t *GitDiffAdvancedTool) Description() string { return metadata.Description }

func (t *GitDiffAdvancedTool) RequiresApproval() bool { return false }

func (t *GitDiffAdvancedTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"error_log": {
				"type": "string",
				"description": "Output de erro ou traceback do teste que falhou"
			}
		},
		"required": ["error_log"]
	}`)
}

type Correlation struct {
	File        string `json:"file"`
	LineNumber  int    `json:"line_number"`
	HunkContext string `json:"hunk_context"`
	Reason      string `json:"reason"`
}

func (t *GitDiffAdvancedTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		ErrorLog string `json:"error_log"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	// 1. Executa git diff
	cmd := exec.CommandContext(ctx, "git", "diff")
	cmd.Dir = t.workspaceRoot
	outBytes, err := cmd.Output()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao executar git diff: %s", err)}, nil
	}

	diffStr := string(outBytes)
	if len(strings.TrimSpace(diffStr)) == 0 {
		return tools.Result{Success: true, Data: "[]\n(Nenhuma alteração local detectada no git diff)"}, nil
	}

	// 2. Parse do git diff para encontrar linhas modificadas
	// Mapeia arquivo -> conjunto de linhas modificadas
	modifiedLines := make(map[string][]int)
	modifiedContexts := make(map[string]map[int]string) // file -> line -> content

	lines := strings.Split(diffStr, "\n")
	currentFile := ""
	currentLine := 0

	fileHeaderRegex := regexp.MustCompile(`^\+\+\+\ b/(.*)`)
	hunkHeaderRegex := regexp.MustCompile(`^@@\ -\d+(?:,\d+)?\ \+(\d+)(?:,\d+)?\ @@`)

	for _, line := range lines {
		if m := fileHeaderRegex.FindStringSubmatch(line); m != nil {
			currentFile = m[1]
			modifiedLines[currentFile] = []int{}
			modifiedContexts[currentFile] = make(map[int]string)
			continue
		}

		if currentFile == "" {
			continue
		}

		if m := hunkHeaderRegex.FindStringSubmatch(line); m != nil {
			startLine, _ := strconv.Atoi(m[1])
			currentLine = startLine
			continue
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			modifiedLines[currentFile] = append(modifiedLines[currentFile], currentLine)
			modifiedContexts[currentFile][currentLine] = line[1:]
			currentLine++
		} else if strings.HasPrefix(line, " ") {
			currentLine++
		}
	}

	// 3. Correlaciona as modificações com o traceback do log de erro
	var correlations []Correlation
	errorLogLower := strings.ToLower(input.ErrorLog)

	for file, lines := range modifiedLines {
		fileLower := strings.ToLower(file)
		// Verifica se o nome do arquivo aparece no log de erro
		if strings.Contains(errorLogLower, fileLower) {
			for _, lineNum := range lines {
				// Procura no log de erro por padrões como "file.go:line" ou "file:line"
				pattern1 := fmt.Sprintf("%s:%d", fileLower, lineNum)
				pattern2 := fmt.Sprintf("line %d", lineNum)
				
				if strings.Contains(errorLogLower, pattern1) || (strings.Contains(errorLogLower, fileLower) && strings.Contains(errorLogLower, pattern2)) {
					correlations = append(correlations, Correlation{
						File:        file,
						LineNumber:  lineNum,
						HunkContext: modifiedContexts[file][lineNum],
						Reason:      "Linha modificada citada no traceback de erro.",
					})
				}
			}
		}
	}

	// Se não achou correlação exata de linha, mas achou o arquivo modificado citado
	if len(correlations) == 0 {
		for file, lines := range modifiedLines {
			fileLower := strings.ToLower(file)
			if strings.Contains(errorLogLower, fileLower) {
				correlations = append(correlations, Correlation{
					File:        file,
					LineNumber:  -1,
					HunkContext: "Arquivo modificado citado no traceback, sem linha correspondente exata",
					Reason:      "Arquivo alterado presente no traceback de erro (linhas modificadas no diff: " + fmt.Sprint(lines) + ")",
				})
			}
		}
	}

	data, _ := json.MarshalIndent(correlations, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
