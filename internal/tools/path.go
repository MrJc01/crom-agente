package tools

import (
	"fmt"
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
