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

var (
	activeProcMu sync.Mutex
	activeProc   *ActiveProcess
)

// TerminalCommandTool executa comandos de shell usando PTY com streaming e suporte a SIGINT
type TerminalCommandTool struct {
	workspaceRoot   string
	blockedCommands []string
	stream          io.Writer
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

func (t *TerminalCommandTool) ID() string {
	return "terminal_command"
}

func (t *TerminalCommandTool) Description() string {
	return "Executa um comando shell no workspace utilizando um terminal PTY controlado. Suporta execução assíncrona de comandos, streaming de saída e sinal de interrupção (Ctrl+C)."
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
				"enum": ["run", "interrupt"],
				"description": "Ação a executar: 'run' (executa novo comando), 'interrupt' (envia SIGINT/Ctrl+C ao comando ativo)",
				"default": "run"
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
		Command string `json:"command"`
		Action  string `json:"action"`
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

	// Evitar executar múltiplos comandos paralelos no mesmo terminal_command
	activeProcMu.Lock()
	if activeProc != nil {
		activeProcMu.Unlock()
		return Result{Success: false, Error: "já existe um comando em execução no PTY. Envie a ação 'interrupt' primeiro se quiser cancelá-lo."}, nil
	}
	activeProcMu.Unlock()

	// Executa em bash -c
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
				// Usamos SetReadDeadline para não travar a leitura indefinidamente
				_ = f.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				n, readErr := f.Read(buffer)
				if n > 0 {
					chunk := buffer[:n]
					outputBuilder.Write(chunk)
					// Stream em tempo real
					if t.stream != nil {
						_, _ = t.stream.Write(chunk)
					}
				}
				if readErr != nil {
					// Fim da execução ou erro esperado quando PTY fecha
					if readErr == io.EOF || strings.Contains(readErr.Error(), "input/output error") || strings.Contains(readErr.Error(), "i/o timeout") {
						// Se for apenas timeout, continua lendo; se for EOF/IO error e processo morreu, encerra.
						if readErr != io.EOF && strings.Contains(readErr.Error(), "i/o timeout") {
							// Verifica se o processo ainda está ativo
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

	// Envia SIGINT para o processo
	_ = p.Cmd.Process.Signal(syscall.SIGINT)

	// Cancela contexto para desenhar encerramento imediato
	p.Cancel()

	return Result{
		Success: true,
		Data:    "Sinal de interrupção (SIGINT/Ctrl+C) enviado com sucesso para o processo do terminal.",
	}, nil
}

// killProcessGroup envia SIGKILL para toda a árvore do processo
func (t *TerminalCommandTool) killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}
}
