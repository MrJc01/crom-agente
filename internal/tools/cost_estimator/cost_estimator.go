package cost_estimator

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de cost_estimator: " + err.Error())
	}
}

type CostEstimatorTool struct {
	workspaceRoot string
	stateManager  *state.StateManager
}

func NewCostEstimatorTool(workspaceRoot string, sm *state.StateManager) *CostEstimatorTool {
	return &CostEstimatorTool{
		workspaceRoot: workspaceRoot,
		stateManager:  sm,
	}
}

func (t *CostEstimatorTool) ID() string { return metadata.ID }

func (t *CostEstimatorTool) Description() string { return metadata.Description }

func (t *CostEstimatorTool) RequiresApproval() bool { return false }

func (t *CostEstimatorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"model": {
				"type": "string",
				"description": "Modelo de LLM usado (opcional, padrão: gpt-4o)"
			}
		},
		"required": []
	}`)
}

type CostReport struct {
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

func (t *CostEstimatorTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(args, &input)

	model := strings.ToLower(input.Model)
	if model == "" {
		model = "gpt-4o"
	}

	if t.stateManager == nil {
		return tools.Result{Success: false, Error: "StateManager não configurado nesta ferramenta"}, nil
	}

	// 1. Somar tokens de todas as iterações salvas
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0

	iterationsDir := filepath.Join(filepath.Dir(t.stateManager.FilePath()), "iterations")
	files, err := os.ReadDir(iterationsDir)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				dataPath := filepath.Join(iterationsDir, f.Name())
				dataBytes, readErr := os.ReadFile(dataPath)
				if readErr == nil {
					var log state.IterationLog
					if json.Unmarshal(dataBytes, &log) == nil {
						promptTokens += log.PromptTokens
						completionTokens += log.CompletionTokens
						totalTokens += log.TotalTokens
					}
				}
			}
		}
	}

	// Se não achou logs de iterações, usa o total do estado atual
	if totalTokens == 0 {
		totalTokens = t.stateManager.GetState().TokensGastos
		// Aproximação 80% prompt / 20% completion
		promptTokens = int(float64(totalTokens) * 0.8)
		completionTokens = totalTokens - promptTokens
	}

	// 2. Tabela de Preços (Custo por 1 milhão de tokens)
	var promptPriceUSD, completionPriceUSD float64
	switch {
	case strings.Contains(model, "gpt-4o-mini"):
		promptPriceUSD = 0.150
		completionPriceUSD = 0.600
	case strings.Contains(model, "gpt-4o"):
		promptPriceUSD = 5.00
		completionPriceUSD = 15.00
	case strings.Contains(model, "claude-3-5-sonnet") || strings.Contains(model, "sonnet"):
		promptPriceUSD = 3.00
		completionPriceUSD = 15.00
	case strings.Contains(model, "gemini-1.5-pro") || strings.Contains(model, "pro"):
		promptPriceUSD = 1.25
		completionPriceUSD = 5.00
	case strings.Contains(model, "gemini-1.5-flash") || strings.Contains(model, "flash"):
		promptPriceUSD = 0.075
		completionPriceUSD = 0.30
	default:
		// Default gpt-4o pricing
		promptPriceUSD = 5.00
		completionPriceUSD = 15.00
	}

	costUSD := (float64(promptTokens)/1000000.0)*promptPriceUSD + (float64(completionTokens)/1000000.0)*completionPriceUSD

	res := CostReport{
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		EstimatedCostUSD: costUSD,
	}

	data, _ := json.MarshalIndent(res, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
