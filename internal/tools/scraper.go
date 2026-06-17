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

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ScraperTool faz raspagem de páginas HTML de documentação e as converte para Markdown
type ScraperTool struct {
	workspaceRoot string
}

// NewScraperTool cria a ferramenta scraper
func NewScraperTool(workspaceRoot string) *ScraperTool {
	return &ScraperTool{workspaceRoot: workspaceRoot}
}

func (t *ScraperTool) ID() string { return "scraper" }

func (t *ScraperTool) Description() string {
	return "Faz raspagem de páginas de documentação pública (HTML) e converte o conteúdo principal em Markdown limpo, ignorando elementos de navegação, cabeçalhos, rodapés e banners de cookies."
}

func (t *ScraperTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL da documentação a ser raspada"
			}
		},
		"required": ["url"]
	}`)
}

func (t *ScraperTool) RequiresApproval() bool { return false }

func (t *ScraperTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	u, err := url.Parse(input.URL)
	if err != nil {
		return Result{Success: false, Error: "URL inválida: " + err.Error()}, nil
	}

	// Proteção contra SSRF
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao resolver host %s: %v", host, err)}, nil
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("acesso negado por segurança (SSRF): o IP '%s' resolvido para o host '%s' está em uma faixa restrita", ip.String(), host),
			}, nil
		}
	}

	// Dispara requisição HTTP GET
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", input.URL, nil)
	if err != nil {
		return Result{Success: false, Error: "erro ao criar request: " + err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Crom-Agente/0.2.0 (Autonomous Agent; Scraper)")

	resp, err := client.Do(req)
	if err != nil {
		return Result{Success: false, Error: "erro ao disparar requisição: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Success: false, Error: fmt.Sprintf("resposta do servidor não-200: %s", resp.Status)}, nil
	}

	// Limita tamanho a 5MB
	limitReader := io.LimitReader(resp.Body, 5*1024*1024)
	doc, err := html.Parse(limitReader)
	if err != nil {
		return Result{Success: false, Error: "erro ao parsear HTML: " + err.Error()}, nil
	}

	// Converte para markdown
	markdown := convertHTMLToMarkdown(doc)
	cleaned := strings.TrimSpace(markdown)
	if cleaned == "" {
		cleaned = "(nenhum conteúdo útil extraído)"
	}

	return Result{Success: true, Data: cleaned}, nil
}

// convertHTMLToMarkdown extrai o conteúdo de forma recursiva e formata como markdown
func convertHTMLToMarkdown(n *html.Node) string {
	var sb strings.Builder
	walkHTML(n, &sb, false)
	return sb.String()
}

func walkHTML(n *html.Node, sb *strings.Builder, inPre bool) {
	if n == nil {
		return
	}

	// Ignorar tags irrelevantes ou de layout/cookies/navegação
	if n.Type == html.ElementNode {
		tagName := n.DataAtom.String()
		switch tagName {
		case "head", "script", "style", "nav", "footer", "iframe", "noscript", "header":
			return
		}

		// Filtrar classes e IDs que sugerem cookies, menus, rodapés, banners, etc.
		for _, attr := range n.Attr {
			val := strings.ToLower(attr.Val)
			if attr.Key == "id" || attr.Key == "class" {
				if strings.Contains(val, "cookie") || strings.Contains(val, "banner") ||
					strings.Contains(val, "footer") || strings.Contains(val, "menu") ||
					strings.Contains(val, "nav") || strings.Contains(val, "sidebar") {
					return
				}
			}
		}
	}

	// Processar o nó atual
	var blockPrefix, blockSuffix string
	var currentInPre = inPre

	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.H1:
			blockPrefix = "\n# "
			blockSuffix = "\n"
		case atom.H2:
			blockPrefix = "\n## "
			blockSuffix = "\n"
		case atom.H3:
			blockPrefix = "\n### "
			blockSuffix = "\n"
		case atom.H4:
			blockPrefix = "\n#### "
			blockSuffix = "\n"
		case atom.H5, atom.H6:
			blockPrefix = "\n##### "
			blockSuffix = "\n"
		case atom.P:
			blockPrefix = "\n"
			blockSuffix = "\n"
		case atom.Br:
			blockPrefix = "\n"
		case atom.Li:
			blockPrefix = "\n- "
		case atom.Pre:
			blockPrefix = "\n```\n"
			blockSuffix = "\n```\n"
			currentInPre = true
		case atom.Code:
			if !inPre {
				blockPrefix = "`"
				blockSuffix = "`"
			}
		case atom.A:
			// Extrair link
			var href string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
					break
				}
			}
			// Para links simples, apenas processamos os filhos normalmente
			_ = href // Pode ser expandido futuramente: [text](href)
		}
	}

	sb.WriteString(blockPrefix)

	if n.Type == html.TextNode {
		text := n.Data
		if !inPre {
			text = strings.ReplaceAll(text, "\n", " ")
			text = strings.Join(strings.Fields(text), " ")
			if text != "" {
				currentStr := sb.String()
				// Só insere espaço se o caractere anterior não for um delimitador de bloco ou espaço
				if len(currentStr) > 0 && 
					!strings.HasSuffix(currentStr, " ") && 
					!strings.HasSuffix(currentStr, "\n") && 
					!strings.HasSuffix(currentStr, "# ") && 
					!strings.HasSuffix(currentStr, "- ") &&
					!strings.HasSuffix(currentStr, "`") {
					sb.WriteString(" ")
				}
				sb.WriteString(text)
			}
		} else {
			sb.WriteString(text)
		}
	}

	// Recursão para os filhos
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, sb, currentInPre)
	}

	sb.WriteString(blockSuffix)
}
