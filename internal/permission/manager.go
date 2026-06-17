package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PermissionMode define o modo de permissões
type PermissionMode string

const (
	// ModeTotalAccess indica acesso total imediato
	ModeTotalAccess PermissionMode = "total_access"

	// ModeAskEveryTime indica perguntar sempre que disparado
	ModeAskEveryTime PermissionMode = "ask_every_time"

	// ModeScoped indica fluxo interativo que pode salvar grants whitelisted
	ModeScoped PermissionMode = "scoped"
)

// Grant define uma permissão concedida pelo usuário
type Grant struct {
	Action string `json:"action"` // e.g. "command", "write_file", "read_file"
	Target string `json:"target"` // e.g. "git *", "/workspace/src/*", "*"
}

// PermissionManager gerencia o carregamento de grants e fluxo de autorização
type PermissionManager struct {
	mu             sync.Mutex
	workspacePath  string
	mode           PermissionMode
	grants         []Grant
	grantsFilePath string
	askFunc        func(action, target string) (bool, bool) // (approved, remember)
}

// NewPermissionManager cria um novo gerenciador de permissões
func NewPermissionManager(workspacePath string, mode string, askFunc func(action, target string) (bool, bool)) *PermissionManager {
	pmMode := ModeScoped
	switch mode {
	case "total_access":
		pmMode = ModeTotalAccess
	case "ask_every_time":
		pmMode = ModeAskEveryTime
	}

	return &PermissionManager{
		workspacePath:  workspacePath,
		mode:           pmMode,
		grantsFilePath: filepath.Join(workspacePath, ".crom", "permissions.json"),
		askFunc:        askFunc,
		grants:         []Grant{},
	}
}

func (pm *PermissionManager) loadGrantsLocked() error {
	data, err := os.ReadFile(pm.grantsFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			pm.grants = []Grant{}
			return nil
		}
		return err
	}

	var list []Grant
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	pm.grants = list
	return nil
}

func (pm *PermissionManager) saveGrantsLocked() error {
	dir := filepath.Dir(pm.grantsFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(pm.grants, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(pm.grantsFilePath, data, 0644)
}

// Authorize verifica se a ação solicitada é permitida, consultando o usuário ou arquivo de grants se necessário
func (pm *PermissionManager) Authorize(action, target string) (bool, error) {
	if pm.mode == ModeTotalAccess {
		return true, nil
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Recarrega os grants atuais
	_ = pm.loadGrantsLocked()

	// Verifica se há match com algum grant existente
	for _, g := range pm.grants {
		if g.Action == action && matchTarget(g.Target, target) {
			return true, nil
		}
	}

	if pm.mode == ModeAskEveryTime {
		if pm.askFunc == nil {
			return false, fmt.Errorf("interação com usuário desabilitada (askFunc é nil)")
		}
		approved, _ := pm.askFunc(action, target)
		return approved, nil
	}

	// ModeScoped: pergunta e oferece salvar
	if pm.askFunc == nil {
		return false, fmt.Errorf("interação com usuário desabilitada (askFunc é nil)")
	}

	approved, remember := pm.askFunc(action, target)
	if approved && remember {
		pm.grants = append(pm.grants, Grant{Action: action, Target: target})
		_ = pm.saveGrantsLocked()
	}

	return approved, nil
}

func matchTarget(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(target, prefix)
	}
	return pattern == target
}
