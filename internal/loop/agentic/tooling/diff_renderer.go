package tooling

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crom/crom-agente/internal/security"
)

// MessageHandler define a interface mínima necessária para reportar logs
type MessageHandler interface {
	OnMessage(role, msg string)
}

// RenderDiffZone renderiza e envia um diff visual das alterações propostas de escrita/edição para o handler
func RenderDiffZone(handler MessageHandler, workspaceDir string, rawArgs string, toolID string) {
	if handler == nil {
		return
	}
	switch toolID {
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return
		}

		// Resolver caminho do arquivo
		filePath := args.Path
		if !filepath.IsAbs(filePath) && workspaceDir != "" {
			filePath = filepath.Join(workspaceDir, filePath)
		}

		// Ler conteúdo atual do arquivo (pode não existir se for arquivo novo)
		oldContent := ""
		if data, err := os.ReadFile(filePath); err == nil {
			oldContent = string(data)
		}

		diffOutput := security.RenderDiff(args.Path, oldContent, args.Content)
		handler.OnMessage("system", diffOutput)

	case "edit_file":
		var args struct {
			Path               string `json:"path"`
			TargetContent      string `json:"target_content"`
			ReplacementContent string `json:"replacement_content"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return
		}

		// Para edit_file, mostramos o diff do trecho alterado
		diffOutput := security.RenderDiff(args.Path, args.TargetContent, args.ReplacementContent)
		handler.OnMessage("system", diffOutput)
	}
}
