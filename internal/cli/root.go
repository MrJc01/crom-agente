package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/daemon"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/orchestrator"
	"github.com/crom/crom-agente/internal/permission"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/spf13/cobra"
)

// Version é preenchido em tempo de compilação via ldflags
var Version = "dev"

// storagePath define o diretório padrão de armazenamento do estado do agente
var storagePath string

// Flags globais de configuração e override
var workspacePath string
var cliProvider string
var cliModel string
var cliMaxIterations int
var cliMaxFailures int
var cliTimeout int
var cliMaxHistory int
var cliPermissionMode string
var cliSession string
var cliDisablePromptOptimization bool

// rootCmd é o comando raiz do crom-agente
var rootCmd = &cobra.Command{
	Use:   "crom-agente",
	Short: "Orquestrador de agentes autônomos em Go",
	Long: `crom-agente é um orquestrador de agentes autônomos de alta performance.
Ele executa tarefas de forma iterativa através de um ciclo ReAct 
(Reasoning and Acting), com suporte a ferramentas nativas, 
subagentes concorrentes e múltiplos provedores de LLM.`,
}

// versionCmd exibe a versão atual do binário
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Exibe a versão do crom-agente",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("crom-agente %s\n", Version)
	},
}

// stateCmd exibe o estado atual do agente persistido no disco
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Exibe o estado atual do agente",
	RunE: func(cmd *cobra.Command, args []string) error {
		var sm *state.StateManager
		if cliSession != "" {
			sm = state.NewSessionStateManager(storagePath, cliSession)
		} else {
			sm = state.NewStateManager(storagePath)
		}
		if err := sm.LoadState(); err != nil {
			return fmt.Errorf("falha ao carregar estado: %w", err)
		}

		s := sm.GetState()
		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  crom-agente :: Estado Atual")
		cmd.Println("═══════════════════════════════════════")
		cmd.Printf("  Status:     %s\n", s.UltimoStatus)
		cmd.Printf("  Tarefa:     %s\n", s.TarefaEmAndamento)
		cmd.Printf("  Tokens:     %d\n", s.TokensGastos)
		cmd.Printf("  Turnos:     %d\n", s.TotalTurnos)
		cmd.Printf("  Diretório:  %s\n", s.DiretorioAtual)
		cmd.Printf("  Timestamp:  %s\n", s.Timestamp.Format("2006-01-02 15:04:05"))
		cmd.Println("═══════════════════════════════════════")

		if len(s.LogsRelevantes) > 0 {
			cmd.Println("  Logs Recentes:")
			for i, log := range s.LogsRelevantes {
				cmd.Printf("    [%d] %s\n", i+1, log)
			}
		}

		return nil
	},
}

// cliEventHandler imprime eventos no terminal em tempo real
type cliEventHandler struct {
	out io.Writer
}

func (h *cliEventHandler) OnStatusChange(status string) {
	fmt.Fprintf(h.out, " [status] %s...\n", status)
}

func (h *cliEventHandler) OnMessage(role string, content string) {
	switch role {
	case "assistant":
		fmt.Fprintf(h.out, "\n🤖 Assistant:\n%s\n\n", content)
	case "system":
		fmt.Fprintf(h.out, "⚙️ System: %s\n", content)
	case "user":
		fmt.Fprintf(h.out, "👤 User: %s\n", content)
	case "tool":
		fmt.Fprintf(h.out, "🛠️ Tool Result: %s\n", content)
	}
}

