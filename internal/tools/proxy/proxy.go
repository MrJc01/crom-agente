package proxy

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de proxy: " + err.Error())
	}
}

// ActiveProxy representa um proxy TCP ativo rodando em background
type ActiveProxy struct {
	ID          string             `json:"id"`
	LocalAddr   string             `json:"local_addr"`
	TargetAddr  string             `json:"target_addr"`
	LogPath     string             `json:"log_path"`
	Listener    net.Listener       `json:"-"`
	Cancel      context.CancelFunc `json:"-"`
	Connections int                `json:"connections"`
	mu          sync.Mutex
}

var (
	proxiesMu     sync.RWMutex
	activeProxies = make(map[string]*ActiveProxy)
)

// ProxyTool gerencia proxies TCP temporários para interceptação de tráfego
type ProxyTool struct {
	workspaceRoot string
	jail          bool
}

// NewProxyTool cria a ferramenta proxy
func NewProxyTool(workspaceRoot string, jail bool) *ProxyTool {
	return &ProxyTool{workspaceRoot: workspaceRoot, jail: jail}
}

func (t *ProxyTool) ID() string { return metadata.ID }

func (t *ProxyTool) Description() string {
	return metadata.Description
}

func (t *ProxyTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["start", "stop", "list"],
				"description": "Ação a executar"
			},
			"proxy_id": {
				"type": "string",
				"description": "Identificador único do proxy (obrigatório para 'stop')"
			},
			"local_port": {
				"type": "integer",
				"description": "Porta local para escutar (ex: 8081). Se 0, escolhe porta dinâmica.",
				"default": 0
			},
			"target_addr": {
				"type": "string",
				"description": "Endereço de destino no formato host:porta (ex: localhost:8080 ou 93.184.216.34:80)"
			},
			"log_file": {
				"type": "string",
				"description": "Caminho do arquivo para salvar os logs de tráfego no workspace (default: .crom/proxy_<id>.log)"
			}
		},
		"required": ["action"]
	}`)
}

// RequiresApproval — Monitoramento de tráfego é uma ação crítica de privacidade/segurança, exige aprovação HITL
func (t *ProxyTool) RequiresApproval() bool { return true }

func (t *ProxyTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Action     string `json:"action"`
		ProxyID    string `json:"proxy_id"`
		LocalPort  int    `json:"local_port"`
		TargetAddr string `json:"target_addr"`
		LogFile    string `json:"log_file"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	switch input.Action {
	case "start":
		if input.TargetAddr == "" {
			return tools.Result{Success: false, Error: "target_addr é obrigatório para iniciar o proxy"}, nil
		}
		return t.startProxy(ctx, input.LocalPort, input.TargetAddr, input.LogFile)
	case "stop":
		if input.ProxyID == "" {
			return tools.Result{Success: false, Error: "proxy_id é obrigatório para parar o proxy"}, nil
		}
		return t.stopProxy(input.ProxyID)
	case "list":
		return t.listProxies()
	default:
		return tools.Result{Success: false, Error: fmt.Sprintf("ação desconhecida: %q", input.Action)}, nil
	}
}

func (t *ProxyTool) startProxy(ctx context.Context, localPort int, targetAddr string, logFile string) (tools.Result, error) {
	// Limitar formato de target_addr
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return tools.Result{Success: false, Error: "target_addr inválido, deve estar no formato host:porta: " + err.Error()}, nil
	}
	_ = host
	_ = portStr

	// Definir arquivo de log seguro dentro do workspace
	var logPath string
	if logFile != "" {
		var err error
		logPath, err = tools.ValidatePath(t.workspaceRoot, logFile, t.jail)
		if err != nil {
			return tools.Result{Success: false, Error: "log_file: " + err.Error()}, nil
		}
	} else {
		// Log padrão em .crom/
		cromDir := filepath.Join(t.workspaceRoot, ".crom")
		_ = os.MkdirAll(cromDir, 0755)
		logPath = filepath.Join(cromDir, fmt.Sprintf("proxy_%d.log", time.Now().UnixNano()))
	}

	// Criar listener local
	localAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return tools.Result{Success: false, Error: "falha ao iniciar listener local: " + err.Error()}, nil
	}

	resolvedLocalAddr := listener.Addr().String()
	proxyID := fmt.Sprintf("proxy-%d", time.Now().UnixNano()%10000)

	proxyCtx, cancel := context.WithCancel(context.Background())

	p := &ActiveProxy{
		ID:         proxyID,
		LocalAddr:  resolvedLocalAddr,
		TargetAddr: targetAddr,
		LogPath:    logPath,
		Listener:   listener,
		Cancel:     cancel,
	}

	proxiesMu.Lock()
	activeProxies[proxyID] = p
	proxiesMu.Unlock()

	// Inicia rotina de accept
	go func() {
		defer func() {
			listener.Close()
			proxiesMu.Lock()
			delete(activeProxies, proxyID)
			proxiesMu.Unlock()
		}()

		// Abrir arquivo de log
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		defer f.Close()

		_, _ = f.WriteString(fmt.Sprintf("[%s] Proxy iniciado: %s -> %s\n", time.Now().Format(time.RFC3339), resolvedLocalAddr, targetAddr))

		for {
			conn, err := listener.Accept()
			if err != nil {
				// Listener fechado ou cancelado
				return
			}

			p.mu.Lock()
			p.Connections++
			connID := p.Connections
			p.mu.Unlock()

			go t.handleConnection(proxyCtx, conn, targetAddr, f, connID)
		}
	}()

	data, _ := json.MarshalIndent(p, "", "  ")
	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Proxy iniciado com sucesso!\n%s", string(data)),
	}, nil
}

