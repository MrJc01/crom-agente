package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/orchestrator"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Daemon struct {
	manager      *orchestrator.MultiAgentManager
	ipc          *IPCServer
	apiServer    *APIServer
	grpcServer   *GRPCServer
	notifier     *DesktopNotifier
	headless     bool
	quit         chan struct{}
	logFile      *os.File
	trayQuit     chan struct{}
	wg           sync.WaitGroup
	APIPort      int
	GRPCPort     int
	sessionToken string
}

// NewDaemon cria um novo Daemon
func NewDaemon(headless bool) *Daemon {
	mgr := orchestrator.NewMultiAgentManager()
	ipcServer := NewIPCServer(mgr)
	apiServer := NewAPIServer(mgr, ipcServer.router)

	mgr.OnSchedule = func(workspaceName, sessionName, task string, delaySecs int, provider, model string) {
		apiServer.ScheduleTimerTask(workspaceName, sessionName, task, delaySecs, provider, model)
	}
	mgr.OnBackgroundExit = func(workspaceName, sessionName, task string, provider, model string) {
		apiServer.ScheduleTimerTask(workspaceName, sessionName, task, 0, provider, model)
	}

	return &Daemon{
		manager:    mgr,
		ipc:        ipcServer,
		apiServer:  apiServer,
		grpcServer: NewGRPCServer(mgr, ipcServer.router),
		notifier:   NewDesktopNotifier("crom-agente"),
		headless:   headless,
		quit:       make(chan struct{}),
		trayQuit:   make(chan struct{}),
		APIPort:    9090,
		GRPCPort:   9091,
	}
}

func (d *Daemon) setupLogging() error {
	dir, err := config.GlobalDir()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(dir, 0755)

	logPath := filepath.Join(dir, "daemon.log")

	// Logger rotativo usando lumberjack
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
	}

	// Multiplexa saída para stdout (terminal) e arquivo (lumberjack)
	multiWriter := io.MultiWriter(os.Stdout, lumberjackLogger)

	// Configura o handler do slog
	handler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Redireciona log padrão (log.Printf) para o multiWriter pra não perder compatibilidade com libs legadas
	log.SetOutput(multiWriter)
	
	// Salva referência do lumberjack para Close() manual se precisar (a interface do io.WriteCloser)
	d.logFile = nil // Não precisamos mais do *os.File cru

	return nil
}

