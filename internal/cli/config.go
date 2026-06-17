package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/crom/crom-agente/internal/config"
	"github.com/spf13/cobra"
)

var revealEnv bool

// configCmd representa a base para comandos de configuração
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Gerencia as configurações globais, locais e de ambiente",
	Long:  `Permite visualizar e alterar configurações no global.json, .env e no config.json do workspace.`,
}

// --- Comandos de Configuração Global ---

var globalCmd = &cobra.Command{
	Use:   "global",
	Short: "Gerencia a configuração global (~/.crom/global.json)",
}

var globalListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todas as configurações globais",
	RunE: func(cmd *cobra.Command, args []string) error {
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		cfg, err := config.LoadGlobalConfig(gDir)
		if err != nil {
			return err
		}

		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  Configuração Global (~/.crom/global.json)")
		cmd.Println("═══════════════════════════════════════")
		cmd.Printf("  default_provider:                 %s\n", cfg.DefaultProvider)
		cmd.Printf("  default_model:                    %s\n", cfg.DefaultModel)
		cmd.Printf("  max_iterations_default:           %d\n", cfg.MaxIterationsDefault)
		cmd.Printf("  max_consecutive_failures_default: %d\n", cfg.MaxConsecutiveFailDefault)
		cmd.Printf("  max_tokens_per_task_default:      %d\n", cfg.MaxTokensPerTaskDefault)
		cmd.Printf("  tool_timeout_seconds_default:     %d\n", cfg.ToolTimeoutSecondsDefault)
		cmd.Printf("  max_message_history_default:      %d\n", cfg.MaxMessageHistoryDefault)
		cmd.Printf("  log_level:                        %s\n", cfg.LogLevel)
		cmd.Printf("  telemetry_enabled:                %t\n", cfg.TelemetryEnabled)
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

var globalGetCmd = &cobra.Command{
	Use:   "get [chave]",
	Short: "Exibe o valor de uma chave na configuração global",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		cfg, err := config.LoadGlobalConfig(gDir)
		if err != nil {
			return err
		}

		val, err := getGlobalField(cfg, key)
		if err != nil {
			return err
		}
		cmd.Println(val)
		return nil
	},
}

var globalSetCmd = &cobra.Command{
	Use:   "set [chave] [valor]",
	Short: "Define o valor de uma chave na configuração global",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		cfg, err := config.LoadGlobalConfig(gDir)
		if err != nil {
			return err
		}

		if err := setGlobalField(cfg, key, val); err != nil {
			return err
		}

		if err := config.SaveGlobalConfig(gDir, cfg); err != nil {
			return fmt.Errorf("falha ao salvar config global: %w", err)
		}

		cmd.Printf("✓ Configuração global '%s' atualizada para '%s'\n", key, val)
		return nil
	},
}

// --- Comandos de Configuração de Ambiente (.env) ---

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Gerencia as variáveis de ambiente e chaves secretas (~/.crom/.env)",
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todas as variáveis de ambiente (mascaradas por padrão)",
	RunE: func(cmd *cobra.Command, args []string) error {
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		env, err := config.LoadEnvVars(gDir)
		if err != nil {
			return err
		}

		vars := env.All()
		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  Variáveis de Ambiente (~/.crom/.env)")
		cmd.Println("═══════════════════════════════════════")
		if len(vars) == 0 {
			cmd.Println("  Nenhuma variável configurada.")
		} else {
			for k, v := range vars {
				displayVal := config.MaskedValue(v)
				if revealEnv {
					displayVal = v
				}
				cmd.Printf("  %s=%s\n", k, displayVal)
			}
		}
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

var envSetCmd = &cobra.Command{
	Use:   "set [chave] [valor]",
	Short: "Define o valor de uma variável de ambiente",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		env, err := config.LoadEnvVars(gDir)
		if err != nil {
			return err
		}

		env.Set(key, val)
		if err := env.Save(gDir); err != nil {
			return fmt.Errorf("falha ao salvar .env: %w", err)
		}

		cmd.Printf("✓ Variável '%s' salva no .env\n", key)
		return nil
	},
}

// --- Comandos de Configuração de Workspace (config.json) ---

var configWorkspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Gerencia a configuração do workspace local (.crom/config.json)",
}

var configWorkspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista as configurações do workspace atual",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWorkspaceConfig(workspacePath)
		if err != nil {
			return err
		}

		cmd.Println("═══════════════════════════════════════")
		cmd.Printf("  Configuração do Workspace (%s)\n", workspacePath)
		cmd.Println("═══════════════════════════════════════")
		cmd.Printf("  workspace_name:           %s\n", cfg.WorkspaceName)
		cmd.Printf("  provider:                 %s\n", formatOptString(cfg.Provider))
		cmd.Printf("  model:                    %s\n", formatOptString(cfg.Model))
		cmd.Printf("  max_iterations:           %s\n", formatOptInt(cfg.MaxIterations))
		cmd.Printf("  max_consecutive_failures: %s\n", formatOptInt(cfg.MaxConsecutiveFail))
		cmd.Printf("  max_tokens_per_task:      %s\n", formatOptInt(cfg.MaxTokensPerTask))
		cmd.Printf("  tool_timeout_seconds:     %s\n", formatOptInt(cfg.ToolTimeoutSeconds))
		cmd.Printf("  max_message_history:      %s\n", formatOptInt(cfg.MaxMessageHistory))
		cmd.Printf("  permission_mode:          %s\n", cfg.PermissionMode)
		cmd.Printf("  workspace_jail:           %t\n", cfg.WorkspaceJail)
		cmd.Printf("  auto_verify:              %t\n", cfg.AutoVerify)
		cmd.Printf("  allowed_tools:            %s\n", strings.Join(cfg.AllowedTools, ", "))
		cmd.Printf("  blocked_commands:         %s\n", strings.Join(cfg.BlockedCommands, ", "))
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

