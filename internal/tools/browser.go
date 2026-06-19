package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

// NewBrowserTool cria uma nova instância de BrowserTool
func NewBrowserTool(workspacePath string, headless bool) *BrowserTool {
	return &BrowserTool{
		workspacePath: workspacePath,
		headless:      headless,
	}
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
		el, err := page.Element(params.Selector)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("elemento %q não encontrado: %v", params.Selector, err)}, nil
		}
		err = el.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao clicar no elemento %q: %v", params.Selector, err)}, nil
		}
		return Result{Success: true, Data: fmt.Sprintf("Elemento %q clicado com sucesso", params.Selector)}, nil

	case "type":
		if params.Selector == "" || params.Text == "" {
			return Result{Success: false, Error: "selector e text são obrigatórios para a ação 'type'"}, nil
		}
		el, err := page.Element(params.Selector)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("elemento %q não encontrado: %v", params.Selector, err)}, nil
		}
		err = el.Input(params.Text)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao digitar no elemento %q: %v", params.Selector, err)}, nil
		}
		return Result{Success: true, Data: fmt.Sprintf("Texto digitado com sucesso no elemento %q", params.Selector)}, nil

	case "screenshot":
		if params.URL != "" {
			err := page.Navigate(params.URL)
			if err != nil {
				return Result{Success: false, Error: fmt.Sprintf("falha ao navegar para %s antes de capturar screenshot: %v", params.URL, err)}, nil
			}
			_ = page.WaitLoad()
			time.Sleep(1 * time.Second) // Delay para carregamento completo de recursos visuais
		}
		imgBytes, err := page.Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
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
