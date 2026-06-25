package git_conflict

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
		panic("falha ao carregar metadados de git_conflict: " + err.Error())
	}
}

// ConflictBlock representa um bloco de conflito de merge encontrado em um arquivo
type ConflictBlock struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Ours      string `json:"ours"`   // Conteúdo do lado HEAD (<<<<<<< HEAD)
	Theirs    string `json:"theirs"` // Conteúdo do lado da branch incoming (>>>>>>> branch)
	Marker    string `json:"marker"` // Nome da branch incoming
}

// GitConflictTool analisa e extrai conflitos de merge de arquivos
type GitConflictTool struct {
	workspaceRoot string
	jail          bool
}

// NewGitConflictTool cria a ferramenta git_conflict
func NewGitConflictTool(workspaceRoot string, jail bool) *GitConflictTool {
	return &GitConflictTool{workspaceRoot: workspaceRoot, jail: jail}
}

func (t *GitConflictTool) ID() string { return metadata.ID }

func (t *GitConflictTool) Description() string {
	return metadata.Description
}

func (t *GitConflictTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["scan", "analyze"],
				"description": "Ação: 'scan' para encontrar arquivos conflituosos, 'analyze' para extrair blocos de um arquivo"
			},
			"path": {
				"type": "string",
				"description": "Caminho do arquivo a analisar (obrigatório para 'analyze')"
			}
		},
		"required": ["action"]
	}`)
}

func (t *GitConflictTool) RequiresApproval() bool { return false }

func (t *GitConflictTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	switch input.Action {
	case "scan":
		return t.scanConflicts(ctx)
	case "analyze":
		if input.Path == "" {
			return tools.Result{Success: false, Error: "caminho do arquivo é obrigatório para 'analyze'"}, nil
		}
		return t.analyzeConflicts(ctx, input.Path)
	default:
		return tools.Result{Success: false, Error: fmt.Sprintf("ação desconhecida: %q. Use 'scan' ou 'analyze'", input.Action)}, nil
	}
}

func (t *GitConflictTool) scanConflicts(ctx context.Context) (tools.Result, error) {
	var conflictFiles []string

	err := filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignora erros de acesso
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Pular diretórios irrelevantes
		name := info.Name()
		if info.IsDir() {
			skip := []string{".git", "node_modules", "vendor", ".crom", "__pycache__", "build", "dist"}
			for _, s := range skip {
				if name == s {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Pular binários e arquivos grandes
		if info.Size() > 5*1024*1024 { // 5MB
			return nil
		}

		if hasConflictMarkers(path) {
			rel, _ := filepath.Rel(t.workspaceRoot, path)
			conflictFiles = append(conflictFiles, rel)
		}
		return nil
	})

	if err != nil && err != context.Canceled {
		return tools.Result{Success: false, Error: "erro ao escanear workspace: " + err.Error()}, nil
	}

	if len(conflictFiles) == 0 {
		return tools.Result{Success: true, Data: "Nenhum arquivo com conflitos de merge encontrado."}, nil
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"total":          len(conflictFiles),
		"conflict_files": conflictFiles,
	}, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}

func (t *GitConflictTool) analyzeConflicts(ctx context.Context, path string) (tools.Result, error) {
	absPath, err := tools.ValidatePath(t.workspaceRoot, path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	blocks, err := parseConflictBlocks(absPath)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	if len(blocks) == 0 {
		return tools.Result{Success: true, Data: fmt.Sprintf("Arquivo '%s' não contém conflitos de merge.", path)}, nil
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"file":      path,
		"total":     len(blocks),
		"conflicts": blocks,
	}, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}

// hasConflictMarkers verifica rapidamente se um arquivo contém marcadores de conflito
func hasConflictMarkers(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "<<<<<<<") {
			return true
		}
	}
	return false
}

// parseConflictBlocks extrai todos os blocos de conflito de um arquivo
func parseConflictBlocks(path string) ([]ConflictBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo: %w", err)
	}
	defer f.Close()

	var blocks []ConflictBlock
	scanner := bufio.NewScanner(f)

	lineNum := 0
	var currentBlock *ConflictBlock
	var oursLines []string
	var theirsLines []string
	inOurs := false
	inTheirs := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "<<<<<<< ") {
			currentBlock = &ConflictBlock{
				File:      filepath.Base(path),
				StartLine: lineNum,
				Marker:    "",
			}
			oursLines = nil
			theirsLines = nil
			inOurs = true
			inTheirs = false
			continue
		}

		if line == "=======" && currentBlock != nil {
			inOurs = false
			inTheirs = true
			continue
		}

		if strings.HasPrefix(line, ">>>>>>> ") && currentBlock != nil {
			currentBlock.EndLine = lineNum
			currentBlock.Marker = strings.TrimPrefix(line, ">>>>>>> ")
			currentBlock.Ours = strings.Join(oursLines, "\n")
			currentBlock.Theirs = strings.Join(theirsLines, "\n")
			blocks = append(blocks, *currentBlock)
			currentBlock = nil
			inOurs = false
			inTheirs = false
			continue
		}

		if inOurs {
			oursLines = append(oursLines, line)
		} else if inTheirs {
			theirsLines = append(theirsLines, line)
		}
	}

	return blocks, scanner.Err()
}
