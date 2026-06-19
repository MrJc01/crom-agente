package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ActiveProcess holds references to the currently running process under PTY
type ActiveProcess struct {
	Cmd      *exec.Cmd
	PTY      *os.File
	Cancel   context.CancelFunc
	DoneChan chan struct{}
}

// SafeBuffer é um buffer circular de bytes thread-safe
type SafeBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func NewSafeBuffer(max int) *SafeBuffer {
	return &SafeBuffer{
		buf: make([]byte, 0, max),
		max: max,
	}
}

func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if len(sb.buf)+len(p) > sb.max {
		toRemove := len(sb.buf) + len(p) - sb.max
		if toRemove >= len(sb.buf) {
			sb.buf = sb.buf[:0]
		} else {
			sb.buf = sb.buf[toRemove:]
		}
	}
	sb.buf = append(sb.buf, p...)
	return len(p), nil
}

func (sb *SafeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return string(sb.buf)
}

// BackgroundProcess representa um processo sendo executado em segundo plano
type BackgroundProcess struct {
	ID        string       `json:"id"`
	Command   string       `json:"command"`
	Cmd       *exec.Cmd    `json:"-"`
	PTY       *os.File     `json:"-"`
	StartedAt time.Time    `json:"started_at"`
	Logs      *SafeBuffer  `json:"-"`
}

var (
	activeProcMu      sync.Mutex
	activeProc        *ActiveProcess
	backgroundProcsMu sync.Mutex
	backgroundProcs   = make(map[string]*BackgroundProcess)
)

func generateBgID() string {
	return fmt.Sprintf("bg-%d", time.Now().UnixNano()%100000)
}

// TerminalCommandTool executa comandos de shell usando PTY com streaming e suporte a SIGINT
type TerminalCommandTool struct {
	workspaceRoot    string
	blockedCommands  []string
	stream           io.Writer
	onBackgroundExit func(bgID, cmdStr, logs string, success bool)
}

// NewTerminalCommandTool cria a ferramenta terminal_command
func NewTerminalCommandTool(workspaceRoot string, blocked []string, stream ...io.Writer) *TerminalCommandTool {
	var s io.Writer
	if len(stream) > 0 && stream[0] != nil {
		s = stream[0]
	} else {
		s = os.Stdout
	}
	return &TerminalCommandTool{
		workspaceRoot:   workspaceRoot,
		blockedCommands: blocked,
		stream:          s,
	}
}

// SetOnBackgroundExit define o callback chamado quando um processo em background finaliza
func (t *TerminalCommandTool) SetOnBackgroundExit(cb func(bgID, cmdStr, logs string, success bool)) {
	t.onBackgroundExit = cb
}

func (t *TerminalCommandTool) ID() string {
	return "terminal_command"
}

func (t *TerminalCommandTool) Description() string {
	return "Executa um comando shell no workspace utilizando um terminal PTY controlado. Suporta execução assíncrona/background de comandos, streaming de saída e ações de controle (list, kill, logs)."
}

