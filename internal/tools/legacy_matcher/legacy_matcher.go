package legacy_matcher

import (
	"context"
	_ "embed"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
		panic("falha ao carregar metadados de legacy_matcher: " + err.Error())
	}
}

type LegacyMatcherTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewLegacyMatcherTool(workspaceRoot string, workspaceJail bool) *LegacyMatcherTool {
	return &LegacyMatcherTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *LegacyMatcherTool) ID() string { return metadata.ID }

func (t *LegacyMatcherTool) Description() string { return metadata.Description }

func (t *LegacyMatcherTool) RequiresApproval() bool { return false }

func (t *LegacyMatcherTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Trecho de código ou padrão estilístico para encontrar similaridade"
			},
			"extensions": {
				"type": "array",
				"items": { "type": "string" },
				"description": "Lista de extensões de arquivos para restringir a busca (ex: ['.go', '.py'])"
			}
		},
		"required": ["pattern"]
	}`)
}

type Match struct {
	File       string  `json:"file"`
	LineNumber int     `json:"line_number"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
}

func (t *LegacyMatcherTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Pattern    string   `json:"pattern"`
		Extensions []string `json:"extensions"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	queryTokens := tokenize(input.Pattern)
	if len(queryTokens) == 0 {
		return tools.Result{Success: false, Error: "padrão de busca inválido ou muito curto"}, nil
	}

	exts := make(map[string]bool)
	for _, ext := range input.Extensions {
		exts[strings.ToLower(ext)] = true
	}

	var matches []Match

	// Escaneia arquivos no workspace
	_ = filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == ".crom" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		// Se foi especificado extensões, filtra
		if len(exts) > 0 && !exts[ext] {
			return nil
		}

		// Filtro padrão por tipo de arquivo relevante
		if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" {
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			relPath, _ := filepath.Rel(t.workspaceRoot, path)
			lines := strings.Split(string(contentBytes), "\n")
			for idx, line := range lines {
				lineNum := idx + 1
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				lineTokens := tokenize(trimmed)
				if len(lineTokens) == 0 {
					continue
				}

				// Calcula overlap Jaccard
				intersection := 0
				unionMap := make(map[string]bool)
				queryMap := make(map[string]bool)

				for _, q := range queryTokens {
					queryMap[q] = true
					unionMap[q] = true
				}

				for _, l := range lineTokens {
					if queryMap[l] {
						intersection++
					}
					unionMap[l] = true
				}

				jaccard := float64(intersection) / float64(len(unionMap))
				if jaccard > 0.25 {
					// Adiciona correspondência
					matches = append(matches, Match{
						File:       relPath,
						LineNumber: lineNum,
						Content:    trimmed,
						Similarity: math.Round(jaccard*100) / 100,
					})
				}
			}
		}
		return nil
	})

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	if len(matches) > 10 {
		matches = matches[:10]
	}

	data, _ := json.MarshalIndent(matches, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}

func tokenize(s string) []string {
	reg := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	matches := reg.FindAllString(strings.ToLower(s), -1)
	var tokens []string
	ignore := map[string]bool{
		"func": true, "def": true, "function": true, "var": true, "let": true, "const": true,
		"return": true, "if": true, "else": true, "for": true, "in": true, "import": true,
	}
	for _, m := range matches {
		if len(m) > 1 && !ignore[m] {
			tokens = append(tokens, m)
		}
	}
	return tokens
}
