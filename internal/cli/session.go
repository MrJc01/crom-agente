package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crom/crom-agente/internal/state"
	"github.com/spf13/cobra"
)

// sessionCmd representa a base para comandos de sessões
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Gerencia as sessões de chat isoladas do workspace",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todas as sessões de chat registradas no workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionsDir := filepath.Join(storagePath, "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			if os.IsNotExist(err) {
				cmd.Println("Nenhuma sessão criada neste workspace.")
				return nil
			}
			return err
		}

		cmd.Println("═══════════════════════════════════════")
		cmd.Println("  Sessões de Chat Registradas")
		cmd.Println("═══════════════════════════════════════")
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				cmd.Printf("  - %s\n", entry.Name())
				count++
			}
		}
		if count == 0 {
			cmd.Println("  Nenhuma sessão encontrada.")
		}
		cmd.Println("═══════════════════════════════════════")
		return nil
	},
}

var sessionCreateCmd = &cobra.Command{
	Use:   "create [nome]",
	Short: "Cria uma nova sessão de chat no workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sm := state.NewSessionStateManager(storagePath, name)
		if err := sm.LoadState(); err != nil {
			return err
		}
		cmd.Printf("✓ Sessão '%s' criada com sucesso.\n", name)
		return nil
	},
}

var sessionDeleteCmd = &cobra.Command{
	Use:   "delete [nome]",
	Short: "Exclui uma sessão de chat e seu histórico",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sessionDir := filepath.Join(storagePath, "sessions", name)
		info, err := os.Stat(sessionDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("sessão '%s' não encontrada", name)
			}
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("sessão '%s' não encontrada", name)
		}
		if err := os.RemoveAll(sessionDir); err != nil {
			return err
		}
		cmd.Printf("✓ Sessão '%s' excluída com sucesso.\n", name)
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionListCmd, sessionCreateCmd, sessionDeleteCmd)
	rootCmd.AddCommand(sessionCmd)
}
