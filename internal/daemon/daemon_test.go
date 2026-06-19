package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/daemon/pb"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestDaemon_LifecycleAndIPC(t *testing.T) {
	// Cria diretorio global temporario e redireciona env HOME
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Inicializa diretorio global do crom
	gDir, err := config.GlobalDir()
	if err != nil {
		t.Fatalf("falha ao obter diretorio global: %v", err)
	}
	_ = os.MkdirAll(gDir, 0755)

	// Grava configuracao global de mock
	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)

	// Salva env vazio
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	// Cria e configura o daemon
	d := NewDaemon(true) // modo headless para teste
	d.APIPort = 19090
	d.GRPCPort = 19095

	// Cria um canal de erro para monitorar o Start
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	// Aguarda o daemon subir (tentando abrir conexao com socket)
	sockPath, err := SocketPath()
	if err != nil {
		t.Fatalf("falha ao obter socket path: %v", err)
	}

	var conn net.Conn
	var connErr error
	for i := 0; i < 20; i++ {
		conn, connErr = net.Dial("unix", sockPath)
		if connErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if connErr != nil {
		t.Fatalf("falha ao conectar ao daemon: %v", connErr)
	}
	defer conn.Close()

	// 1. Testa Ping
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	pingMsg := IPCMessage{Type: "ping"}
	if err := enc.Encode(pingMsg); err != nil {
		t.Fatalf("erro ao enviar ping: %v", err)
	}

	var pingResp IPCResponse
	if err := dec.Decode(&pingResp); err != nil {
		t.Fatalf("erro ao decodificar ping: %v", err)
	}
	if !pingResp.Success {
		t.Fatalf("ping respondeu com falha: %s", pingResp.Error)
	}
	var pong string
	_ = json.Unmarshal(pingResp.Data, &pong)
	if pong != "pong" {
		t.Fatalf("esperava 'pong', obteve %q", pong)
	}

	// 2. Testa Status
	statusMsg := IPCMessage{Type: "status"}
	if err := enc.Encode(statusMsg); err != nil {
		t.Fatalf("erro ao enviar status: %v", err)
	}

	var statusResp IPCResponse
	if err := dec.Decode(&statusResp); err != nil {
		t.Fatalf("erro ao decodificar status: %v", err)
	}
	if !statusResp.Success {
		t.Fatalf("status respondeu com falha: %s", statusResp.Error)
	}

	// 3. Testa PID existente
	pidPath, _ := PIDPath()
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("arquivo de PID nao foi criado pelo daemon")
	}

	// Para o daemon
	d.Stop()

	// Aguarda o processo Start retornar
	select {
	case err := <-startErrChan:
		if err != nil {
			t.Fatalf("daemon retornou erro no Start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando parada do daemon")
	}

	// Verifica se os arquivos temporarios foram removidos
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("arquivo socket ainda existe apos Stop")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("arquivo PID ainda existe apos Stop")
	}
}

