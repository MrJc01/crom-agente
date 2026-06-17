package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClientTool executa requisições HTTP seguras (protegidas contra SSRF e com limite de tamanho)
type HTTPClientTool struct {
	workspaceRoot string
}

// NewHTTPClientTool cria a ferramenta http_client
func NewHTTPClientTool(workspaceRoot string) *HTTPClientTool {
	return &HTTPClientTool{workspaceRoot: workspaceRoot}
}

func (t *HTTPClientTool) ID() string { return "http_client" }

func (t *HTTPClientTool) Description() string {
	return "Executa requisições HTTP GET ou POST seguras para servidores externos. Protege contra SSRF (bloqueia IPs de redes privadas e locais) e possui limites rígidos de timeout e tamanho de resposta."
}

func (t *HTTPClientTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"method": {
				"type": "string",
				"enum": ["GET", "POST"],
				"description": "Método HTTP (GET ou POST)",
				"default": "GET"
			},
			"url": {
				"type": "string",
				"description": "URL de destino (ex: https://api.github.com/repos/octocat/hello-world)"
			},
			"headers": {
				"type": "object",
				"additionalProperties": {
					"type": "string"
				},
				"description": "Headers HTTP opcionais"
			},
			"body": {
				"type": "string",
				"description": "Corpo da requisição para chamadas POST"
			}
		},
		"required": ["url"]
	}`)
}

func (t *HTTPClientTool) RequiresApproval() bool { return false }

func (t *HTTPClientTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	method := strings.ToUpper(input.Method)
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "POST" {
		return Result{Success: false, Error: "método HTTP não suportado. Apenas GET e POST são permitidos."}, nil
	}

	u, err := url.Parse(input.URL)
	if err != nil {
		return Result{Success: false, Error: "URL inválida: " + err.Error()}, nil
	}

	// Proteção contra SSRF: resolver host e checar se é IP privado ou de loopback
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao resolver host %s: %v", host, err)}, nil
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("acesso negado por segurança (SSRF): o IP '%s' resolvido para o host '%s' está em uma faixa restrita (privada/loopback)", ip.String(), host),
			}, nil
		}
	}

	// Cliente HTTP com timeout curto (10 segundos)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var bodyReader io.Reader
	if method == "POST" {
		bodyReader = strings.NewReader(input.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, input.URL, bodyReader)
	if err != nil {
		return Result{Success: false, Error: "erro ao criar request: " + err.Error()}, nil
	}

	// Headers padrão
	req.Header.Set("User-Agent", "Crom-Agente/0.2.0 (Autonomous Agent; +https://github.com/crom/crom-agente)")
	for k, v := range input.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Result{Success: false, Error: "erro ao disparar requisição: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	// Limitar leitura de resposta para evitar downloads gigantes (máximo de 2MB)
	limitReader := io.LimitReader(resp.Body, 2*1024*1024)
	respData, err := io.ReadAll(limitReader)
	if err != nil {
		return Result{Success: false, Error: "erro ao ler corpo da resposta: " + err.Error()}, nil
	}

	return Result{
		Success: true,
		Data:    fmt.Sprintf("Status: %s\n\n%s", resp.Status, string(respData)),
	}, nil
}
