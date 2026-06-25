package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/crom/crom-agente/internal/daemon"
	"github.com/spf13/cobra"
)

var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Verifica a saude dos componentes do sistema (Daemon, IPC, Rede)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("🩺 Iniciando diagnostico de saude (Healthcheck)...")

		allOk := true

		// 1. Checa IPC / Daemon
		cmd.Printf("[1/3] Checando Daemon IPC... ")
		sockPath, err := daemon.SocketPath()
		if err != nil {
			cmd.Printf("❌ ERRO (%v)\n", err)
			allOk = false
		} else {
			conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
			if err != nil {
				cmd.Printf("❌ INATIVO (%v)\n", err)
				allOk = false
			} else {
				conn.Close()
				cmd.Printf("✅ OK\n")
			}
		}

		// 2. Checa Portas padroes (9090 API HTTP)
		cmd.Printf("[2/3] Checando API HTTP local... ")
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://localhost:9090/api/status") // endpoint genérico de teste
		if err != nil {
			// Pode nao estar habilitado, mas vamos verificar se a porta responde
			conn, err := net.DialTimeout("tcp", "localhost:9090", 2*time.Second)
			if err != nil {
				cmd.Printf("❌ INATIVO (Porta 9090 fechada)\n")
				allOk = false
			} else {
				conn.Close()
				cmd.Printf("✅ OK (Porta aberta, mas endpoint falhou)\n")
			}
		} else {
			resp.Body.Close()
			cmd.Printf("✅ OK\n")
		}

		// 3. Checa Conectividade com a Cloud
		cmd.Printf("[3/3] Checando conectividade Cloud (CromIA API)... ")
		respCloud, err := client.Get("https://cloud.ia.crom.run")
		if err != nil {
			cmd.Printf("❌ FALHA (%v)\n", err)
			allOk = false
		} else {
			respCloud.Body.Close()
			cmd.Printf("✅ OK\n")
		}

		fmt.Println("--------------------------------------------------")
		if allOk {
			cmd.Println("✨ Sistema saudavel e operacional!")
			return nil
		}

		cmd.Println("⚠️ O sistema apresenta falhas. Verifique os logs e dependencias.")
		os.Exit(1)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(healthcheckCmd)
}
