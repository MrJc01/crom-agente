package port_monitor_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/tools/port_monitor"
)

func TestPortMonitorTool(t *testing.T) {
	ws := t.TempDir()
	tool := port_monitor.NewPortMonitorTool(ws)

	// 1. Iniciar um TCP listener local em porta dinâmica do S.O.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao iniciar listener para teste: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// 2. Testar porta ativa
	args := json.RawMessage(fmt.Sprintf(`{"port": %d, "timeout_ms": 500}`, port))
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("esperava sucesso ao escanear porta ativa: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "aberta") {
		t.Fatalf("mensagem inesperada para porta aberta: %s", res.Data)
	}

	// 3. Fechar listener e testar porta fechada
	listener.Close()
	// Aguardar breve liberação do S.O.
	time.Sleep(50 * time.Millisecond)

	res, err = tool.Execute(context.Background(), args)
	if err != nil || res.Success {
		t.Fatalf("esperava falha ao escanear porta inativa: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Error, "fechada") {
		t.Fatalf("mensagem inesperada para porta fechada: %s", res.Error)
	}
}