func (h *cliEventHandler) OnEvent(event loop.AgentEvent) {
	switch event.Event {
	case "thinking":
		provider, _ := event.Data["provider"].(string)
		model, _ := event.Data["model"].(string)
		fmt.Fprintf(h.out, "  💭 [iter %d] Pensando... (%s/%s)\n", event.Iteration, provider, model)
	case "message":
		// Mensagem já tratada pelo OnMessage legado
	case "tool_call":
		toolName, _ := event.Data["tool"].(string)
		fmt.Fprintf(h.out, "  🔧 [iter %d] Chamando: %s\n", event.Iteration, toolName)
	case "tool_result":
		toolName, _ := event.Data["tool"].(string)
		success, _ := event.Data["success"].(bool)
		if success {
			fmt.Fprintf(h.out, "  ✅ [iter %d] %s: OK\n", event.Iteration, toolName)
		} else {
			errMsg, _ := event.Data["error"].(string)
			fmt.Fprintf(h.out, "  ❌ [iter %d] %s: %s\n", event.Iteration, toolName, errMsg)
		}
	case "error":
		if errData, ok := event.Data["error"]; ok {
			if agentErr, ok := errData.(loop.AgentError); ok {
				fmt.Fprintf(h.out, "  ⚠️  [iter %d] ERRO [%s]: %s\n", event.Iteration, agentErr.Code, agentErr.Message)
			} else {
				fmt.Fprintf(h.out, "  ⚠️  [iter %d] ERRO: %v\n", event.Iteration, errData)
			}
		}
	case "finished":
		reason, _ := event.Data["reason"].(string)
		totalIter, _ := event.Data["total_iterations"].(int)
		fmt.Fprintf(h.out, "  🏁 Finalizado (%s) em %d iterações.\n", reason, totalIter)
	}
}

