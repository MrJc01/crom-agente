package config

// PermissionMode define os modos de permissão de segurança do orquestrador
type PermissionMode string

const (
	// ModeTotalAccess indica acesso total livre sem HITL
	ModeTotalAccess PermissionMode = "total_access"

	// ModeAskEveryTime indica perguntar sempre que uma ação crítica for disparada
	ModeAskEveryTime PermissionMode = "ask_every_time"

	// ModeScopedPermissions indica que o usuário aprova uma vez e o escopo é memorizado
	ModeScopedPermissions PermissionMode = "scoped_permissions"
)

// Config representa as configurações básicas de inicialização do orquestrador
type Config struct {
	WorkspaceRoot  string         `json:"workspace_root"`
	StoragePath    string         `json:"storage_path"`
	AllowSystem    bool           `json:"allow_system"`
	PermissionMode PermissionMode `json:"permission_mode"`
}

// DefaultConfig retorna uma configuração padrão segura
func DefaultConfig() Config {
	return Config{
		AllowSystem:    false,
		PermissionMode: ModeScopedPermissions,
	}
}