func (t *ProxyTool) stopProxy(id string) (tools.Result, error) {
	proxiesMu.Lock()
	p, ok := activeProxies[id]
	proxiesMu.Unlock()

	if !ok {
		return tools.Result{Success: false, Error: fmt.Sprintf("proxy com ID '%s' não encontrado", id)}, nil
	}

	p.Listener.Close()
	p.Cancel()

	return tools.Result{Success: true, Data: fmt.Sprintf("Proxy '%s' parado com sucesso. Logs gravados em: %s", id, p.LogPath)}, nil
}

func (t *ProxyTool) listProxies() (tools.Result, error) {
	proxiesMu.RLock()
	defer proxiesMu.RUnlock()

	list := make([]*ActiveProxy, 0, len(activeProxies))
	for _, p := range activeProxies {
		list = append(list, p)
	}

	data, _ := json.MarshalIndent(list, "", "  ")
	return tools.Result{Success: true, Data: string(data)}, nil
}

func (t *ProxyTool) handleConnection(ctx context.Context, localConn net.Conn, targetAddr string, logWriter io.Writer, connID int) {
	defer localConn.Close()

	// Dial para o destino com timeout de 5 segundos
	var dialer net.Dialer
	remoteConn, err := dialer.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		_, _ = fmt.Fprintf(logWriter, "[%s] [Conn #%d] Erro ao conectar ao destino %s: %v\n", time.Now().Format(time.RFC3339), connID, targetAddr, err)
		return
	}
	defer remoteConn.Close()

	_, _ = fmt.Fprintf(logWriter, "[%s] [Conn #%d] Conexão estabelecida: %s\n", time.Now().Format(time.RFC3339), connID, localConn.RemoteAddr())

	var wg sync.WaitGroup
	wg.Add(2)

	// Local -> Remote
	go func() {
		defer wg.Done()
		t.pipeData(localConn, remoteConn, logWriter, connID, ">>> Local -> Remote")
	}()

	// Remote -> Local
	go func() {
		defer wg.Done()
		t.pipeData(remoteConn, localConn, logWriter, connID, "<<< Remote -> Local")
	}()

	// Aguarda fim da transferência ou cancelamento do contexto
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}

	_, _ = fmt.Fprintf(logWriter, "[%s] [Conn #%d] Conexão encerrada.\n", time.Now().Format(time.RFC3339), connID)
}

func (t *ProxyTool) pipeData(src net.Conn, dst net.Conn, logWriter io.Writer, connID int, direction string) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			// Escrever dados no destino
			_, wErr := dst.Write(buf[:n])
			if wErr != nil {
				return
			}

			// Escrever payload no log (limitar representação visual no arquivo para 500 bytes por leitura)
			snippet := buf[:n]
			truncated := ""
			if len(snippet) > 500 {
				snippet = snippet[:500]
				truncated = "... [truncado]"
			}

			// Se parece texto imprimível, escreve como string; senão, como hex dump
			isText := true
			for _, b := range snippet {
				if (b < 32 || b > 126) && b != '\n' && b != '\r' && b != '\t' {
					isText = false
					break
				}
			}

			if isText {
				_, _ = fmt.Fprintf(logWriter, "[%s] [Conn #%d] %s (%d bytes):\n%s%s\n", time.Now().Format(time.RFC3339), connID, direction, n, string(snippet), truncated)
			} else {
				_, _ = fmt.Fprintf(logWriter, "[%s] [Conn #%d] %s (binário - %d bytes):\n%x%s\n", time.Now().Format(time.RFC3339), connID, direction, n, snippet, truncated)
			}
		}

		if err != nil {
			return
		}
	}
}
