package terminal_command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

func TestTerminalCommandTool(t *testing.T) {
	ws := t.TempDir()

	tool := NewTerminalCommandTool(ws, []string{"sudo", "rm -rf"})

	// 1. Executa comando válido
	args := json.RawMessage(`{"command": "echo 'hello terminal'"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar comando: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "hello terminal") {
		t.Fatalf("saída incorreta: %s", res.Data)
	}

	// 2. Executa comando bloqueado
	argsBad := json.RawMessage(`{"command": "sudo apt update"}`)
	res, _ = tool.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava bloqueio do comando sudo")
	}
	if !strings.Contains(res.Error, "comando bloqueado") {
		t.Fatalf("mensagem de bloqueio inválida: %s", res.Error)
	}
}

func TestTerminalCommandTool_StreamingAndInterrupt(t *testing.T) {
	ws := t.TempDir()

	var buf strings.Builder
	tool := NewTerminalCommandTool(ws, nil, &buf)

	// 1. Testar streaming de um comando longo (sleep 1 com echo)
	args := json.RawMessage(`{"command": "echo 'start' && sleep 0.2 && echo 'end'"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar comando com sleep: %v, res: %+v", err, res)
	}

	streamed := buf.String()
	if !strings.Contains(streamed, "start") || !strings.Contains(streamed, "end") {
		t.Fatalf("streaming falhou, dados gravados no buffer: %q", streamed)
	}

	// 2. Testar interrupção (Ctrl+C)
	longArgs := json.RawMessage(`{"command": "sleep 10"}`)

	errChan := make(chan error, 1)
	resChan := make(chan tools.Result, 1)

	go func() {
		r, e := tool.Execute(context.Background(), longArgs)
		resChan <- r
		errChan <- e
	}()

	// Aguarda um instante para o processo iniciar
	time.Sleep(100 * time.Millisecond)

	// Envia sinal de interrupção
	interruptArgs := json.RawMessage(`{"action": "interrupt"}`)
	intRes, err := tool.Execute(context.Background(), interruptArgs)
	if err != nil || !intRes.Success {
		t.Fatalf("falha ao enviar interrupt: %v, res: %+v", err, intRes)
	}

	// Aguarda resultado do comando interrompido
	r := <-resChan
	e := <-errChan

	if e != nil {
		t.Fatalf("erro inesperado na interrupção: %v", e)
	}
	if r.Success {
		t.Fatal("esperava que o comando interrompido retornasse Success = false")
	}
	if !strings.Contains(r.Error, "interrompido") {
		t.Fatalf("mensagem de erro inesperada para interrupção: %s", r.Error)
	}
}

func TestTerminalCommandTool_BackgroundActions(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, nil)

	// 1. Inicia um comando sleep curto em background
	argsRun := json.RawMessage(`{"command": "sleep 2", "background": true}`)
	resRun, err := tool.Execute(context.Background(), argsRun)
	if err != nil || !resRun.Success {
		t.Fatalf("erro ao rodar comando em background: %v, res: %+v", err, resRun)
	}

	// Extrai ID do processo a partir do retorno
	var bgID string
	_, _ = fmt.Sscanf(resRun.Data, "Processo iniciado em background com sucesso. ID: %s", &bgID)
	bgID = strings.TrimSuffix(bgID, ".")
	if !strings.HasPrefix(bgID, "bg-") {
		t.Fatalf("ID do processo inválido: %s", bgID)
	}

	// 2. Lista os processos em background
	argsList := json.RawMessage(`{"action": "list"}`)
	resList, err := tool.Execute(context.Background(), argsList)
	if err != nil || !resList.Success {
		t.Fatalf("erro ao listar processos: %v", err)
	}
	if !strings.Contains(resList.Data, bgID) {
		t.Fatalf("esperava encontrar processo %s na listagem: %s", bgID, resList.Data)
	}

	// 3. Lê logs (deve funcionar, mesmo que esteja vazio)
	argsLogs := json.RawMessage(fmt.Sprintf(`{"action": "logs", "process_id": "%s"}`, bgID))
	resLogs, err := tool.Execute(context.Background(), argsLogs)
	if err != nil || !resLogs.Success {
		t.Fatalf("erro ao ler logs: %v", err)
	}

	// 4. Encerra (kill) o processo
	argsKill := json.RawMessage(fmt.Sprintf(`{"action": "kill", "process_id": "%s"}`, bgID))
	resKill, err := tool.Execute(context.Background(), argsKill)
	if err != nil || !resKill.Success {
		t.Fatalf("erro ao encerrar processo: %v", err)
	}

	// 5. Verifica se foi removido da listagem
	resList2, _ := tool.Execute(context.Background(), argsList)
	if strings.Contains(resList2.Data, bgID) {
		t.Fatalf("esperava que o processo %s tivesse sido removido: %s", bgID, resList2.Data)
	}
}

