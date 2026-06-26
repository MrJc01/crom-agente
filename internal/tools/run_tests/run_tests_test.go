package run_tests_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/run_tests"
)

func TestRunTestsTool(t *testing.T) {
	ws := t.TempDir()
	tool := run_tests.NewRunTestsTool(ws)

	// 1. Testa detecção de stack vazia
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("erro ao executar run_tests: %v", err)
	}
	if res.Success {
		t.Fatal("esperava falha por falta de testes detectáveis")
	}

	// 2. Simula Go project
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test"), 0644)

	// Executa com comando customizado leve que sempre passa
	argsCustom := json.RawMessage(`{"command": "echo 'tests passed'"}`)
	res, err = tool.Execute(context.Background(), argsCustom)
	if err != nil || !res.Success {
		t.Fatalf("falha ao rodar testes customizados: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "tests passed") {
		t.Fatalf("saída inesperada: %s", res.Data)
	}

	// 3. Testa parsers de diagnóstico (cobertura, AssertionError e ambiente)
	argsDiagnostics := json.RawMessage(`{"command": "echo -e 'coverage: 92.4% of statements\\nAssertionError: expected nil\\nfoo/bar.go:123'"}`)
	resDiag, errDiag := tool.Execute(context.Background(), argsDiagnostics)
	if errDiag != nil || !resDiag.Success {
		t.Fatalf("falha ao executar testes de diagnóstico: %v, res: %+v", errDiag, resDiag)
	}
	
	if !strings.Contains(resDiag.Data, "coverage: 92.4%") {
		t.Errorf("esperava encontrar cobertura Go no output do diagnóstico, obteve:\n%s", resDiag.Data)
	}
	if !strings.Contains(resDiag.Data, "AssertionError: expected nil") {
		t.Errorf("esperava encontrar AssertionError no diagnóstico, obteve:\n%s", resDiag.Data)
	}
	if !strings.Contains(resDiag.Data, "foo/bar.go:123") {
		t.Errorf("esperava encontrar localizações suspeitas no diagnóstico, obteve:\n%s", resDiag.Data)
	}
}

func TestMockProxy(t *testing.T) {
	mp, err := run_tests.StartMockProxy()
	if err != nil {
		t.Fatalf("erro ao iniciar mock proxy: %v", err)
	}
	defer mp.Close()

	// 1. Testa requisição HTTP
	proxyURL, err := url.Parse("http://" + mp.Addr())
	if err != nil {
		t.Fatalf("erro ao parsear proxy url: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	resp, err := client.Get("http://example.com/some-endpoint")
	if err != nil {
		t.Fatalf("erro ao fazer get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code esperado 200, obteve: %d", resp.StatusCode)
	}

	if resp.Header.Get("X-Crom-Mocked") != "true" {
		t.Errorf("header X-Crom-Mocked esperado 'true', obteve: %q", resp.Header.Get("X-Crom-Mocked"))
	}

	// 2. Testa requisição HTTPS (deve retornar erro 502 / proxy error)
	respHttps, err := client.Get("https://api.openai.com/v1/chat/completions")
	if err == nil {
		respHttps.Body.Close()
		t.Errorf("esperava erro ao conectar HTTPS pelo proxy")
	}

	// Verifica se os logs capturaram as chamadas
	hosts, urls := mp.GetRecorded()
	if len(hosts) != 1 || !strings.Contains(hosts[0], "api.openai.com") {
		t.Errorf("esperava encontrar host api.openai.com nos logs, obteve: %v", hosts)
	}
	if len(urls) != 1 || !strings.Contains(urls[0], "http://example.com/some-endpoint") {
		t.Errorf("esperava encontrar url do example.com nos logs, obteve: %v", urls)
	}
}

func TestRunTestsToolWithProxy(t *testing.T) {
	ws := t.TempDir()
	tool := run_tests.NewRunTestsTool(ws)
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test"), 0644)

	args := json.RawMessage(`{"command": "env"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("falha ao rodar testes: %v, res: %+v", err, res)
	}

	if !strings.Contains(res.Data, "HTTP_PROXY=") {
		t.Errorf("esperava que a variável HTTP_PROXY estivesse injetada no ambiente do subprocesso, obteve:\n%s", res.Data)
	}
}
