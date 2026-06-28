package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePath valida se um caminho é seguro e reside dentro do workspace se jail for true.
// Retorna o caminho absoluto resolvido.
func ValidatePath(workspaceRoot, targetPath string, jail bool) (string, error) {
	// Normaliza caminhos do Windows para barras normais para compatibilidade cruzada
	targetPath = strings.ReplaceAll(targetPath, "\\", "/")

	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("caminho de workspace inválido: %w", err)
	}

	var absTarget string
	if filepath.IsAbs(targetPath) {
		absTarget = filepath.Clean(targetPath)
	} else {
		absTarget = filepath.Clean(filepath.Join(absWorkspace, targetPath))
	}

	if jail {
		rel, err := filepath.Rel(absWorkspace, absTarget)
		if err != nil {
			return "", fmt.Errorf("acesso negado: path traversal detectado: %w", err)
		}
		if strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("acesso negado: o arquivo '%s' está fora do sandbox do workspace (%s)", targetPath, absWorkspace)
		}
	}

	return absTarget, nil
}

// EnsureDir tenta criar o diretório e, em caso de falha típica como 'not a directory',
// retorna uma mensagem de erro didática orientando o LLM para auto-correção.
func EnsureDir(path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		if strings.Contains(err.Error(), "not a directory") {
			return fmt.Errorf("FALHA DE SISTEMA DE ARQUIVOS (Auto-Correção): Não foi possível criar a pasta '%s' porque um dos diretórios pais já existe como um ARQUIVO DE TEXTO comum. Verifique se você gravou um arquivo no lugar da pasta e remova/renomeie esse arquivo antes de tentar novamente", path)
		}
		return fmt.Errorf("erro ao criar diretórios pai: %w", err)
	}
	return nil
}