func (t *TerminalCommandTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "O comando shell completo a ser executado (obrigatório para action 'run')"
			},
			"action": {
				"type": "string",
				"enum": ["run", "interrupt", "list", "kill", "logs"],
				"description": "Ação a executar: 'run' (executa comando), 'interrupt' (envia SIGINT ao comando foreground ativo), 'list' (lista processos em background), 'kill' (encerra processo em background), 'logs' (lê logs de um processo em background)",
				"default": "run"
			},
			"background": {
				"type": "boolean",
				"description": "Se verdadeiro (usando com 'run'), executa o comando em background sem bloquear o loop e retorna imediatamente"
			},
			"process_id": {
				"type": "string",
				"description": "O ID do processo em background (obrigatório para 'kill' e 'logs')"
			}
		},
		"required": []
	}`)
}

func (t *TerminalCommandTool) RequiresApproval() bool {
	return true
}

func (t *TerminalCommandTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Command    string `json:"command"`
		Action     string `json:"action"`
		Background bool   `json:"background"`
		ProcessID  string `json:"process_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos de JSON"}, nil
	}

	action := input.Action
	if action == "" {
		action = "run"
	}

	if action == "interrupt" {
		return t.interruptActiveProcess()
	}

	if action == "list" {
		backgroundProcsMu.Lock()
		defer backgroundProcsMu.Unlock()

		if len(backgroundProcs) == 0 {
			return Result{Success: true, Data: "Nenhum processo em background ativo."}, nil
		}

		var lines []string
		for id, p := range backgroundProcs {
			status := "Running"
			if p.Cmd.ProcessState != nil && p.Cmd.ProcessState.Exited() {
				status = "Exited"
			}
			lines = append(lines, fmt.Sprintf("- ID: %s | Status: %s | Comando: %q | Iniciado em: %s", id, status, p.Command, p.StartedAt.Format("15:04:05")))
		}
		return Result{Success: true, Data: strings.Join(lines, "\n")}, nil
	}

	if action == "kill" {
		procID := strings.TrimSpace(input.ProcessID)
		if procID == "" {
			return Result{Success: false, Error: "o parâmetro 'process_id' é obrigatório para a ação 'kill'"}, nil
		}

		backgroundProcsMu.Lock()
		p, exists := backgroundProcs[procID]
		backgroundProcsMu.Unlock()

		if !exists {
			return Result{Success: false, Error: fmt.Sprintf("processo com ID '%s' não encontrado", procID)}, nil
		}

		t.killProcessGroup(p.Cmd)
		_ = p.PTY.Close()

		backgroundProcsMu.Lock()
		delete(backgroundProcs, procID)
		backgroundProcsMu.Unlock()

		return Result{Success: true, Data: fmt.Sprintf("Processo %s encerrado com sucesso.", procID)}, nil
	}

	if action == "logs" {
		procID := strings.TrimSpace(input.ProcessID)
		if procID == "" {
			return Result{Success: false, Error: "o parâmetro 'process_id' é obrigatório para a ação 'logs'"}, nil
		}

		backgroundProcsMu.Lock()
		p, exists := backgroundProcs[procID]
		backgroundProcsMu.Unlock()

		if !exists {
			return Result{Success: false, Error: fmt.Sprintf("processo com ID '%s' não encontrado ou já finalizado", procID)}, nil
		}

		output := p.Logs.String()
		if output == "" {
			output = "(sem saída de terminal até o momento)"
		}

		return Result{Success: true, Data: fmt.Sprintf("Logs do processo %s:\n%s", procID, output)}, nil
	}

	command := strings.TrimSpace(input.Command)
	if command == "" {
		return Result{Success: false, Error: "o comando não pode ser vazio"}, nil
	}

	// Validação de segurança contra comandos bloqueados
	for _, blocked := range t.blockedCommands {
		if strings.Contains(command, blocked) {
			return Result{Success: false, Error: fmt.Sprintf("comando bloqueado pelas políticas de segurança: contém '%s'", blocked)}, nil
		}
	}

	if input.Background {
		bgCtx := context.Background()
		c := exec.CommandContext(bgCtx, "bash", "-c", command)
		c.Dir = t.workspaceRoot

		f, err := pty.Start(c)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("erro ao iniciar pseudo-terminal em background: %s", err.Error())}, nil
		}

		bgID := generateBgID()
		logsBuf := NewSafeBuffer(32768) // 32KB buffer

		p := &BackgroundProcess{
			ID:        bgID,
			Command:   command,
			Cmd:       c,
			PTY:       f,
			StartedAt: time.Now(),
			Logs:      logsBuf,
		}

		backgroundProcsMu.Lock()
		backgroundProcs[bgID] = p
		backgroundProcsMu.Unlock()

		// Goroutine para ler a saída em background
		go func() {
			defer func() {
				backgroundProcsMu.Lock()
				delete(backgroundProcs, bgID)
				backgroundProcsMu.Unlock()
				_ = f.Close()
				_ = c.Wait()
				
				success := false
				if c.ProcessState != nil {
					success = c.ProcessState.Success()
				}
				if t.onBackgroundExit != nil {
					t.onBackgroundExit(bgID, command, logsBuf.String(), success)
				}
			}()

			buffer := make([]byte, 2048)
			for {
				_ = f.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, readErr := f.Read(buffer)
				if n > 0 {
					chunk := buffer[:n]
					_, _ = logsBuf.Write(chunk)
					if t.stream != nil {
						_, _ = t.stream.Write(chunk)
					}
				}
				if readErr != nil {
					if readErr == io.EOF || strings.Contains(readErr.Error(), "input/output error") {
						return
					}
					if strings.Contains(readErr.Error(), "i/o timeout") {
						if c.ProcessState != nil && c.ProcessState.Exited() {
							return
						}
						continue
					}
					return
				}
			}
		}()

		// Aguarda um curtíssimo tempo para ver se o comando falha imediatamente
		time.Sleep(100 * time.Millisecond)
		backgroundProcsMu.Lock()
		_, exists := backgroundProcs[bgID]
		backgroundProcsMu.Unlock()

		if !exists {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("O comando em background terminou imediatamente. Logs:\n%s", logsBuf.String()),
			}, nil
		}

		return Result{
			Success: true,
			Data:    fmt.Sprintf("Processo iniciado em background com sucesso. ID: %s. Use 'list' para monitorar ou 'logs' para ver a saída.", bgID),
		}, nil
	}

	// Evitar executar múltiplos comandos paralelos no mesmo terminal_command
	activeProcMu.Lock()
	if activeProc != nil {
		activeProcMu.Unlock()
		return Result{Success: false, Error: "já existe um comando em execução no PTY. Envie a ação 'interrupt' primeiro se quiser cancelá-lo."}, nil
	}
	activeProcMu.Unlock()

	// Executa em bash -c (modo foreground)
	c := exec.CommandContext(ctx, "bash", "-c", command)
	c.Dir = t.workspaceRoot

	f, err := pty.Start(c)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao iniciar pseudo-terminal: %s", err.Error())}, nil
	}

	doneChan := make(chan struct{})
	procCtx, procCancel := context.WithCancel(ctx)

	p := &ActiveProcess{
		Cmd:      c,
		PTY:      f,
		Cancel:   procCancel,
		DoneChan: doneChan,
	}

	activeProcMu.Lock()
	activeProc = p
	activeProcMu.Unlock()

	defer func() {
		activeProcMu.Lock()
		activeProc = nil
		activeProcMu.Unlock()
		f.Close()
		procCancel()
	}()

	outChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Goroutine de leitura não-bloqueante/streaming
	go func() {
		var outputBuilder strings.Builder
		buffer := make([]byte, 2048)
		for {
			select {
			case <-procCtx.Done():
				return
			default:
				_ = f.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, readErr := f.Read(buffer)
				if n > 0 {
					chunk := buffer[:n]
					outputBuilder.Write(chunk)
					if t.stream != nil {
						_, _ = t.stream.Write(chunk)
					}
				}
				if readErr != nil {
					if readErr == io.EOF || strings.Contains(readErr.Error(), "input/output error") || strings.Contains(readErr.Error(), "i/o timeout") {
						if readErr != io.EOF && strings.Contains(readErr.Error(), "i/o timeout") {
							if c.ProcessState != nil && c.ProcessState.Exited() {
								outChan <- outputBuilder.String()
								return
							}
							continue
						}
						outChan <- outputBuilder.String()
						return
					}
					errChan <- readErr
					return
				}
			}
		}
	}()

	// Aguarda processo finalizar ou contexto expirar
	select {
	case <-ctx.Done():
		t.killProcessGroup(c)
		return Result{Success: false, Error: "comando cancelado por timeout ou contexto"}, ctx.Err()
	case <-procCtx.Done():
		t.killProcessGroup(c)
		return Result{Success: false, Error: "comando interrompido via sinal SIGINT"}, nil
	case err = <-errChan:
		return Result{Success: false, Error: fmt.Sprintf("erro na leitura do terminal: %v", err)}, nil
	case out := <-outChan:
		_ = c.Wait()
		close(doneChan)
		return Result{
			Success: c.ProcessState.Success(),
			Data:    out,
		}, nil
	}
}

// interruptActiveProcess envia syscall.SIGINT (Ctrl+C) ao processo ativo
func (t *TerminalCommandTool) interruptActiveProcess() (Result, error) {
	activeProcMu.Lock()
	p := activeProc
	activeProcMu.Unlock()

	if p == nil {
		return Result{Success: false, Error: "nenhum processo ativo no terminal para interromper"}, nil
	}

	_ = p.Cmd.Process.Signal(syscall.SIGINT)
	p.Cancel()

	return Result{
		Success: true,
		Data:    "Sinal de interrupção (SIGINT/Ctrl+C) enviado com sucesso para o processo do terminal.",
	}, nil
}

// killProcessGroup envia SIGKILL para toda a árvore do processo
func (t *TerminalCommandTool) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		pid := cmd.Process.Pid
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}
}