func TestDaemon_StalePIDCleanup(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	pidPath, _ := PIDPath()
	// Escreve um PID invalido/inexistente (ex: 999999 que nao esta rodando)
	_ = os.WriteFile(pidPath, []byte("999999\n"), 0644)

	d := NewDaemon(true)
	d.APIPort = 19091
	d.GRPCPort = 19096
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	// Verifica se o daemon limpou o PID antigo e subiu com sucesso
	sockPath, _ := SocketPath()
	var connErr error
	for i := 0; i < 20; i++ {
		_, connErr = net.Dial("unix", sockPath)
		if connErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if connErr != nil {
		t.Fatalf("falha ao conectar com daemon apos limpar stale PID: %v", connErr)
	}

	d.Stop()
	<-startErrChan
}

func TestDaemon_Autostart(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Habilita autostart
	err := ConfigureAutostart(true)
	if err != nil {
		t.Fatalf("falha ao habilitar autostart: %v", err)
	}

	autostartFile := filepath.Join(tempHome, ".config", "autostart", "crom-agente.desktop")
	if _, err := os.Stat(autostartFile); err != nil {
		t.Fatalf("arquivo .desktop de autostart nao foi criado")
	}

	// Desabilita autostart
	err = ConfigureAutostart(false)
	if err != nil {
		t.Fatalf("falha ao desabilitar autostart: %v", err)
	}

	if _, err := os.Stat(autostartFile); !os.IsNotExist(err) {
		t.Fatalf("arquivo .desktop de autostart nao foi removido")
	}
}

func TestAPIServer_HealthAndStatus(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CROM_ENABLE_AUTH", "true")

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	d := NewDaemon(true)
	d.APIPort = 19092
	d.GRPCPort = 19097
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	// Aguarda o servidor HTTP subir
	apiURL := "http://127.0.0.1:19092"
	var resp *http.Response
	var err error
	for i := 0; i < 30; i++ {
		resp, err = http.Get(apiURL + "/health")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("falha ao conectar na API HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("esperava status 200 OK, obteve: %d", resp.StatusCode)
	}

	// Le o token gerado com retries para evitar race condition
	var token string
	for i := 0; i < 30; i++ {
		tokenBytes, err := os.ReadFile(filepath.Join(gDir, "session_token"))
		if err == nil {
			token = string(tokenBytes)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if token == "" {
		t.Fatalf("falha ao ler token de sessao")
	}

	// Testa /status
	statusResp, err := http.Get(apiURL + "/status?token=" + token)
	if err != nil {
		t.Fatalf("erro ao consultar /status: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("esperava status 200 no /status, obteve: %d", statusResp.StatusCode)
	}

	d.Stop()
	<-startErrChan
}

func TestAPIServer_WebSocket(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CROM_ENABLE_AUTH", "true")

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	d := NewDaemon(true)
	d.APIPort = 19093
	d.GRPCPort = 19098
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	var err error

	// Le o token gerado com retries
	var token string
	for i := 0; i < 30; i++ {
		tokenBytes, err := os.ReadFile(filepath.Join(gDir, "session_token"))
		if err == nil {
			token = string(tokenBytes)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if token == "" {
		t.Fatalf("falha ao ler token de sessao")
	}

	// Aguarda servidor
	wsURL := "ws://127.0.0.1:19093/ws?token=" + token
	var wsConn *websocket.Conn
	for i := 0; i < 30; i++ {
		wsConn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("falha ao conectar no WebSocket: %v", err)
	}
	defer wsConn.Close()

	// Envia mensagem de subscribe
	subMsg := IPCMessage{Type: "subscribe", Workspace: "meu-ws"}
	if err := wsConn.WriteJSON(subMsg); err != nil {
		t.Fatalf("falha ao enviar subscribe no WS: %v", err)
	}

	d.Stop()
	<-startErrChan
}

func TestAPIServer_TokenAuthentication(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CROM_ENABLE_AUTH", "true")

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	d := NewDaemon(true)
	d.APIPort = 19100
	d.GRPCPort = 19101
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	apiURL := "http://127.0.0.1:19100"
	var err error
	for i := 0; i < 30; i++ {
		var resp *http.Response
		resp, err = http.Get(apiURL + "/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("falha ao conectar na API: %v", err)
	}

	tokenBytes, err := os.ReadFile(filepath.Join(gDir, "session_token"))
	if err != nil {
		t.Fatalf("falha ao ler token de sessao: %v", err)
	}
	token := string(tokenBytes)

	// Sem token
	resp, err := http.Get(apiURL + "/status")
	if err == nil {
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("esperava 401 sem token, obteve: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Token incorreto
	req, _ := http.NewRequest("GET", apiURL + "/status", nil)
	req.Header.Set("Authorization", "Bearer incorreto")
	resp, err = http.DefaultClient.Do(req)
	if err == nil {
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("esperava 401 com token incorreto, obteve: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Token correto
	req, _ = http.NewRequest("GET", apiURL + "/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("erro no status: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("esperava 200 com token correto, obteve: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// WS sem token
	wsURL := "ws://127.0.0.1:19100/ws"
	_, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("esperava falha de conexao WS sem token")
	}

	// WS com token
	wsURLWithToken := "ws://127.0.0.1:19100/ws?token=" + token
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURLWithToken, nil)
	if err != nil {
		t.Fatalf("erro ao conectar no WS com token: %v", err)
	}
	wsConn.Close()

	d.Stop()
	<-startErrChan
}

func TestGRPCServer_TokenAuthentication(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CROM_ENABLE_AUTH", "true")

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	d := NewDaemon(true)
	d.APIPort = 19110
	d.GRPCPort = 19111
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	// Aguarda subir
	grpcAddr := "127.0.0.1:19111"
	var conn *grpc.ClientConn
	var err error
	for i := 0; i < 30; i++ {
		conn, err = grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("falha ao conectar no gRPC: %v", err)
	}
	defer conn.Close()

	// Le o token gerado
	tokenBytes, err := os.ReadFile(filepath.Join(gDir, "session_token"))
	if err != nil {
		t.Fatalf("falha ao ler token de sessao: %v", err)
	}
	token := string(tokenBytes)

	ctx := context.Background()

	req := &pb.StartAgentRequest{Workspace: "/invalid_path/ws-test", Task: "tarefa-test"}
	resp := &pb.StartAgentResponse{}

	// 1. Chamada sem token
	err = conn.Invoke(ctx, "/daemon.AgentService/StartAgent", req, resp)
	if err == nil {
		t.Error("esperava erro sem token gRPC")
	}

	// 2. Chamada com token invalido
	badCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer invalid-token"))
	err = conn.Invoke(badCtx, "/daemon.AgentService/StartAgent", req, resp)
	if err == nil {
		t.Error("esperava erro com token gRPC invalido")
	}

	// 3. Chamada com token valido (mas workspace inexistente, entao deve falhar na execucao/orquestrador, nao na autenticacao)
	goodCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	err = conn.Invoke(goodCtx, "/daemon.AgentService/StartAgent", req, resp)
	if err != nil {
		t.Fatalf("erro ao chamar StartAgent com token gRPC valido: %v", err)
	}
	// Como o workspace nao existe, deve retornar Success = false na struct pb.StartAgentResponse, mas nao erro de gRPC/autenticacao
	if resp.Success {
		t.Error("esperava erro de workspace inexistente, mas retornou sucesso")
	}

	d.Stop()
	<-startErrChan
}

func TestAPIServer_MultipleWebSocketsConcurrency(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("CROM_ENABLE_AUTH", "true")

	gDir, _ := config.GlobalDir()
	_ = os.MkdirAll(gDir, 0755)

	gCfg := config.DefaultGlobalConfig()
	gCfg.DefaultProvider = "mock"
	_ = config.SaveGlobalConfig(gDir, gCfg)
	env := &config.EnvVars{}
	_ = env.Save(gDir)

	d := NewDaemon(true)
	d.APIPort = 19120
	d.GRPCPort = 19121
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- d.Start()
	}()

	// Aguarda subir
	apiURL := "http://127.0.0.1:19120"
	var err error
	for i := 0; i < 30; i++ {
		var resp *http.Response
		resp, err = http.Get(apiURL + "/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("falha ao conectar na API: %v", err)
	}

	tokenBytes, err := os.ReadFile(filepath.Join(gDir, "session_token"))
	if err != nil {
		t.Fatalf("falha ao ler token de sessao: %v", err)
	}
	token := string(tokenBytes)

	// Conecta N conexoes WebSocket simultaneas
	n := 5
	conns := make([]*websocket.Conn, n)
	wsURL := "ws://127.0.0.1:19120/ws?token=" + token

	for i := 0; i < n; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("falha na conexao WebSocket %d: %v", i, err)
		}
		conns[i] = conn
	}

	// Envia mensagens de subscribe simultaneas
	for i, conn := range conns {
		subMsg := IPCMessage{Type: "subscribe", Workspace: fmt.Sprintf("ws-%d", i)}
		if err := conn.WriteJSON(subMsg); err != nil {
			t.Fatalf("falha no write subscribe para conexao %d: %v", i, err)
		}
	}

	// Fecha todas as conexoes
	for _, conn := range conns {
		conn.Close()
	}

	d.Stop()
	<-startErrChan
}