// runCmd executa a tarefa instanciando o ReAct loop com as configurações resolvidas
var runCmd = &cobra.Command{
	Use:   "run [tarefa]",
	Short: "Executa uma tarefa utilizando o agente",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]

		// Tenta conectar ao daemon via socket Unix
		sockPath, err := daemon.SocketPath()
		if err == nil {
			conn, err := net.Dial("unix", sockPath)
			if err == nil {
				defer conn.Close()

				absPath, err := filepath.Abs(workspacePath)
				if err != nil {
					return err
				}

				wsList, _ := orchestrator.LoadWorkspaces()
				var targetWsName string
				for _, ws := range wsList {
					if ws.Path == absPath {
						targetWsName = ws.Name
						break
					}
				}

				if targetWsName == "" {
					targetWsName = filepath.Base(absPath)
					mgr := orchestrator.NewMultiAgentManager()
					_ = mgr.AddWorkspace(targetWsName, absPath)
				}

				enc := json.NewEncoder(conn)
				dec := json.NewDecoder(conn)

				req := daemon.IPCMessage{
					Type:      "run",
					Workspace: targetWsName,
					Session:   cliSession,
					Task:      task,
					Provider:  cliProvider,
					Model:     cliModel,
				}

				if err := enc.Encode(req); err != nil {
					return err
				}

				for {
					var resp daemon.IPCResponse
					if err := dec.Decode(&resp); err != nil {
						if err == io.EOF {
							break
						}
						return err
					}

					if !resp.Success {
						return fmt.Errorf("daemon erro: %s", resp.Error)
					}

					var evt struct {
						Type    string `json:"type"`
						Status  string `json:"status"`
						Role    string `json:"role"`
						Content string `json:"content"`
						Action  string `json:"action"`
						Target  string `json:"target"`
					}
					_ = json.Unmarshal(resp.Data, &evt)

					switch evt.Type {
					case "started":
						cmd.Printf("⚡ crom-agente (via Daemon) :: Iniciando execucao...\n")
						cmd.Printf("⚡ Tarefa: %q\n", task)
						cmd.Println("═══════════════════════════════════════")
					case "status":
						cmd.Printf(" [status] %s...\n", evt.Status)
					case "message":
						switch evt.Role {
						case "assistant":
							cmd.Printf("\n🤖 Assistant:\n%s\n\n", evt.Content)
						case "system":
							cmd.Printf("⚙️ System: %s\n", evt.Content)
						case "user":
							cmd.Printf("👤 User: %s\n", evt.Content)
						case "tool":
							cmd.Printf("🛠️ Tool Result: %s\n", evt.Content)
						}
					case "ask_permission":
						cmd.Printf("\n⚠️  [HITL (Daemon)] crom-agente solicita permissao para a acao [%s] no alvo: %q\n", evt.Action, evt.Target)
						cmd.Print("👉 Pressione [a] para aprovar uma vez, [s] para sempre permitir, [r] para rejeitar: ")
						var response string
						_, _ = fmt.Scanln(&response)
						response = strings.TrimSpace(strings.ToLower(response))

						approved := false
						remember := false
						if response == "s" {
							approved = true
							remember = true
						} else if response == "a" {
							approved = true
						}

						respPayload, _ := json.Marshal(map[string]bool{
							"approved": approved,
							"remember": remember,
						})
						reply := daemon.IPCMessage{
							Type:    "permission_response",
							Payload: respPayload,
						}
						_ = enc.Encode(reply)
					}

					if !resp.Stream {
						break
					}
				}

				cmd.Println("═══════════════════════════════════════")
				cmd.Println("✓ Execucao via Daemon concluida.")
				return nil
			}
		}

		// Fallback: execucao standalone
		// 1. Carregar diretório global
		gDir, err := config.GlobalDir()
		if err != nil {
			return fmt.Errorf("falha ao obter diretório global: %w", err)
		}

		// 2. Carregar configuração global
		global, err := config.LoadGlobalConfig(gDir)
		if err != nil {
			return fmt.Errorf("falha ao carregar configuração global: %w", err)
		}

		// 3. Carregar env vars (.env)
		env, err := config.LoadEnvVars(gDir)
		if err != nil {
			return fmt.Errorf("falha ao carregar variáveis de ambiente: %w", err)
		}

		// 4. Carregar configuração do workspace
		workspace, err := config.LoadWorkspaceConfig(workspacePath)
		if err != nil {
			return fmt.Errorf("falha ao carregar configuração do workspace: %w", err)
		}

		// 5. Resolver configuração final com CLI flags
		flags := getCLIFlags(cmd)
		resolved := config.Resolve(global, workspace, flags)

		// 6. Instanciar LLM Provider
		provider, err := llm.NewProvider(resolved.Provider, resolved.Model, func(key string) string {
			return env.Get(key)
		})
		if err != nil {
			return fmt.Errorf("falha ao criar provedor de LLM: %w", err)
		}

		// 7. Instanciar StateManager
		var sm *state.StateManager
		if cliSession != "" {
			sm = state.NewSessionStateManager(storagePath, cliSession)
		} else {
			sm = state.NewStateManager(storagePath)
		}
		if err := sm.LoadState(); err != nil {
			return fmt.Errorf("falha ao carregar estado: %w", err)
		}

		// 8. Inicializar o PermissionManager interativo
		askFunc := func(ctx context.Context, action, target string) (bool, bool) {
			cmd.Printf("\n⚠️  [HITL] crom-agente solicita permissão para a ação [%s] no alvo: %q\n", action, target)
			cmd.Print("👉 Pressione [a] para aprovar uma vez, [s] para sempre permitir, [r] para rejeitar: ")
			var response string
			_, _ = fmt.Scanln(&response)
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "s" {
				return true, true
			}
			if response == "a" {
				return true, false
			}
			return false, false
		}
		pm := permission.NewPermissionManager(workspacePath, resolved.PermissionMode, askFunc)

		// 9. Executar loop ReAct
		handler := &cliEventHandler{out: cmd.OutOrStdout()}
		al := loop.New(provider, sm, handler, resolved)

		// Registrar ferramentas nativas
		al.RegisterTool(tools.NewScheduleTimerTool(workspacePath, nil))
		al.RegisterTool(tools.NewReadFileTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewWriteFileTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewTerminalCommandTool(workspacePath, resolved.BlockedCommands, cmd.OutOrStdout()))
		al.RegisterTool(tools.NewDiffReplaceTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewRenameFileTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewDeleteFileTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewTreeTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewGrepTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewPortMonitorTool(workspacePath))
		al.RegisterTool(tools.NewGitStatusTool(workspacePath))
		al.RegisterTool(tools.NewGitLogTool(workspacePath))
		al.RegisterTool(tools.NewGitDiffTool(workspacePath))
		al.RegisterTool(tools.NewGitAddTool(workspacePath))
		al.RegisterTool(tools.NewGitCommitTool(workspacePath))
		al.RegisterTool(tools.NewGitBranchTool(workspacePath))
		al.RegisterTool(tools.NewGitConflictTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewHTTPClientTool(workspacePath))
		al.RegisterTool(tools.NewScraperTool(workspacePath))
		browserTool := tools.NewBrowserTool(workspacePath, resolved.BrowserHeadless)
		browserTool.SetOnNavigate(func(url string) {
			_ = sm.SetBrowserURL(url)
		})
		browserTool.SetRestoreURL(func() string {
			return sm.GetBrowserURL()
		})
		al.RegisterTool(browserTool)
		al.RegisterTool(tools.NewComputerControlTool(workspacePath))
		al.RegisterTool(tools.NewDatabaseTesterTool(workspacePath))
		al.RegisterTool(tools.NewProxyTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewRunTestsTool(workspacePath))


		al.RegisterTool(tools.NewStackTranslatorTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewDocGeneratorTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewCodeExplainerTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewMockGeneratorTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewComplexityReducerTool(workspacePath, resolved.WorkspaceJail))
		al.RegisterTool(tools.NewMemoryLeakScannerTool(workspacePath, resolved.WorkspaceJail))

		// Carrega ferramentas dinâmicas da pasta .crom/tools do workspace
		dynamicToolsDir := filepath.Join(workspacePath, ".crom", "tools")
		if loadedTools, err := tools.LoadScriptsFromDir(dynamicToolsDir); err == nil {
			for _, t := range loadedTools {
				al.RegisterTool(t)
			}
		}

		al.SetPermissionManager(pm)



		cmd.Printf("⚡ crom-agente :: Iniciando execução com provedor %q (modelo: %q)\n", resolved.Provider, resolved.Model)
		cmd.Printf("⚡ Tarefa: %q\n", task)
		cmd.Println("═══════════════════════════════════════")

		ctx := context.Background()
		if err := al.Execute(ctx, task); err != nil {
			return fmt.Errorf("falha na execução da tarefa: %w", err)
		}

		cmd.Println("═══════════════════════════════════════")
		cmd.Println("✓ Execução concluída com sucesso.")
		return nil
	},
}

