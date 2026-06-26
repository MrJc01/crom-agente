package mttr_report

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
		panic("falha ao carregar metadados de mttr_report: " + err.Error())
	}
}

type MTTRReportTool struct {
	workspaceRoot string
	stateManager  *state.StateManager
}

func NewMTTRReportTool(workspaceRoot string, sm *state.StateManager) *MTTRReportTool {
	return &MTTRReportTool{
		workspaceRoot: workspaceRoot,
		stateManager:  sm,
	}
}

func (t *MTTRReportTool) ID() string { return metadata.ID }

func (t *MTTRReportTool) Description() string { return metadata.Description }

func (t *MTTRReportTool) RequiresApproval() bool { return false }

func (t *MTTRReportTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

type Report struct {
	BugsDetected      int      `json:"bugs_detected"`
	FirstErrorAt      string   `json:"first_error_at,omitempty"`
	ResolvedAt        string   `json:"resolved_at,omitempty"`
	MTTRSeconds       float64  `json:"mttr_seconds"`
	MTTRFormatted     string   `json:"mttr_formatted"`
	CorrectionCycles  int      `json:"correction_cycles"`
	Status            string   `json:"status"` // "resolved", "in_progress", "no_errors"
}

func (t *MTTRReportTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if t.stateManager == nil {
		return tools.Result{Success: false, Error: "StateManager não configurado nesta ferramenta"}, nil
	}

	// 1. Carregar logs de iterações
	iterationsDir := filepath.Join(filepath.Dir(t.stateManager.FilePath()), "iterations")
	files, err := os.ReadDir(iterationsDir)
	if err != nil {
		return tools.Result{Success: true, Data: "{}\n(Nenhuma iteração registrada na pasta iterations)"}, nil
	}

	var logs []state.IterationLog
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			dataPath := filepath.Join(iterationsDir, f.Name())
			dataBytes, readErr := os.ReadFile(dataPath)
			if readErr == nil {
				var log state.IterationLog
				if json.Unmarshal(dataBytes, &log) == nil {
					logs = append(logs, log)
				}
			}
		}
	}

	// Ordena por iteração
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Iteration < logs[j].Iteration
	})

	var firstErrorTime time.Time
	var successTime time.Time
	bugCount := 0
	cycles := 0
	hasActiveError := false

	for _, log := range logs {
		hasFailure := false
		for _, toolTrace := range log.ToolsCalled {
			if !toolTrace.Success {
				hasFailure = true
				break
			}
		}

		// Checa se as mensagens contem indicadores de falha
		for _, msg := range log.Messages {
			if msg.Role == "tool" && (strings.Contains(msg.Content, "VALIDATION_ERROR") || strings.Contains(msg.Content, "ROLLBACK_TRIGGERED") || strings.Contains(msg.Content, "TEST_FAILURE")) {
				hasFailure = true
				break
			}
		}

		if hasFailure {
			cycles++
			if !hasActiveError {
				bugCount++
				hasActiveError = true
				if firstErrorTime.IsZero() {
					firstErrorTime = log.Timestamp
				}
			}
		} else {
			// Se o turno foi limpo (sucesso) e tínhamos erro pendente
			if hasActiveError {
				successTime = log.Timestamp
				hasActiveError = false
			}
		}
	}

	// Se terminou e ainda tem erro ativo, mas existiram sucessos parciais anteriores
	if hasActiveError && successTime.IsZero() {
		// não resolvido ainda
	}

	report := Report{
		BugsDetected:     bugCount,
		CorrectionCycles: cycles,
		Status:           "no_errors",
	}

	if !firstErrorTime.IsZero() {
		report.FirstErrorAt = firstErrorTime.Format(time.RFC3339)
		if !successTime.IsZero() {
			report.ResolvedAt = successTime.Format(time.RFC3339)
			report.Status = "resolved"
			duration := successTime.Sub(firstErrorTime)
			report.MTTRSeconds = duration.Seconds()
			report.MTTRFormatted = duration.Round(time.Second).String()
		} else {
			report.Status = "in_progress"
			duration := time.Since(firstErrorTime)
			report.MTTRSeconds = duration.Seconds()
			report.MTTRFormatted = duration.Round(time.Second).String() + " (decorrido)"
		}
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
