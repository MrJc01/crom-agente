package concurrency_lock

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de concurrency_lock: " + err.Error())
	}
}

// locks maps file absolute path -> owner ID (e.g. session name or agent ID)
var (
	locksMu     sync.Mutex
	locks       = make(map[string]lockEntry)
	lockTTL     = 60 * time.Second
	cleanupOnce sync.Once
)

type lockEntry struct {
	Owner     string    `json:"owner"`
	Timestamp time.Time `json:"locked_at"`
}

func init() {
	// Iniciar goroutine de limpeza de locks expirados
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			for range ticker.C {
				locksMu.Lock()
				now := time.Now()
				for path, entry := range locks {
					if now.Sub(entry.Timestamp) > lockTTL {
						delete(locks, path)
					}
				}
				locksMu.Unlock()
			}
		}()
	})
}

type ConcurrencyLockTool struct {
	workspaceRoot string
}

func NewConcurrencyLockTool(workspaceRoot string) *ConcurrencyLockTool {
	return &ConcurrencyLockTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *ConcurrencyLockTool) ID() string { return metadata.ID }

func (t *ConcurrencyLockTool) Description() string { return metadata.Description }

func (t *ConcurrencyLockTool) RequiresApproval() bool { return false }

func (t *ConcurrencyLockTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho relativo ou absoluto do arquivo"
			},
			"action": {
				"type": "string",
				"enum": ["acquire", "release", "status"],
				"description": "Ação a realizar: 'acquire' (adquire trava), 'release' (libera trava), 'status' (verifica status)"
			},
			"owner": {
				"type": "string",
				"description": "Identificador do proprietário da trava (ex: 'agent-1')"
			}
		},
		"required": ["path", "action"]
	}`)
}

func (t *ConcurrencyLockTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Owner  string `json:"owner"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	absPath := input.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.workspaceRoot, absPath)
	}
	absPath = filepath.Clean(absPath)

	owner := input.Owner
	if owner == "" {
		// Tenta obter do context
		if sessName, ok := ctx.Value("session_name").(string); ok && sessName != "" {
			owner = sessName
		} else {
			owner = "default-orchestrator"
		}
	}

	locksMu.Lock()
	defer locksMu.Unlock()

	switch input.Action {
	case "acquire":
		if entry, exists := locks[absPath]; exists {
			if entry.Owner == owner {
				return tools.Result{
					Success: true,
					Data:    fmt.Sprintf("✓ Trava já pertence ao proprietário '%s'.", owner),
				}, nil
			}
			return tools.Result{
				Success: false,
				Error:   fmt.Sprintf("falha ao adquirir trava: arquivo está travado por '%s' desde %s", entry.Owner, entry.Timestamp.Format("15:04:05")),
			}, nil
		}
		locks[absPath] = lockEntry{
			Owner:     owner,
			Timestamp: time.Now(),
		}
		return tools.Result{
			Success: true,
			Data:    fmt.Sprintf("✓ Trava adquirida com sucesso para o arquivo por '%s'.", owner),
		}, nil

	case "release":
		if entry, exists := locks[absPath]; exists {
			if entry.Owner != owner && owner != "default-orchestrator" {
				return tools.Result{
					Success: false,
					Error:   fmt.Sprintf("permissão negada: trava pertence a '%s' e você é '%s'", entry.Owner, owner),
				}, nil
			}
			delete(locks, absPath)
			return tools.Result{
				Success: true,
				Data:    "✓ Trava liberada com sucesso.",
			}, nil
		}
		return tools.Result{
			Success: true,
			Data:    "Info: O arquivo já não possuía trava ativa.",
		}, nil

	case "status":
		entry, exists := locks[absPath]
		res := map[string]interface{}{
			"path":      absPath,
			"locked":    exists,
			"owner":     entry.Owner,
			"locked_at": entry.Timestamp,
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		return tools.Result{
			Success: true,
			Data:    string(data),
		}, nil

	default:
		return tools.Result{Success: false, Error: "ação desconhecida"}, nil
	}
}
