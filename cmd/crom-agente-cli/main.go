package main

import (
	"fmt"
	"os"

	"github.com/crom/crom-agente/internal/cli-tui"
	"github.com/spf13/cobra"
)

var (
	workspacePath  string
	storagePath    string
	cliSession     string
	cliProvider    string
	cliModel       string
	cliTimeout     int
	cliPermissions string
)

var rootCmd = &cobra.Command{
	Use:   "crom-agente-cli",
	Short: "Interface de Terminal Interativa (TUI) para crom-agente",
	Long: `crom-agente-cli fornece uma experiência de chat interativa rica (REPL/TUI) 
baseada em terminal para co-programar com o crom-agente.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Inicializa e executa a TUI do Bubble Tea
		return tui.Start(tui.Options{
			WorkspacePath:  workspacePath,
			StoragePath:    storagePath,
			SessionName:    cliSession,
			Provider:       cliProvider,
			Model:          cliModel,
			TimeoutSeconds: cliTimeout,
			PermissionMode: cliPermissions,
		})
	},
}

func init() {
	rootCmd.Flags().StringVarP(&workspacePath, "workspace", "w", ".", "Caminho para o workspace do projeto")
	rootCmd.Flags().StringVarP(&storagePath, "storage", "s", ".crom", "Diretório de armazenamento do estado do agente")
	rootCmd.Flags().StringVar(&cliSession, "session", "cli-session", "Nome da sessão de chat/execução")
	rootCmd.Flags().StringVar(&cliProvider, "provider", "", "Override do provedor de LLM (openai, gemini, anthropic, ollama, openrouter)")
	rootCmd.Flags().StringVar(&cliModel, "model", "", "Override do modelo de LLM")
	rootCmd.Flags().IntVar(&cliTimeout, "timeout", 30, "Override de timeout para execução de ferramentas (segundos)")
	rootCmd.Flags().StringVar(&cliPermissions, "permission-mode", "scoped", "Modo de permissão (total_access, ask_every_time, scoped)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao iniciar CLI: %v\n", err)
		os.Exit(1)
	}
}
