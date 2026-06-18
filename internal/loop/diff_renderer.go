package loop

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crom/crom-agente/internal/security"
)

// renderDiffZone renderiza e envia um diff visual das alterações propostas de escrita/edição para o handler
func (al *AgenticLoop) renderDiffZone(rawArgs string, toolID string, workspaceDir string) {
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
		al.handler.OnMessage("system", diffOutput)

	case "diff_replace":
		var args struct {
			Path               string `json:"path"`
			TargetContent      string `json:"target_content"`
			ReplacementContent string `json:"replacement_content"`
		}
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return
		}

		// Para diff_replace, mostramos o diff do trecho alterado
		diffOutput := security.RenderDiff(args.Path, args.TargetContent, args.ReplacementContent)
		al.handler.OnMessage("system", diffOutput)
	}
}
