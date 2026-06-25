package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/database_tester"
	"github.com/crom/crom-agente/internal/tools/http_client"
	"github.com/crom/crom-agente/internal/tools/proxy"
	"github.com/crom/crom-agente/internal/tools/scraper"
	"golang.org/x/net/html"
)

func TestHTTPClient_NormalAndSSRF(t *testing.T) {
	ws := t.TempDir()
	tool := http_client.NewHTTPClientTool(ws)

	// 1. Servidor HTTP de teste
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("resposta do servidor de teste"))
	}))
	defer ts.Close()

	// 2. Requisição normal (vai falhar devido à proteção SSRF caso localhost/127.0.0.1 seja bloqueado)
	// Vamos validar se bloqueia 127.0.0.1
	argsLocal := json.RawMessage(fmt.Sprintf(`{"url": "%s"}`, ts.URL))
	res, err := tool.Execute(context.Background(), argsLocal)
	if err != nil {
		t.Fatalf("erro ao executar http_client: %v", err)
	}
	if res.Success {
		t.Fatal("esperava que http_client bloqueasse acesso a localhost (SSRF protection)")
	}
	if !strings.Contains(res.Error, "SSRF") {
		t.Fatalf("esperava erro de SSRF, obteve: %s", res.Error)
	}

	// 3. Testa com URL pública (gemini.google.com ou similar - mockando o DNS resolution para evitar falha se offline)
	// Mas no nosso código, net.LookupIP é usado.
	// Vamos validar se a mensagem de erro é de DNS ou SSRF.
	argsBadDNS := json.RawMessage(`{"url": "http://10.0.0.1/metadata"}`)
	res, _ = tool.Execute(context.Background(), argsBadDNS)
	if res.Success {
		t.Fatal("esperava falha por SSRF ao tentar acessar IP privado 10.0.0.1")
	}
	if !strings.Contains(res.Error, "SSRF") {
		t.Fatalf("esperava erro contendo 'SSRF', obteve: %s", res.Error)
	}
}

func TestScraper(t *testing.T) {
	ws := t.TempDir()
	tool := scraper.NewScraperTool(ws)

	// Validação de SSRF na URL local
	argsLocal := json.RawMessage(`{"url": "http://localhost:8080"}`)
	res, err := tool.Execute(context.Background(), argsLocal)
	if err != nil {
		t.Fatalf("erro ao executar scraper: %v", err)
	}
	if res.Success {
		t.Fatal("esperava bloqueio de SSRF no scraper")
	}

	// Teste do parser HTML interno diretamente
	htmlContent := `
	<html>
		<head><title>Ignorado</title></head>
		<body>
			<header>Ignorado</header>
			<nav>Navegacao ignorada</nav>
			<div class="cookie-banner">Banner de cookies ignorado</div>
			<div id="footer">Rodapé ignorado</div>
			<h1>Titulo Principal</h1>
			<p>Texto do paragrafo com <code>codigo inline</code>.</p>
			<pre><code>codigo em bloco</code></pre>
			<ul>
				<li>Item 1</li>
				<li>Item 2</li>
			</ul>
		</body>
	</html>`

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("erro ao parsear html de teste: %v", err)
	}

	markdown := scraper.ConvertHTMLToMarkdown(doc)
	cleaned := strings.TrimSpace(markdown)

	if !strings.Contains(cleaned, "# Titulo Principal") {
		t.Fatalf("markdown gerado nao contem h1 formatado: %s", cleaned)
	}
	if !strings.Contains(cleaned, "- Item 1") {
		t.Fatalf("markdown gerado nao contem lista formatada: %s", cleaned)
	}
	if strings.Contains(cleaned, "cookie-banner") || strings.Contains(cleaned, "Rodapé ignorado") {
		t.Fatalf("markdown gerado contem tags que deveriam ser ignoradas: %s", cleaned)
	}
}

