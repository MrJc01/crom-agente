package read_log_paginated

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
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
		panic("falha ao carregar metadados de read_log_paginated: " + err.Error())
	}
}

type ReadLogPaginatedTool struct {
	workspaceRoot string
	workspaceJail bool
}

func NewReadLogPaginatedTool(workspaceRoot string, workspaceJail bool) *ReadLogPaginatedTool {
	return &ReadLogPaginatedTool{
		workspaceRoot: workspaceRoot,
		workspaceJail: workspaceJail,
	}
}

func (t *ReadLogPaginatedTool) ID() string { return metadata.ID }

func (t *ReadLogPaginatedTool) Description() string { return metadata.Description }

func (t *ReadLogPaginatedTool) RequiresApproval() bool { return false }

func (t *ReadLogPaginatedTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto para o arquivo de log"
			},
			"page": {
				"type": "integer",
				"description": "Número da página a ser lida (1-based, padrão: 1)"
			},
			"page_size_kb": {
				"type": "integer",
				"description": "Tamanho da página em Kilobytes (padrão: 100, máximo: 500)"
			}
		},
		"required": ["path"]
	}`)
}

type PaginatedResult struct {
	FilePath    string `json:"file_path"`
	CurrentPage int    `json:"current_page"`
	TotalPages  int    `json:"total_pages"`
	PageSizeKB  int    `json:"page_size_kb"`
	Content     string `json:"content"`
	HasMore     bool   `json:"has_more"`
}

func (t *ReadLogPaginatedTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path       string `json:"path"`
		Page       int    `json:"page"`
		PageSizeKB int    `json:"page_size_kb"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSizeKB <= 0 {
		input.PageSizeKB = 100
	}
	if input.PageSizeKB > 500 {
		input.PageSizeKB = 500
	}

	targetPath := input.Path
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(t.workspaceRoot, targetPath)
	}

	if t.workspaceJail {
		rel, err := filepath.Rel(t.workspaceRoot, targetPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{Success: false, Error: "acesso negado: fora do workspace"}, nil
		}
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao abrir arquivo: %s", err)}, nil
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao ler metadados do arquivo: %s", err)}, nil
	}

	fileSize := stat.Size()
	pageSizeBytes := int64(input.PageSizeKB * 1024)
	totalPages := int(fileSize / pageSizeBytes)
	if fileSize%pageSizeBytes > 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}

	if input.Page > totalPages {
		return tools.Result{Success: false, Error: fmt.Sprintf("página %d excede o total de páginas (%d)", input.Page, totalPages)}, nil
	}

	offset := int64(input.Page-1) * pageSizeBytes
	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao mover cursor de leitura: %s", err)}, nil
	}

	buffer := make([]byte, pageSizeBytes)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao ler bloco do arquivo: %s", err)}, nil
	}

	relPath, _ := filepath.Rel(t.workspaceRoot, targetPath)
	res := PaginatedResult{
		FilePath:    relPath,
		CurrentPage: input.Page,
		TotalPages:  totalPages,
		PageSizeKB:  input.PageSizeKB,
		Content:     string(buffer[:n]),
		HasMore:     input.Page < totalPages,
	}

	data, _ := json.MarshalIndent(res, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
