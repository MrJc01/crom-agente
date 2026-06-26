package semantic_search

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
	"unicode"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de semantic_search: " + err.Error())
	}
}

// BM25 parameters (standard Okapi BM25 defaults)
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// codeExtensions lists file extensions to index
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".jsx": true, ".tsx": true, ".java": true, ".rs": true,
	".rb": true, ".c": true, ".cpp": true, ".h": true,
	".css": true, ".html": true, ".vue": true, ".svelte": true,
}

// docInfo holds pre-computed per-document data for BM25
type docInfo struct {
	path      string
	wordCount int                // total word count (for length normalization)
	termFreqs map[string]int     // term -> raw frequency count
}

// SemanticSearchTool realiza busca local usando BM25 com IDF real
type SemanticSearchTool struct {
	workspaceRoot string
}

// NewSemanticSearchTool cria a ferramenta
func NewSemanticSearchTool(workspaceRoot string) *SemanticSearchTool {
	return &SemanticSearchTool{workspaceRoot: workspaceRoot}
}

func (t *SemanticSearchTool) ID() string { return metadata.ID }

func (t *SemanticSearchTool) Description() string { return metadata.Description }

func (t *SemanticSearchTool) RequiresApproval() bool { return false }

func (t *SemanticSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Texto ou log de erro para buscar correspondência no código"
			}
		},
		"required": ["query"]
	}`)
}

// SearchResult represents a single search match with its BM25 score
type SearchResult struct {
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

func (t *SemanticSearchTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	terms := tokenize(input.Query)
	if len(terms) == 0 {
		return tools.Result{Success: false, Error: "consulta de busca vazia ou inválida"}, nil
	}

	// Phase 1: Collect all documents and pre-compute term frequencies
	docs, err := t.indexDocuments(ctx)
	if err != nil || len(docs) == 0 {
		return tools.Result{Success: true, Data: "[]"}, nil
	}

	// Phase 2: Compute corpus-level statistics
	N := float64(len(docs))
	avgDL := computeAvgDocLength(docs)

	// Phase 3: Compute IDF for each query term
	idf := make(map[string]float64, len(terms))
	for _, term := range terms {
		df := 0 // document frequency: how many docs contain this term
		for _, doc := range docs {
			if doc.termFreqs[term] > 0 {
				df++
			}
		}
		// Standard BM25 IDF: log((N - df + 0.5) / (df + 0.5) + 1)
		idf[term] = math.Log((N-float64(df)+0.5)/(float64(df)+0.5) + 1.0)
	}

	// Phase 4: Score each document using BM25
	var results []SearchResult
	for _, doc := range docs {
		score := 0.0
		dl := float64(doc.wordCount)

		for _, term := range terms {
			tf := float64(doc.termFreqs[term])
			if tf == 0 {
				continue
			}
			// BM25 formula: IDF * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * dl/avgdl))
			numerator := tf * (bm25K1 + 1.0)
			denominator := tf + bm25K1*(1.0-bm25B+bm25B*dl/avgDL)
			score += idf[term] * numerator / denominator
		}

		if score > 0 {
			relPath, _ := filepath.Rel(t.workspaceRoot, doc.path)
			results = append(results, SearchResult{
				Path:  relPath,
				Score: math.Round(score*1000) / 1000, // 3 decimal places
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 10 {
		results = results[:10]
	}

	dataJSON, _ := json.MarshalIndent(results, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(dataJSON),
	}, nil
}

// indexDocuments walks the workspace and pre-computes term frequencies for each file
func (t *SemanticSearchTool) indexDocuments(ctx context.Context) ([]docInfo, error) {
	var docs []docInfo

	err := filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Check context cancellation periodically
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == ".crom" || name == "node_modules" || name == "vendor" ||
				name == "__pycache__" || name == ".venv" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip large files (>512KB) to avoid memory pressure
		if info.Size() > 512*1024 {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] {
			return nil
		}

		contentBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		content := strings.ToLower(string(contentBytes))
		words := tokenizeContent(content)

		// Build term frequency map
		freqs := make(map[string]int, len(words)/2)
		for _, w := range words {
			freqs[w]++
		}

		docs = append(docs, docInfo{
			path:      path,
			wordCount: len(words),
			termFreqs: freqs,
		})

		return nil
	})

	return docs, err
}

// computeAvgDocLength returns the average document length across the corpus
func computeAvgDocLength(docs []docInfo) float64 {
	if len(docs) == 0 {
		return 1.0
	}
	total := 0
	for _, d := range docs {
		total += d.wordCount
	}
	avg := float64(total) / float64(len(docs))
	if avg < 1.0 {
		return 1.0
	}
	return avg
}

// tokenize splits a query into searchable terms with camelCase awareness
func tokenize(query string) []string {
	// First split by camelCase boundaries, then by non-alphanumeric
	expanded := splitCamelCase(query)
	reg := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	matches := reg.FindAllString(strings.ToLower(expanded), -1)

	seen := make(map[string]bool, len(matches))
	var terms []string
	for _, m := range matches {
		if len(m) > 2 && !stopWords[m] && !seen[m] {
			seen[m] = true
			terms = append(terms, m)
		}
	}
	return terms
}

// tokenizeContent splits file content into words for frequency counting
func tokenizeContent(content string) []string {
	reg := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	matches := reg.FindAllString(content, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			result = append(result, m)
		}
	}
	return result
}

// splitCamelCase inserts spaces at camelCase boundaries
// e.g. "readFileContent" -> "read File Content"
func splitCamelCase(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (i+1 < len(runes) && unicode.IsLower(runes[i+1]) || unicode.IsLower(runes[i-1])) {
			sb.WriteRune(' ')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// stopWords contains common words to filter from queries
var stopWords = map[string]bool{
	"and": true, "the": true, "for": true, "not": true,
	"this": true, "that": true, "with": true, "from": true,
	"are": true, "was": true, "were": true, "been": true,
	"have": true, "has": true, "had": true, "does": true,
	"did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "can": true,
	"var": true, "let": true, "const": true, "nil": true,
	"true": true, "false": true, "null": true, "undefined": true,
}
