package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/crom/crom-agente/internal/daemon"
	"github.com/spf13/cobra"
)

var headlessFlag bool
var enableAutostart bool
var disableAutostart bool

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Gerencia o daemon persistente do crom-agente",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Inicia o daemon persistente",
	RunE: func(cmd *cobra.Command, args []string) error {
		d := daemon.NewDaemon(headlessFlag)
		cmd.Printf("🟢 Iniciando daemon crom-agente (headless: %v)...\n", headlessFlag)
		return d.Start()
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Para o daemon persistente que esta em execucao",
	RunE: func(cmd *cobra.Command, args []string) error {
		pidPath, err := daemon.PIDPath()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(pidPath)
		if err != nil {
			if os.IsNotExist(err) {
				cmd.Println("⚪ O daemon nao esta rodando (arquivo PID nao encontrado).")
				return nil
			}
			return err
		}

		var pid int
		_, err = fmt.Sscanf(string(data), "%d", &pid)
		if err != nil {
			return fmt.Errorf("arquivo PID corrompido: %w", err)
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}

		// Envia sinal SIGTERM
		cmd.Printf("🔴 Enviando sinal de interrupcao para o daemon (PID: %d)...\n", pid)
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			cmd.Printf("⚠️ Falha ao enviar sinal SIGTERM: %v. Removendo arquivo PID.\n", err)
			_ = os.Remove(pidPath)
			return nil
		}

		// Aguarda ate 5 segundos para desligamento gracioso
		for i := 0; i < 50; i++ {
			err = process.Signal(syscall.Signal(0))
			if err != nil {
				cmd.Println("🔴 Daemon encerrado com sucesso.")
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Se ainda estiver vivo, forca parada
		cmd.Println("⚠️ O daemon nao respondeu ao desligamento gracioso. Forcando parada...")
		_ = process.Kill()
		_ = os.Remove(pidPath)
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Verifica o status do daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath, err := daemon.SocketPath()
		if err != nil {
			return err
		}

		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			cmd.Println("🔴 Daemon inativo (nao foi possivel conectar ao socket Unix).")
			return nil
		}
		defer conn.Close()

		enc := json.NewEncoder(conn)
		dec := json.NewDecoder(conn)

		// Solicita status dos workspaces
		req := daemon.IPCMessage{Type: "status"}
		if err := enc.Encode(req); err != nil {
			return err
		}

		var resp daemon.IPCResponse
		if err := dec.Decode(&resp); err != nil {
			return err
		}

		if !resp.Success {
			return fmt.Errorf("erro ao obter status: %s", resp.Error)
		}

		type WsStatus struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Task   string `json:"task"`
		}
		var list []WsStatus
		if err := json.Unmarshal(resp.Data, &list); err != nil {
			return err
		}

		pidPath, _ := daemon.PIDPath()
		pidBytes, _ := os.ReadFile(pidPath)
		pidStr := stringsTrim(string(pidBytes))

		cmd.Println("════════════════════════════════════════════════════════")
		cmd.Printf("  🟢 Daemon Ativo (PID: %s)\n", pidStr)
		cmd.Println("════════════════════════════════════════════════════════")
		if len(list) == 0 {
			cmd.Println("  Nenhum workspace registrado no orquestrador.")
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
	},
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Reinicia o daemon persistente",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := daemonStopCmd.RunE(cmd, args)
		if err != nil {
			cmd.Printf("⚠️ Erro ao parar o daemon: %v. Tentando iniciar...\n", err)
		}
		time.Sleep(500 * time.Millisecond)
		return daemonStartCmd.RunE(cmd, args)
	},
}

var daemonAutostartCmd = &cobra.Command{
	Use:   "autostart",
	Short: "Configura a inicializacao automatica com a sessao grafica",
	RunE: func(cmd *cobra.Command, args []string) error {
		if enableAutostart {
			err := daemon.ConfigureAutostart(true)
			if err != nil {
				return err
			}
			cmd.Println("✓ Inicializacao automatica habilitada com sucesso.")
			return nil
		}

		if disableAutostart {
			err := daemon.ConfigureAutostart(false)
			if err != nil {
				return err
			}
			cmd.Println("✓ Inicializacao automatica desabilitada.")
			return nil
		}

		return fmt.Errorf("especifique --enable ou --disable")
	},
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

func init() {
	daemonStartCmd.Flags().BoolVar(&headlessFlag, "headless", false, "Inicia o daemon sem interface grafica de bandeja")
	daemonAutostartCmd.Flags().BoolVar(&enableAutostart, "enable", false, "Habilita a inicializacao automatica")
	daemonAutostartCmd.Flags().BoolVar(&disableAutostart, "disable", false, "Desabilita a inicializacao automatica")

	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonStatusCmd, daemonRestartCmd, daemonAutostartCmd)
	rootCmd.AddCommand(daemonCmd)
}
