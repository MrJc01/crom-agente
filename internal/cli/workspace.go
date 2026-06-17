package cli

import (
	"encoding/json"
	"net"
	"path/filepath"

	"github.com/crom/crom-agente/internal/daemon"
	"github.com/crom/crom-agente/internal/orchestrator"
	"github.com/crom/crom-agente/internal/state"
	"github.com/spf13/cobra"
)

var workspaceNameFlag string
var showAllStatus bool

// workspaceCmd representa a base para comandos de workspaces
var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Gerencia os workspaces de projetos registrados",
}

var workspaceAddCmd = &cobra.Command{
	Use:   "add [caminho]",
	Short: "Registra um novo workspace no orquestrador",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		name := workspaceNameFlag
		if name == "" {
			name = filepath.Base(path)
		}

		mgr := orchestrator.NewMultiAgentManager()
		if err := mgr.AddWorkspace(name, path); err != nil {
			return err
		}
		cmd.Printf("✓ Workspace '%s' registrado com sucesso em '%s'\n", name, path)
		return nil
	},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todos os workspaces registrados",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := orchestrator.LoadWorkspaces()
		if err != nil {
			return err
		}

		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  Workspaces Registrados")
		cmd.Println("═══════════════════════════════════════")
		if len(list) == 0 {
			cmd.Println("  Nenhum workspace registrado. Use 'workspace add <path>'")
		} else {
			for _, ws := range list {
				cmd.Printf("  %-15s -> %s\n", ws.Name, ws.Path)
			}
		}
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove [nome]",
	Short: "Remove um workspace registrado",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		mgr := orchestrator.NewMultiAgentManager()
		if err := mgr.RemoveWorkspace(name); err != nil {
			return err
		}
		cmd.Printf("✓ Workspace '%s' removido do registro\n", name)
		return nil
	},
}

// statusCmd substitui/amplia a visualização do estado com suporte a múltiplos workspaces
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Exibe o status do agente no workspace atual ou de todos os workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Tenta obter status via daemon
		sockPath, err := daemon.SocketPath()
		if err == nil {
			conn, err := net.Dial("unix", sockPath)
			if err == nil {
				defer conn.Close()

				enc := json.NewEncoder(conn)
				dec := json.NewDecoder(conn)

				req := daemon.IPCMessage{Type: "status"}
				if err := enc.Encode(req); err == nil {
					var resp daemon.IPCResponse
					if err := dec.Decode(&resp); err == nil && resp.Success {
						type WsStatus struct {
							Name   string `json:"name"`
							Status string `json:"status"`
							Task   string `json:"task"`
						}
						var list []WsStatus
						if err := json.Unmarshal(resp.Data, &list); err == nil {
							if showAllStatus {
								cmd.Println("════════════════════════════════════════════════════════")
								cmd.Println("  Status de Todos os Workspaces (via Daemon)")
								cmd.Println("════════════════════════════════════════════════════════")
								if len(list) == 0 {
									cmd.Println("  Nenhum workspace registrado.")
								} else {
									for _, ws := range list {
										taskStr := ws.Task
										if taskStr == "" {
											taskStr = "<nenhuma>"
										}
										cmd.Printf("  Workspace: %-15s | Status: %-10s | Tarefa: %s\n", ws.Name, ws.Status, taskStr)
									}
								}
								cmd.Println("════════════════════════════════════════════════════════")
								return nil
							} else {
								// Filtra pelo workspace atual
								absPath, err := filepath.Abs(workspacePath)
								if err == nil {
									wsList, _ := orchestrator.LoadWorkspaces()
									var currentWsName string
									for _, ws := range wsList {
										if ws.Path == absPath {
											currentWsName = ws.Name
											break
										}
									}
									if currentWsName != "" {
										for _, ws := range list {
											if ws.Name == currentWsName {
												cmd.Println("═══════════════════════════════════════")
												cmd.Println("  crom-agente :: Estado Atual (via Daemon)")
												cmd.Println("═══════════════════════════════════════")
												cmd.Printf("  Workspace:  %s\n", ws.Name)
												cmd.Printf("  Status:     %s\n", ws.Status)
												cmd.Printf("  Tarefa:     %s\n", ws.Task)
												cmd.Println("═══════════════════════════════════════")
												return nil
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		if showAllStatus {
			list, err := orchestrator.LoadWorkspaces()
			if err != nil {
				return err
			}

			cmd.Println("════════════════════════════════════════════════════════")
			cmd.Println("  Status de Todos os Workspaces")
			cmd.Println("════════════════════════════════════════════════════════")
			if len(list) == 0 {
				cmd.Println("  Nenhum workspace registrado.")
			} else {
				for _, ws := range list {
					sm := state.NewStateManager(filepath.Join(ws.Path, ".crom"))
					statusStr := "idle"
					taskStr := "<nenhuma>"
					if err := sm.LoadState(); err == nil {
						s := sm.GetState()
						statusStr = s.UltimoStatus
						if s.TarefaEmAndamento != "" {
							taskStr = s.TarefaEmAndamento
						}
					}
					cmd.Printf("  Workspace: %-15s | Status: %-10s | Tarefa: %s\n", ws.Name, statusStr, taskStr)
				}
			}
			cmd.Println("════════════════════════════════════════════════════════")
			return nil
		}

		// Se não passar a flag --all, delega ao comando normal stateCmd
		return stateCmd.RunE(cmd, args)
	},
}

func init() {
	workspaceAddCmd.Flags().StringVar(&workspaceNameFlag, "name", "", "Nome customizado para identificar o workspace")
	workspaceCmd.AddCommand(workspaceAddCmd, workspaceListCmd, workspaceRemoveCmd)

	statusCmd.Flags().BoolVar(&showAllStatus, "all", false, "Exibe o status de todos os workspaces registrados")

	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(statusCmd)
}
