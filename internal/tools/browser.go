package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// BrowserTool controla uma instância do Chromium rodando via CDP
type BrowserTool struct {
	mu            sync.Mutex
	workspacePath string
	headless      bool
	browser       *rod.Browser
	page          *rod.Page
	onNavigate    func(url string)
	restoreURL    func() string
}

// NewBrowserTool cria uma nova instância de BrowserTool
func NewBrowserTool(workspacePath string, headless bool) *BrowserTool {
	return &BrowserTool{
		workspacePath: workspacePath,
		headless:      headless,
	}
}

// SetOnNavigate define o callback para mudanças de URL do navegador
func (b *BrowserTool) SetOnNavigate(cb func(url string)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onNavigate = cb
}

// SetRestoreURL define a função para recuperar a última URL a fim de restaurá-la na inicialização
func (b *BrowserTool) SetRestoreURL(cb func() string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreURL = cb
}

// ID retorna o identificador único da ferramenta
func (b *BrowserTool) ID() string {
	return "browser_action"
}

// Description retorna a descrição da ferramenta para o LLM
func (b *BrowserTool) Description() string {
	return "Controla um navegador web para interagir com páginas (navegar, clicar, digitar, extrair HTML, tirar screenshots)."
}

// ParametersSchema define os parâmetros aceitos em formato JSON Schema
func (b *BrowserTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["navigate", "click", "type", "screenshot", "get_html"],
				"description": "Ação a ser executada no navegador"
			},
			"url": {
				"type": "string",
				"description": "URL para navegação (obrigatório para action 'navigate')"
			},
			"selector": {
				"type": "string",
				"description": "Seletor CSS do elemento (obrigatório para actions 'click' e 'type')"
			},
			"text": {
				"type": "string",
				"description": "Texto a ser digitado (obrigatório para action 'type')"
			},
			"path": {
				"type": "string",
				"description": "Caminho opcional do arquivo para salvar a captura de tela (ex: 'screenshot.png'). Se especificado, grava a imagem diretamente no disco no caminho informado."
			}
		},
		"required": ["action"]
	}`)
}

// RequiresApproval indica se a ação necessita de aprovação HITL.
// Interações web são seguras contra alterações de sistema do host, por isso retornamos false.
func (b *BrowserTool) RequiresApproval() bool {
	return false
}

func (b *BrowserTool) getPage() (*rod.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil {
		l := launcher.New().Headless(b.headless)
		l.Set("disable-gpu")
		l.Set("no-sandbox")
		l.Set("disable-setuid-sandbox")
		l.Set("disable-dev-shm-usage")
		
		// Procura navegador no PATH padrão (Chrome, Chromium, Brave, Edge)
		if path, found := launcher.LookPath(); found {
			l.Bin(path)
		}

		u, err := l.Launch()
		if err != nil {
			return nil, fmt.Errorf("falha ao iniciar launcher do browser: %w", err)
		}

		b.browser = rod.New().ControlURL(u).MustConnect()
	}

	if b.page == nil {
		p, err := b.browser.Page(proto.TargetCreateTarget{})
		if err != nil {
			return nil, fmt.Errorf("falha ao abrir nova aba/página: %w", err)
		}
		b.page = p

		if b.restoreURL != nil {
			savedURL := b.restoreURL()
			if savedURL != "" && savedURL != "about:blank" {
				_ = p.Navigate(savedURL)
				_ = p.WaitLoad()
			}
		}
	}

	return b.page, nil
}

// Close fecha o browser e limpa recursos
func (b *BrowserTool) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.browser != nil {
		_ = b.browser.Close()
		b.browser = nil
		b.page = nil
	}
}

// Execute executa as ações requisitadas no navegador
func (b *BrowserTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Action   string `json:"action"`
		URL      string `json:"url"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
		Path     string `json:"path"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, err
	}

	page, err := b.getPage()
	if err != nil {
		return Result{Success: false, Error: err.Error()}, err
	}

	// Timeout individual para interações
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	page = page.Context(timeoutCtx)

	defer func() {
		if page != nil {
			if info, err := page.Info(); err == nil && info != nil && info.URL != "" {
				b.mu.Lock()
				cb := b.onNavigate
				b.mu.Unlock()
				if cb != nil {
					cb(info.URL)
				}
			}
		}
	}()

	switch params.Action {
	case "navigate":
		if params.URL == "" {
			return Result{Success: false, Error: "URL é obrigatória para a ação 'navigate'"}, nil
		}
		err := page.Navigate(params.URL)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao navegar para %s: %v", params.URL, err)}, nil
		}
		_ = page.WaitLoad()
		return Result{Success: true, Data: fmt.Sprintf("Navegado com sucesso para %s", params.URL)}, nil

	case "click":
		if params.Selector == "" {
			return Result{Success: false, Error: "selector é obrigatório para a ação 'click'"}, nil
		}
		searchCtx, searchCancel := context.WithTimeout(timeoutCtx, 5*time.Second)
		el, err := page.Context(searchCtx).Element(params.Selector)
		if err != nil {
			searchCancel()
			sug := b.getSuggestions(page)
			return Result{Success: false, Error: fmt.Sprintf("elemento %q não encontrado (timeout 5s): %v.\nElementos clicáveis disponíveis na página:\n%s", params.Selector, err, sug)}, nil
		}
		err = el.Click(proto.InputMouseButtonLeft, 1)
		searchCancel()
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao clicar no elemento %q: %v", params.Selector, err)}, nil
		}
		
		// Aguarda um pequeno momento para renderização e transições
		time.Sleep(500 * time.Millisecond)
		newURL := ""
		title := ""
		if info, err := page.Info(); err == nil && info != nil {
			newURL = info.URL
			title = info.Title
		}
		return Result{Success: true, Data: fmt.Sprintf("Elemento %q clicado com sucesso. URL atual: %s | Título: %s", params.Selector, newURL, title)}, nil

	case "type":
		if params.Selector == "" || params.Text == "" {
			return Result{Success: false, Error: "selector e text são obrigatórios para a ação 'type'"}, nil
		}
		searchCtx, searchCancel := context.WithTimeout(timeoutCtx, 5*time.Second)
		el, err := page.Context(searchCtx).Element(params.Selector)
		if err != nil {
			searchCancel()
			sug := b.getSuggestions(page)
			return Result{Success: false, Error: fmt.Sprintf("elemento %q não encontrado (timeout 5s): %v.\nElementos clicáveis disponíveis na página:\n%s", params.Selector, err, sug)}, nil
		}
		err = el.Input(params.Text)
		searchCancel()
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao digitar no elemento %q: %v", params.Selector, err)}, nil
		}
		
		// Aguarda um pequeno momento para renderização e transições
		time.Sleep(500 * time.Millisecond)
		newURL := ""
		title := ""
		if info, err := page.Info(); err == nil && info != nil {
			newURL = info.URL
			title = info.Title
		}
		return Result{Success: true, Data: fmt.Sprintf("Texto digitado com sucesso no elemento %q. URL atual: %s | Título: %s", params.Selector, newURL, title)}, nil

	case "screenshot":
		if params.URL != "" {
			err := page.Navigate(params.URL)
			if err != nil {
				return Result{Success: false, Error: fmt.Sprintf("falha ao navegar para %s antes de capturar screenshot: %v", params.URL, err)}, nil
			}
			_ = page.WaitLoad()
			time.Sleep(1 * time.Second) // Delay para carregamento completo de recursos visuais
		}
		imgBytes, err := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao capturar screenshot: %v", err)}, nil
		}
		b64 := base64.StdEncoding.EncodeToString(imgBytes)
		if params.Path != "" {
			targetFile, err := ValidatePath(b.workspacePath, params.Path, false)
			if err != nil {
				return Result{Success: false, Error: fmt.Sprintf("caminho de destino inválido: %v", err)}, nil
			}
			if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
				return Result{Success: false, Error: fmt.Sprintf("falha ao criar diretórios pai: %v", err)}, nil
			}
			if err := os.WriteFile(targetFile, imgBytes, 0644); err != nil {
				return Result{Success: false, Error: fmt.Sprintf("falha ao salvar screenshot no disco: %v", err)}, nil
			}
			return Result{Success: true, Data: "image:base64:" + b64 + "\n✓ Screenshot salvo em: " + params.Path}, nil
		}
		return Result{Success: true, Data: "image:base64:" + b64}, nil

	case "get_html":
		html, err := page.HTML()
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao extrair HTML: %v", err)}, nil
		}
		return Result{Success: true, Data: html}, nil

	default:
		return Result{Success: false, Error: fmt.Sprintf("ação desconhecida: %s", params.Action)}, nil
	}
}

// GetCurrentPageContent retorna o HTML e a URL da página atual
func (b *BrowserTool) GetCurrentPageContent() (string, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.page == nil {
		return "", "", fmt.Errorf("navegador não iniciado")
	}

	html, err := b.page.HTML()
	if err != nil {
		return "", "", err
	}

	info, err := b.page.Info()
	if err != nil {
		return html, "", nil
	}

	return html, info.URL, nil
}

// getSuggestions varre os elementos interativos da página e sugere seletores
func (b *BrowserTool) getSuggestions(page *rod.Page) string {
	elements, err := page.Elements("a, button, [role='button'], input[type='button'], input[type='submit']")
	if err != nil {
		return ""
	}
	var list []string
	count := 0
	for _, el := range elements {
		if count >= 15 {
			break
		}
		nodeDesc, err := el.Describe(1, false)
		if err != nil {
			continue
		}
		tagName := strings.ToLower(nodeDesc.LocalName)
		if tagName == "" {
			tagName = strings.ToLower(nodeDesc.NodeName)
		}
		text, _ := el.Text()
		href, _ := el.Attribute("href")
		id, _ := el.Attribute("id")
		class, _ := el.Attribute("class")

		selector := tagName
		if id != nil && *id != "" {
			selector += "#" + *id
		} else if class != nil && *class != "" {
			fields := strings.Fields(*class)
			if len(fields) > 0 {
				selector += "." + fields[0]
			}
		}
		if href != nil && *href != "" && (tagName == "a" || tagName == "link") {
			selector += fmt.Sprintf("[href=%q]", *href)
		}

		cleanText := strings.TrimSpace(text)
		if cleanText != "" {
			list = append(list, fmt.Sprintf("- Seletor: %q | Texto: %q", selector, cleanText))
			count++
		} else if href != nil && *href != "" {
			list = append(list, fmt.Sprintf("- Seletor: %q", selector))
			count++
		}
	}
	if len(list) == 0 {
		return "Nenhum elemento interativo óbvio foi encontrado."
	}
	return strings.Join(list, "\n")
}