var configWorkspaceGetCmd = &cobra.Command{
	Use:   "get [chave]",
	Short: "Exibe o valor de uma chave na configuração do workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		cfg, err := config.LoadWorkspaceConfig(workspacePath)
		if err != nil {
			return err
		}

		val, err := getWorkspaceField(cfg, key)
		if err != nil {
			return err
		}
		cmd.Println(val)
		return nil
	},
}

var configWorkspaceSetCmd = &cobra.Command{
	Use:   "set [chave] [valor]",
	Short: "Define o valor de uma chave na configuração do workspace",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		cfg, err := config.LoadWorkspaceConfig(workspacePath)
		if err != nil {
			return err
		}

		if err := setWorkspaceField(cfg, key, val); err != nil {
			return err
		}

		if err := config.SaveWorkspaceConfig(workspacePath, cfg); err != nil {
			return fmt.Errorf("falha ao salvar config do workspace: %w", err)
		}

		cmd.Printf("✓ Configuração de workspace '%s' atualizada para '%s'\n", key, val)
		return nil
	},
}

// --- Comando de Configuração Resolvida ---

var resolvedCmd = &cobra.Command{
	Use:   "resolved",
	Short: "Exibe a configuração final efetiva resultante da hierarquia de precedência",
	RunE: func(cmd *cobra.Command, args []string) error {
		gDir, err := config.GlobalDir()
		if err != nil {
			return err
		}
		global, err := config.LoadGlobalConfig(gDir)
		if err != nil {
			return err
		}

		// Carrega workspace (se existir, senão usa nil ou defaults)
		workspace, err := config.LoadWorkspaceConfig(workspacePath)
		if err != nil {
			// Ignora erro se não existir diretório, mas LoadWorkspaceConfig cria um padrão se não existir.
			// Para resolved, se o workspace path não for configurado ou não for um repo, podemos tolerar.
		}

		flags := getCLIFlags(cmd)
		resolved := config.Resolve(global, workspace, flags)

		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  Configuração Resolvida (Efetiva)")
		cmd.Println("═══════════════════════════════════════")
		cmd.Printf("  Provider:           %s\n", resolved.Provider)
		cmd.Printf("  Model:              %s\n", resolved.Model)
		cmd.Printf("  MaxIterations:      %d\n", resolved.MaxIterations)
		cmd.Printf("  MaxConsecutiveFail: %d\n", resolved.MaxConsecutiveFail)
		cmd.Printf("  MaxTokensPerTask:   %d\n", resolved.MaxTokensPerTask)
		cmd.Printf("  ToolTimeoutSeconds: %d\n", resolved.ToolTimeoutSeconds)
		cmd.Printf("  MaxMessageHistory:  %d\n", resolved.MaxMessageHistory)
		cmd.Printf("  PermissionMode:     %s\n", resolved.PermissionMode)
		cmd.Printf("  WorkspaceJail:      %t\n", resolved.WorkspaceJail)
		cmd.Printf("  AutoVerify:         %t\n", resolved.AutoVerify)
		cmd.Printf("  AllowedTools:       %s\n", strings.Join(resolved.AllowedTools, ", "))
		cmd.Printf("  BlockedCommands:    %s\n", strings.Join(resolved.BlockedCommands, ", "))
		cmd.Printf("  LogLevel:           %s\n", resolved.LogLevel)
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

func init() {
	// Registrar comandos e subcomandos
	globalCmd.AddCommand(globalListCmd, globalGetCmd, globalSetCmd)
	envCmd.AddCommand(envListCmd, envSetCmd)
	envListCmd.Flags().BoolVar(&revealEnv, "reveal", false, "Exibe os valores reais das chaves de API sem máscara")
	configWorkspaceCmd.AddCommand(configWorkspaceListCmd, configWorkspaceGetCmd, configWorkspaceSetCmd)

	configCmd.AddCommand(globalCmd, envCmd, configWorkspaceCmd, resolvedCmd)
	rootCmd.AddCommand(configCmd)
}

// --- Helpers de Formatação ---

func formatOptString(s string) string {
	if s == "" {
		return "<não definido (usa global)>"
	}
	return s
}

func formatOptInt(i *int) string {
	if i == nil {
		return "<não definido (usa global)>"
	}
	return strconv.Itoa(*i)
}

// --- Helpers de Atribuição Dinâmica ---

func setGlobalField(cfg *config.GlobalConfig, key, val string) error {
	switch key {
	case "default_provider":
		cfg.DefaultProvider = val
	case "default_model":
		cfg.DefaultModel = val
	case "max_iterations_default":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxIterationsDefault = v
	case "max_consecutive_failures_default":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxConsecutiveFailDefault = v
	case "max_tokens_per_task_default":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxTokensPerTaskDefault = v
	case "tool_timeout_seconds_default":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.ToolTimeoutSecondsDefault = v
	case "max_message_history_default":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxMessageHistoryDefault = v
	case "log_level":
		cfg.LogLevel = val
	case "telemetry_enabled":
		v, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("valor inválido para bool: %w", err)
		}
		cfg.TelemetryEnabled = v
	default:
		return fmt.Errorf("chave global desconhecida: %s", key)
	}
	return nil
}

func getGlobalField(cfg *config.GlobalConfig, key string) (string, error) {
	switch key {
	case "default_provider":
		return cfg.DefaultProvider, nil
	case "default_model":
		return cfg.DefaultModel, nil
	case "max_iterations_default":
		return strconv.Itoa(cfg.MaxIterationsDefault), nil
	case "max_consecutive_failures_default":
		return strconv.Itoa(cfg.MaxConsecutiveFailDefault), nil
	case "max_tokens_per_task_default":
		return strconv.Itoa(cfg.MaxTokensPerTaskDefault), nil
	case "tool_timeout_seconds_default":
		return strconv.Itoa(cfg.ToolTimeoutSecondsDefault), nil
	case "max_message_history_default":
		return strconv.Itoa(cfg.MaxMessageHistoryDefault), nil
	case "log_level":
		return cfg.LogLevel, nil
	case "telemetry_enabled":
		return strconv.FormatBool(cfg.TelemetryEnabled), nil
	default:
		return "", fmt.Errorf("chave global desconhecida: %s", key)
	}
}

func setWorkspaceField(cfg *config.WorkspaceConfig, key, val string) error {
	switch key {
	case "workspace_name":
		cfg.WorkspaceName = val
	case "provider":
		cfg.Provider = val
	case "model":
		cfg.Model = val
	case "max_iterations":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxIterations = &v
	case "max_consecutive_failures":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxConsecutiveFail = &v
	case "max_tokens_per_task":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxTokensPerTask = &v
	case "tool_timeout_seconds":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.ToolTimeoutSeconds = &v
	case "max_message_history":
		v, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("valor inválido para int: %w", err)
		}
		cfg.MaxMessageHistory = &v
	case "permission_mode":
		cfg.PermissionMode = val
	case "workspace_jail":
		v, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("valor inválido para bool: %w", err)
		}
		cfg.WorkspaceJail = v
	case "auto_verify":
		v, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("valor inválido para bool: %w", err)
		}
		cfg.AutoVerify = v
	case "allowed_tools":
		if val == "" {
			cfg.AllowedTools = nil
		} else {
			parts := strings.Split(val, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.AllowedTools = parts
		}
	case "blocked_commands":
		if val == "" {
			cfg.BlockedCommands = nil
		} else {
			parts := strings.Split(val, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.BlockedCommands = parts
		}
	default:
		return fmt.Errorf("chave de workspace desconhecida: %s", key)
	}
	return nil
}