// getCLIFlags obtém as flags de override se foram alteradas
func getCLIFlags(cmd *cobra.Command) config.CLIFlags {
	var flags config.CLIFlags
	if cmd.Flags().Changed("provider") {
		flags.Provider = cliProvider
	}
	if cmd.Flags().Changed("model") {
		flags.Model = cliModel
	}
	if cmd.Flags().Changed("max-iterations") {
		flags.MaxIterations = &cliMaxIterations
	}
	if cmd.Flags().Changed("max-failures") {
		flags.MaxConsecutiveFail = &cliMaxFailures
	}
	if cmd.Flags().Changed("timeout") {
		flags.ToolTimeoutSeconds = &cliTimeout
	}
	if cmd.Flags().Changed("max-history") {
		flags.MaxMessageHistory = &cliMaxHistory
	}
	if cmd.Flags().Changed("permission-mode") {
		flags.PermissionMode = cliPermissionMode
	}
	if cmd.Flags().Changed("disable-prompt-optimization") {
		flags.DisablePromptOptimization = &cliDisablePromptOptimization
	}
	return flags
}

func init() {
	rootCmd.PersistentFlags().StringVar(&storagePath, "storage", ".crom", "Diretório de armazenamento do estado do agente")
	rootCmd.PersistentFlags().StringVar(&workspacePath, "workspace", ".", "Caminho para o workspace do projeto")
	rootCmd.PersistentFlags().StringVar(&cliSession, "session", "", "ID ou nome da sessão de chat no workspace")

	rootCmd.PersistentFlags().StringVar(&cliProvider, "provider", "", "Override: Provedor de LLM (openai, gemini, anthropic, ollama, openrouter)")
	rootCmd.PersistentFlags().StringVar(&cliModel, "model", "", "Override: Modelo de LLM (ex: gpt-4o)")
	rootCmd.PersistentFlags().IntVar(&cliMaxIterations, "max-iterations", 0, "Override: Máximo de iterações do loop ReAct")
	rootCmd.PersistentFlags().IntVar(&cliMaxFailures, "max-failures", 0, "Override: Máximo de falhas consecutivas de ferramentas")
	rootCmd.PersistentFlags().IntVar(&cliTimeout, "timeout", 0, "Override: Timeout para execução de ferramentas (segundos)")
	rootCmd.PersistentFlags().IntVar(&cliMaxHistory, "max-history", 0, "Override: Limite de mensagens mantidas no histórico")
	rootCmd.PersistentFlags().StringVar(&cliPermissionMode, "permission-mode", "", "Override: Modo de permissão (total_access, ask_every_time, scoped)")
	rootCmd.PersistentFlags().BoolVar(&cliDisablePromptOptimization, "disable-prompt-optimization", false, "Desabilita a otimização de prompt inicial")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(stateCmd)
	rootCmd.AddCommand(runCmd)
}

// Execute é o ponto de entrada público chamado pelo main.go
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
