package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepTool busca padrões de texto em arquivos
type GrepTool struct {
	workspaceRoot string
	jail          bool
}

// NewGrepTool cria a ferramenta grep
func NewGrepTool(workspaceRoot string, jail bool) *GrepTool {
	return &GrepTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

// ID retorna o identificador da ferramenta
func (t *GrepTool) ID() string {
	return "grep"
}

// Description retorna a descrição da ferramenta
func (t *GrepTool) Description() string {
	return "Busca por ocorrências de uma string ou expressão regular em arquivos de texto no workspace."
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *GrepTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Texto ou expressão regular a ser pesquisada"
			},
			"path": {
				"type": "string",
				"description": "Subdiretório para restringir a busca (opcional, padrão raiz do workspace)"
			},
			"is_regex": {
				"type": "boolean",
				"description": "Se verdadeiro, interpreta a query como uma expressão regular (opcional, padrão falso)"
			}
		},
		"required": ["query"]
	}`)
}

// RequiresApproval indica que esta ferramenta pode rodar sem confirmação
func (t *GrepTool) RequiresApproval() bool {
	return false
}

// MatchResult representa uma linha correspondente na pesquisa
type MatchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// Execute executa a busca textual nos arquivos
func (t *GrepTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Query   string `json:"query"`
		Path    string `json:"path"`
		IsRegex bool   `json:"is_regex"`
	}

	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	if input.Query == "" {
		return Result{Success: false, Error: "a query de busca não pode estar vazia"}, nil
	}

	startDir, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	var compiledRegex *regexp.Regexp
	if input.IsRegex {
		compiledRegex, err = regexp.Compile(input.Query)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("expressão regular inválida: %s", err.Error())}, nil
		}
	}

	var matches []MatchResult
	const maxMatches = 50

	err = filepath.WalkDir(startDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // Ignorar erros individuais de acesso
		}

		if len(matches) >= maxMatches {
			return filepath.SkipDir // Para de andar ao atingir o limite
		}

		base := d.Name()
		// Ignorar pastas de controle
		if d.IsDir() {
			if base == ".git" || base == "node_modules" || base == "build" || base == "dist" || base == "bin" || base == ".crom" || base == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}

		// Filtrar arquivos conhecidos por serem binários ou muito grandes por extensão
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".pdf" || ext == ".zip" || ext == ".tar" || ext == ".gz" || ext == ".exe" || ext == ".dll" || ext == ".so" || ext == ".dylib" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Pular arquivos maiores que 2MB
		if info.Size() > 2*1024*1024 {
			return nil
		}

		// Buscar correspondências dentro do arquivo
		fileMatches, searchErr := t.searchInFile(path, input.Query, compiledRegex, maxMatches-len(matches))
		if searchErr == nil && len(fileMatches) > 0 {
			matches = append(matches, fileMatches...)
		}

		return nil
	})

	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro durante a varredura do diretório: %s", err.Error())}, nil
	}

	jsonData, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	return Result{Success: true, Data: string(jsonData)}, nil
}

// searchInFile lê o arquivo, verifica se é binário, e busca pelo termo
func (t *GrepTool) searchInFile(filePath, query string, re *regexp.Regexp, limit int) ([]MatchResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Checar se o arquivo é binário lendo os primeiros 512 bytes
	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if bytes.Contains(head[:n], []byte{0}) {
		// Contém byte nulo, provavelmente binário
		return nil, nil
	}

	// Voltar ao início do arquivo
	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	// Ler todo o conteúdo
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	var results []MatchResult

	relPath, err := filepath.Rel(t.workspaceRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	for idx, line := range lines {
		if len(results) >= limit {
			break
		}

		matched := false
		if re != nil {
			matched = re.MatchString(line)
		} else {
			matched = strings.Contains(strings.ToLower(line), strings.ToLower(query))
		}

		if matched {
			results = append(results, MatchResult{
				File:    relPath,
				Line:    idx + 1,
				Content: strings.TrimSpace(line),
			})
		}
	}

	return results, nil
}