func getWorkspaceField(cfg *config.WorkspaceConfig, key string) (string, error) {
	switch key {
	case "workspace_name":
		return cfg.WorkspaceName, nil
	case "provider":
		return cfg.Provider, nil
	case "model":
		return cfg.Model, nil
	case "max_iterations":
		if cfg.MaxIterations == nil {
			return "<não definido>", nil
		}
		return strconv.Itoa(*cfg.MaxIterations), nil
	case "max_consecutive_failures":
		if cfg.MaxConsecutiveFail == nil {
			return "<não definido>", nil
		}
		return strconv.Itoa(*cfg.MaxConsecutiveFail), nil
	case "max_tokens_per_task":
		if cfg.MaxTokensPerTask == nil {
			return "<não definido>", nil
		}
		return strconv.Itoa(*cfg.MaxTokensPerTask), nil
	case "tool_timeout_seconds":
		if cfg.ToolTimeoutSeconds == nil {
			return "<não definido>", nil
		}
		return strconv.Itoa(*cfg.ToolTimeoutSeconds), nil
	case "max_message_history":
		if cfg.MaxMessageHistory == nil {
			return "<não definido>", nil
		}
		return strconv.Itoa(*cfg.MaxMessageHistory), nil
	case "permission_mode":
		return cfg.PermissionMode, nil
	case "workspace_jail":
		return strconv.FormatBool(cfg.WorkspaceJail), nil
	case "auto_verify":
		return strconv.FormatBool(cfg.AutoVerify), nil
	case "allowed_tools":
		return strings.Join(cfg.AllowedTools, ","), nil
	case "blocked_commands":
		return strings.Join(cfg.BlockedCommands, ","), nil
	default:
		return "", fmt.Errorf("chave de workspace desconhecida: %s", key)
	}
}
