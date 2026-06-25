package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ValidateCreatedFile verifica se o arquivo criado/editado no caminho especificado possui erros de sintaxe ou compilação.
// Retorna (true, "") se o arquivo for válido, ou (false, "detalhes do erro") caso contrário.
func ValidateCreatedFile(path string, language string) (bool, string) {
	// Se a linguagem estiver vazia, detectamos pela extensão do arquivo
	if language == "" {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			language = "go"
		case ".py":
			language = "python"
		case ".json":
			language = "json"
		case ".html", ".htm":
			language = "html"
		default:
			// Para outras extensões ou se não detectado, retornamos válido por padrão
			return true, ""
		}
	}

	// Ler o conteúdo do arquivo
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("Failed to read file: %v", err)
	}

	switch strings.ToLower(language) {
	case "go":
		// 1. Validação de Sintaxe usando o parser nativo do Go
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return false, fmt.Sprintf("Go syntax error:\n%v", err)
		}

		// 2. Validação usando 'go vet' se o comando estiver disponível
		if _, err := exec.LookPath("go"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", "vet", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				// go vet pode retornar erro sobre pacotes se o arquivo estiver fora do escopo ou faltar imports
				// Retornamos o erro de vet caso seja um erro real de compilação/sintaxe
				outStr := string(output)
				// Se for apenas aviso de que o pacote não pôde ser carregado e não for um erro de código em si:
				if strings.Contains(outStr, "named files must be in single package") ||
					strings.Contains(outStr, "package ") && strings.Contains(outStr, "is not in GOROOT") {
					// Ignora esses avisos de infraestrutura e aceita como válido o parse sintático feito anteriormente
					return true, ""
				}
				return false, fmt.Sprintf("Go vet error:\n%s", outStr)
			}
		}

	case "python":
		// Validação compilando para bytecode usando 'python3' ou 'python'
		var pythonCmd string
		if _, err := exec.LookPath("python3"); err == nil {
			pythonCmd = "python3"
		} else if _, err := exec.LookPath("python"); err == nil {
			pythonCmd = "python"
		}

		if pythonCmd != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, pythonCmd, "-m", "py_compile", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("Python compilation error:\n%s", string(output))
			}
		}

	case "json":
		var temp interface{}
		if err := json.Unmarshal(contentBytes, &temp); err != nil {
			return false, fmt.Sprintf("JSON parse error:\n%v", err)
		}

	case "html":
		// Validação básica usando o tokenizer de HTML
		z := html.NewTokenizer(strings.NewReader(string(contentBytes)))
		for {
			tt := z.Next()
			if tt == html.ErrorToken {
				err := z.Err()
				if err == io.EOF {
					break
				}
				return false, fmt.Sprintf("HTML tokenization error:\n%v", err)
			}
		}
	}

	return true, ""
}