// Start inicia o daemon de fundo
func (d *Daemon) Start() error {
	// 1. Verifica se ja esta rodando
	running, pid := isDaemonRunning()
	if running {
		return fmt.Errorf("daemon ja esta em execucao com PID %d", pid)
	}

	// 2. Configura logs
	if err := d.setupLogging(); err != nil {
		return err
	}

	// 2.5 Configura token de sessao
	if err := d.setupSessionToken(); err != nil {
		d.cleanup()
		return err
	}
	d.apiServer.SessionToken = d.sessionToken
	d.grpcServer.SessionToken = d.sessionToken

	log.Printf("[Daemon] Inicializando crom-agente daemon (PID: %d)", os.Getpid())

	// 3. Escreve arquivo de PID
	pidPath, err := PIDPath()
	if err != nil {
		return err
	}
	err = os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	if err != nil {
		return fmt.Errorf("falha ao gravar PID: %w", err)
	}

	// 4. Inicia Servidor IPC e API HTTP/WebSocket
	if err := d.ipc.Start(); err != nil {
		d.cleanup()
		return err
	}
	log.Printf("[Daemon] Servidor IPC iniciado no socket Unix")

	if err := d.apiServer.Start(d.APIPort); err != nil {
		d.cleanup()
		return err
	}

	if err := d.grpcServer.Start(d.GRPCPort); err != nil {
		d.cleanup()
		return err
	}

	// 4.5 Inicializa os servidores MCP configurados
	if err := d.manager.InitMCPFromConfig(context.Background()); err != nil {
		log.Printf("[Daemon] Erro ao inicializar servidores MCP globais: %v", err)
	}

	// 5. Configura tratamento de sinais do OS
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("[Daemon] Sinal recebido: %v. Encerrando...", sig)
		d.Stop()
	}()

	// 6. Configura auto-deteccao headless baseada em variaveis de ambiente de display no Linux
	if !d.headless {
		disp := os.Getenv("DISPLAY")
		waylandDisp := os.Getenv("WAYLAND_DISPLAY")
		if disp == "" && waylandDisp == "" {
			log.Println("[Daemon] Nenhuma sessao visual (X11/Wayland) detectada. Forcando headless.")
			d.headless = true
		}
	}

	// 7. Loop de atualizacao do System Tray (se nao for headless)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				running := d.manager.ListRunningAgents()
				names := make([]string, len(running))
				for i, r := range running {
					names[i] = fmt.Sprintf("%s (%s)", r.WorkspaceName, r.Task)
				}
				if !d.headless {
					GlobalTray.UpdateRunningAgents(names)
					if len(running) > 0 {
						GlobalTray.SetStatus("🔴 Executando")
					} else {
						GlobalTray.SetStatus("🟢 Ativo (Idle)")
					}
				}
			case <-d.quit:
				return
			}
		}
	}()

	// 8. Roda a bandeja (bloqueante)
	if !d.headless {
		log.Printf("[Daemon] Iniciando interface grafica de bandeja (System Tray)")
		GlobalTray.SetOnStopAgents(func() {
			running := d.manager.ListRunningAgents()
			for _, a := range running {
				log.Printf("[Daemon] Cancelando execucao ativa via tray no workspace '%s'", a.WorkspaceName)
				_ = d.manager.StopAgent(a.WorkspaceName)
			}
		})
		GlobalTray.SetOnOpenWorkspace(func() {
			running := d.manager.ListRunningAgents()
			var path string
			if len(running) > 0 {
				workspaces, _ := orchestrator.LoadWorkspaces()
				for _, w := range workspaces {
					if w.Name == running[0].WorkspaceName {
						path = w.Path
						break
					}
				}
			}
			if path == "" {
				workspaces, _ := orchestrator.LoadWorkspaces()
				if len(workspaces) > 0 {
					path = workspaces[0].Path
				}
			}
			if path != "" {
				_ = openPath(path)
			}
		})

		GlobalTray.Run(func() {
			_ = d.notifier.Notify("crom-agente Iniciado", "O daemon persistente do orquestrador esta pronto.")
		}, func() {
			// Callback ao fechar
		})
	} else {
		log.Printf("[Daemon] Rodando em modo HEADLESS (servidor de fundo)")
		// Mantem o daemon travado ate Stop() fechar o canal
		<-d.trayQuit
	}

	d.wg.Wait()
	log.Printf("[Daemon] Shutdown gracioso finalizado com sucesso.")
	return nil
}

// Stop encerra o daemon graciosamente
func (d *Daemon) Stop() {
	log.Printf("[Daemon] Iniciando desligamento gracioso...")

	// Cancela todos os agentes ativos e para os servidores MCP configurados
	d.manager.Shutdown()

	// Para o servidor IPC, API HTTP/WS e gRPC
	d.ipc.Stop()
	d.apiServer.Stop()
	d.grpcServer.Stop()

	// Sinaliza encerramento de background threads
	close(d.quit)

	// Finaliza bandeja ou headless lock
	if !d.headless {
		GlobalTray.Quit()
	} else {
		select {
		case <-d.trayQuit:
		default:
			close(d.trayQuit)
		}
	}

	d.cleanup()
}

func (d *Daemon) cleanup() {
	pidPath, _ := PIDPath()
	if pidPath != "" {
		_ = os.Remove(pidPath)
	}
	sockPath, _ := SocketPath()
	if sockPath != "" {
		_ = os.Remove(sockPath)
	}
	if dir, err := config.GlobalDir(); err == nil {
		_ = os.Remove(filepath.Join(dir, "session_token"))
	}
	if d.logFile != nil {
		_ = d.logFile.Close()
	}
}

func (d *Daemon) setupSessionToken() error {
	// Desabilita autenticação por padrão, a menos que CROM_ENABLE_AUTH seja explicitamente configurado como true
	if os.Getenv("CROM_ENABLE_AUTH") != "true" {
		log.Println("[Daemon] Autenticacao de sessao desabilitada por padrao (CROM_ENABLE_AUTH!=true)")
		d.sessionToken = ""
		return nil
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("falha ao gerar token de sessao: %w", err)
	}
	d.sessionToken = hex.EncodeToString(tokenBytes)

	dir, err := config.GlobalDir()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(dir, 0755)

	tokenPath := filepath.Join(dir, "session_token")
	if err := os.WriteFile(tokenPath, []byte(d.sessionToken), 0600); err != nil {
		return fmt.Errorf("falha ao salvar token de sessao: %w", err)
	}
	return nil
}

func isDaemonRunning() (bool, int) {
	pidPath, err := PIDPath()
	if err != nil {
		return false, 0
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return false, 0
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}

	// Envia sinal 0 para verificar se processo existe
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true, pid
	}
	return false, 0
}

func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