func TestDatabaseTester_SQLite(t *testing.T) {
	ws := t.TempDir()
	tool := database_tester.NewDatabaseTesterTool(ws)

	// 1. Testa criando e pingando SQLite real
	args := json.RawMessage(`{"type": "sqlite", "dsn": "test.db"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("falha ao testar conexao sqlite: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "Conexão SQLite estabelecida") {
		t.Fatalf("mensagem inesperada: %s", res.Data)
	}

	// 2. Tenta acessar fora do sandbox jail
	argsBad := json.RawMessage(`{"type": "sqlite", "dsn": "../externo.db"}`)
	res, _ = tool.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava erro de sandbox jail para arquivo externo")
	}
	if !strings.Contains(res.Error, "está fora do sandbox") {
		t.Fatalf("mensagem de erro inesperada: %s", res.Error)
	}
}

func TestProxy(t *testing.T) {
	ws := t.TempDir()
	tool := proxy.NewProxyTool(ws, true)

	// 1. Iniciar servidor TCP de destino mock
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao iniciar target listener: %v", err)
	}
	defer targetListener.Close()

	targetAddr := targetListener.Addr().String()

	// Handler do target server (apenas ecoa de volta)
	go func() {
		for {
			conn, err := targetListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	// 2. Inicia o proxy vinculando à porta dinâmica
	logFile := "proxy_test.log"
	startArgs := json.RawMessage(fmt.Sprintf(`{
		"action": "start",
		"local_port": 0,
		"target_addr": "%s",
		"log_file": "%s"
	}`, targetAddr, logFile))

	res, err := tool.Execute(context.Background(), startArgs)
	if err != nil || !res.Success {
		t.Fatalf("falha ao iniciar proxy: %v, res: %+v", err, res)
	}

	// Extrair proxy_id do retorno
	var proxyInfo struct {
		ID        string `json:"id"`
		LocalAddr string `json:"local_addr"`
	}
	// res.Data contem uma mensagem textual antes do JSON, vamos extrair do JSON na segunda linha
	lines := strings.Split(res.Data, "\n")
	jsonStr := strings.Join(lines[1:], "\n")
	if err := json.Unmarshal([]byte(jsonStr), &proxyInfo); err != nil {
		t.Fatalf("erro ao parsear json de resposta do proxy: %v\nData: %s", err, res.Data)
	}

	if proxyInfo.ID == "" || proxyInfo.LocalAddr == "" {
		t.Fatalf("proxy ID ou LocalAddr vazios: %+v", proxyInfo)
	}

	// 3. Conectar ao proxy local e enviar dados
	conn, err := net.Dial("tcp", proxyInfo.LocalAddr)
	if err != nil {
		t.Fatalf("falha ao conectar no proxy local: %v", err)
	}

	msg := "testando proxy tcp"
	_, err = conn.Write([]byte(msg))
	if err != nil {
		t.Fatalf("erro ao enviar dados: %v", err)
	}

	reply := make([]byte, 100)
	n, err := conn.Read(reply)
	if err != nil {
		t.Fatalf("erro ao ler resposta: %v", err)
	}
	conn.Close()

	if string(reply[:n]) != msg {
		t.Fatalf("resposta ecoada invalida: %s", string(reply[:n]))
	}

	// 4. Listar proxies ativos
	listRes, err := tool.Execute(context.Background(), json.RawMessage(`{"action": "list"}`))
	if err != nil || !listRes.Success {
		t.Fatalf("erro ao listar proxies: %v", err)
	}
	if !strings.Contains(listRes.Data, proxyInfo.ID) {
		t.Fatalf("proxy ativo nao encontrado na listagem: %s", listRes.Data)
	}

	// 5. Parar o proxy
	stopArgs := json.RawMessage(fmt.Sprintf(`{"action": "stop", "proxy_id": "%s"}`, proxyInfo.ID))
	stopRes, err := tool.Execute(context.Background(), stopArgs)
	if err != nil || !stopRes.Success {
		t.Fatalf("erro ao parar proxy: %v", err)
	}

	// Verificar se arquivo de log do proxy foi criado no workspace
	logPath := filepath.Join(ws, logFile)
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("arquivo de log do proxy nao foi criado: %v", err)
	}

	logData, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logData), "testando proxy tcp") {
		t.Fatalf("dados enviados nao foram gravados no log do proxy: %s", string(logData))
	}
}
