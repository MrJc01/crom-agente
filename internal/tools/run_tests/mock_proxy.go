package run_tests

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// MockProxy intercepta e loga conexões externas para evitar timeouts/travamentos nos testes
type MockProxy struct {
	listener net.Listener
	addr     string
	mu       sync.Mutex
	hosts    []string
	urls     []string
}

// StartMockProxy inicia um servidor proxy local dinâmico
func StartMockProxy() (*MockProxy, error) {
	// Escuta em localhost numa porta aleatória livre
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar listener do proxy: %w", err)
	}

	mp := &MockProxy{
		listener: l,
		addr:     l.Addr().String(),
	}

	go mp.serve()
	return mp, nil
}

// AddHost registra que um determinado host externo foi solicitado via CONNECT (HTTPS)
func (mp *MockProxy) AddHost(host string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	// Evita duplicatas simples
	for _, h := range mp.hosts {
		if h == host {
			return
		}
	}
	mp.hosts = append(mp.hosts, host)
}

// AddURL registra que uma URL externa foi solicitada via HTTP
func (mp *MockProxy) AddURL(urlStr string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	for _, u := range mp.urls {
		if u == urlStr {
			return
		}
	}
	mp.urls = append(mp.urls, urlStr)
}

// GetRecorded retorna a lista de hosts e URLs gravados
func (mp *MockProxy) GetRecorded() ([]string, []string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	hostsCopy := make([]string, len(mp.hosts))
	copy(hostsCopy, mp.hosts)

	urlsCopy := make([]string, len(mp.urls))
	copy(urlsCopy, mp.urls)

	return hostsCopy, urlsCopy
}

// Addr retorna o endereço do proxy (ex: 127.0.0.1:54321)
func (mp *MockProxy) Addr() string {
	return mp.addr
}

// Close encerra o listener
func (mp *MockProxy) Close() {
	if mp.listener != nil {
		_ = mp.listener.Close()
	}
}

func (mp *MockProxy) serve() {
	for {
		conn, err := mp.listener.Accept()
		if err != nil {
			return
		}
		go mp.handleConnection(conn)
	}
}

func (mp *MockProxy) handleConnection(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) < 2 {
		return
	}

	method := parts[0]
	target := parts[1]

	if method == "CONNECT" {
		mp.AddHost(target)
		// Para requisições HTTPS via CONNECT, podemos rejeitar a conexão com 502 de imediato.
		// Isso informa ao cliente que o acesso externo está bloqueado/indisponível,
		// falhando o teste instantaneamente ao invés de fazê-lo travar/timeout.
		resp := "HTTP/1.1 502 Bad Gateway\r\n" +
			"X-Crom-Mocked: true\r\n" +
			"Content-Length: 0\r\n\r\n"
		_, _ = conn.Write([]byte(resp))
	} else {
		mp.AddURL(target)
		// Para requisições HTTP normais, retornamos uma resposta simulada padrão de sucesso/mock
		resp := "HTTP/1.1 200 OK\r\n" +
			"Content-Type: application/json\r\n" +
			"X-Crom-Mocked: true\r\n" +
			"Content-Length: 46\r\n\r\n" +
			`{"status": "mocked", "message": "Crom Agent"}`
		_, _ = conn.Write([]byte(resp))
	}
}