func TestTerminalCommandTool_BackgroundExitCallback(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, nil)

	callbackFired := make(chan bool, 1)
	var firedID, firedCmd string
	var firedSuccess bool

	tool.SetOnBackgroundExit(func(bgID, cmdStr, logs string, success bool) {
		firedID = bgID
		firedCmd = cmdStr
		firedSuccess = success
		callbackFired <- true
	})

	// Run a short background command (sleep 0.1)
	argsRun := json.RawMessage(`{"command": "sleep 0.1", "background": true}`)
	resRun, err := tool.Execute(context.Background(), argsRun)
	if err != nil || !resRun.Success {
		t.Fatalf("erro ao rodar comando em background: %v, res: %+v", err, resRun)
	}

	// Wait for the callback to fire
	select {
	case <-callbackFired:
		if !strings.HasPrefix(firedID, "bg-") {
			t.Errorf("ID do processo no callback inválido: %s", firedID)
		}
		if !strings.Contains(firedCmd, "sleep 0.1") {
			t.Errorf("comando incorreto no callback: %s", firedCmd)
		}
		if !firedSuccess {
			t.Error("esperava sucesso=true no callback para 'sleep 0.1'")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout aguardando disparo do callback de finalização do processo background")
	}
}

func TestTerminalCommandTool_AutoBackgroundOnTimeout(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, nil)

	// Inicia um comando de sleep longo que bloquearia em foreground
	argsRun := json.RawMessage(`{"command": "sleep 10"}`)

	start := time.Now()
	res, err := tool.Execute(context.Background(), argsRun)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !res.Success {
		t.Fatalf("esperava sucesso ao transicionar para background: %+v", res)
	}
	if duration < 3500*time.Millisecond || duration > 6*time.Second {
		t.Errorf("duração inesperada da execução: %v (deveria ser cerca de 4 segundos)", duration)
	}
	if !strings.Contains(res.Data, "continua rodando em segundo plano") {
		t.Errorf("mensagem de retorno inesperada: %s", res.Data)
	}

	// Extrai ID do processo a partir do retorno
	var bgID string
	idx := strings.Index(res.Data, "(ID: ")
	if idx != -1 {
		parts := strings.Split(res.Data[idx+5:], ")")
		if len(parts) > 0 {
			bgID = parts[0]
		}
	}

	if !strings.HasPrefix(bgID, "bg-") {
		t.Fatalf("ID do processo em background inválido extraído: %q", bgID)
	}

	// Encerra o processo para não deixar rodando em background no teste
	argsKill := json.RawMessage(fmt.Sprintf(`{"action": "kill", "process_id": "%s"}`, bgID))
	resKill, err := tool.Execute(context.Background(), argsKill)
	if err != nil || !resKill.Success {
		t.Fatalf("falha ao matar processo background no teste: %v, %+v", err, resKill)
	}
}

func TestPTY_TerminalCommandRun(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, []string{"rm -rf /"})

	ctx := context.Background()
	args, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'hello from pty'",
		"action":  "run",
	})

	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("Esperado sucesso, got erro: %s", res.Error)
	}

	if !strings.Contains(res.Data, "hello from pty") {
		t.Errorf("Saída inesperada: %s", res.Data)
	}
}

func TestPTY_BlockedCommand(t *testing.T) {
	ws := t.TempDir()
	tool := NewTerminalCommandTool(ws, []string{"sudo"})

	ctx := context.Background()
	args, _ := json.Marshal(map[string]interface{}{
		"command": "sudo cat /etc/shadow",
		"action":  "run",
	})

	res, _ := tool.Execute(ctx, args)
	if res.Success {
		t.Error("Comando bloqueado deveria ter falhado")
	}

	if !strings.Contains(res.Error, "bloqueado") {
		t.Errorf("Mensagem de erro incorreta: %s", res.Error)
	}
}
