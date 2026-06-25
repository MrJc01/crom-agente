package browser_subagent

import (
	"context"
	"encoding/base64"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/tools"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de browser_subagent: " + err.Error())
	}
}

// BrowserSubagentTool executa tarefas completas de navegação de forma autônoma,
// combinando múltiplas ações (navegar, clicar, digitar, verificar resultado via screenshot)
// em um único loop de autodecisão visual.
type BrowserSubagentTool struct {
	mu            sync.Mutex
	workspacePath string
	headless      bool
	browser       *rod.Browser
	page          *rod.Page
	onNavigate    func(url string)
	restoreURL    func() string
}

// NewBrowserSubagentTool cria uma nova instância da ferramenta de subagente de navegador
func NewBrowserSubagentTool(workspacePath string, headless bool) *BrowserSubagentTool {
	return &BrowserSubagentTool{
		workspacePath: workspacePath,
		headless:      headless,
	}
}

// SetOnNavigate define o callback para mudanças de URL do navegador
func (b *BrowserSubagentTool) SetOnNavigate(cb func(url string)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onNavigate = cb
}

// SetRestoreURL define a função para recuperar a última URL a fim de restaurá-la na inicialização
func (b *BrowserSubagentTool) SetRestoreURL(cb func() string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreURL = cb
}

// ID retorna o identificador único da ferramenta
func (b *BrowserSubagentTool) ID() string {
	return metadata.ID
}

// Description descreve a ferramenta para o LLM
func (b *BrowserSubagentTool) Description() string {
	return metadata.Description
}

// ParametersSchema define o JSON Schema dos parâmetros
func (b *BrowserSubagentTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Descrição em texto livre da tarefa a ser executada no navegador (ex: 'Faça login em example.com com usuário admin e senha 1234 e confirme que está logado')."
			},
			"steps": {
				"type": "array",
				"description": "Lista de ações a executar em ordem",
				"items": {
					"type": "object",
					"properties": {
						"action": {
							"type": "string",
							"enum": ["navigate", "click", "type", "wait", "screenshot", "get_html", "scroll"],
							"description": "Tipo de ação a executar"
						},
						"url": {
							"type": "string",
							"description": "URL para navegar (para action 'navigate')"
						},
						"selector": {
							"type": "string",
							"description": "Seletor CSS do elemento (para 'click' e 'type')"
						},
						"text": {
							"type": "string",
							"description": "Texto a digitar ou usar como verificação visual"
						},
						"seconds": {
							"type": "number",
							"description": "Segundos para aguardar (para action 'wait')"
						},
						"path": {
							"type": "string",
							"description": "Caminho do arquivo para salvar o screenshot (para action 'screenshot')"
						},
						"verify_contains": {
							"type": "string",
							"description": "Texto que deve estar presente no HTML da página para que a etapa seja considerada bem-sucedida"
						}
					},
					"required": ["action"]
				}
			},
			"capture_final_screenshot": {
				"type": "boolean",
				"description": "Se true, tira um screenshot final após todas as etapas e o inclui no relatório"
			}
		},
		"required": ["steps"]
	}`)
}

// RequiresApproval indica que esta ferramenta não requer aprovação manual (automatizada)
func (b *BrowserSubagentTool) RequiresApproval() bool {
	return false
}

type subagentStep struct {
	Action         string  `json:"action"`
	URL            string  `json:"url"`
	Selector       string  `json:"selector"`
	Text           string  `json:"text"`
	Seconds        float64 `json:"seconds"`
	Path           string  `json:"path"`
	VerifyContains string  `json:"verify_contains"`
}

type subagentParams struct {
	Task                   string         `json:"task"`
	Steps                  []subagentStep `json:"steps"`
	CaptureFinalScreenshot bool           `json:"capture_final_screenshot"`
}

// StepResult representa o resultado de uma etapa da execução
type StepResult struct {
	Step          int    `json:"step"`
	Action        string `json:"action"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ScreenshotB64 string `json:"screenshot_b64,omitempty"` // presente apenas em etapas de screenshot
}

// Execute executa a sequência de etapas definidas no subagente
func (b *BrowserSubagentTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var params subagentParams
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos de JSON: " + err.Error()}, nil
	}

	if len(params.Steps) == 0 {
		return tools.Result{Success: false, Error: "nenhuma etapa definida em 'steps'"}, nil
	}

	page, err := b.getPage()
	if err != nil {
		return tools.Result{Success: false, Error: "falha ao iniciar navegador: " + err.Error()}, nil
	}

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

	var results []StepResult
	allSuccess := true

	for i, step := range params.Steps {
		stepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		p := page.Context(stepCtx)

		sr := StepResult{
			Step:   i + 1,
			Action: step.Action,
		}

		switch step.Action {
		case "navigate":
			if step.URL == "" {
				sr.Success = false
				sr.Message = "URL é obrigatória para 'navigate'"
			} else {
				if err := p.Navigate(step.URL); err != nil {
					sr.Success = false
					sr.Message = fmt.Sprintf("falha ao navegar para %s: %v", step.URL, err)
				} else {
					_ = p.WaitLoad()
					sr.Success = true
					sr.Message = fmt.Sprintf("Navegado com sucesso para %s", step.URL)
				}
			}

		case "click":
			if step.Selector == "" {
				sr.Success = false
				sr.Message = "selector é obrigatório para 'click'"
			} else {
				searchCtx, searchCancel := context.WithTimeout(stepCtx, 5*time.Second)
				el, err := p.Context(searchCtx).Element(step.Selector)
				if err != nil {
					searchCancel()
					sr.Success = false
					sr.Message = fmt.Sprintf("elemento %q não encontrado (timeout 5s): %v", step.Selector, err)
				} else {
					errClick := el.Click(proto.InputMouseButtonLeft, 1)
					searchCancel()
					if errClick != nil {
						sr.Success = false
						sr.Message = fmt.Sprintf("falha ao clicar em %q: %v", step.Selector, errClick)
					} else {
						// Auto-verificação visual: tira screenshot após clique para confirmar resultado
						imgBytes, screenshotErr := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
						if screenshotErr == nil {
							sr.ScreenshotB64 = base64.StdEncoding.EncodeToString(imgBytes)
						}
						time.Sleep(500 * time.Millisecond)
						newURL := ""
						title := ""
						if info, err := p.Info(); err == nil && info != nil {
							newURL = info.URL
							title = info.Title
						}
						sr.Success = true
						sr.Message = fmt.Sprintf("Elemento %q clicado com sucesso. URL atual: %s | Título: %s", step.Selector, newURL, title)
					}
				}
			}

		case "type":
			if step.Selector == "" || step.Text == "" {
				sr.Success = false
				sr.Message = "selector e text são obrigatórios para 'type'"
			} else {
				searchCtx, searchCancel := context.WithTimeout(stepCtx, 5*time.Second)
				el, err := p.Context(searchCtx).Element(step.Selector)
				if err != nil {
					searchCancel()
					sr.Success = false
					sr.Message = fmt.Sprintf("elemento %q não encontrado (timeout 5s): %v", step.Selector, err)
				} else {
					isContentEditable := false
					if val, errAttr := el.Attribute("contenteditable"); errAttr == nil && val != nil && *val == "true" {
						isContentEditable = true
					}
					if !isContentEditable {
						if classVal, errAttr := el.Attribute("class"); errAttr == nil && classVal != nil && strings.Contains(*classVal, "ProseMirror") {
							isContentEditable = true
						}
					}

					var errInput error
					if isContentEditable {
						if errFocus := el.Focus(); errFocus != nil {
							errInput = errFocus
						} else {
							_, errInput = el.Eval(`(el, txt) => {
								el.focus();
								document.execCommand('selectAll', false, null);
								document.execCommand('delete', false, null);
								document.execCommand('insertText', false, txt);
								el.dispatchEvent(new Event('input', { bubbles: true }));
								return true;
							}`, step.Text)
						}
					} else {
						errInput = el.Input(step.Text)
					}
					searchCancel()
					if errInput != nil {
						sr.Success = false
						sr.Message = fmt.Sprintf("falha ao digitar em %q: %v", step.Selector, errInput)
					} else {
						time.Sleep(500 * time.Millisecond)
						newURL := ""
						title := ""
						if info, err := p.Info(); err == nil && info != nil {
							newURL = info.URL
							title = info.Title
						}
						sr.Success = true
						sr.Message = fmt.Sprintf("Texto digitado com sucesso em %q. URL atual: %s | Título: %s", step.Selector, newURL, title)
					}
				}
			}

		case "wait":
			secs := step.Seconds
			if secs <= 0 {
				secs = 1
			}
			time.Sleep(time.Duration(secs * float64(time.Second)))
			sr.Success = true
			sr.Message = fmt.Sprintf("Aguardou %.1f segundo(s)", secs)

		case "scroll":
			// Scroll para o final da página
			_, _ = p.Eval(`window.scrollTo(0, document.body.scrollHeight)`)
			sr.Success = true
			sr.Message = "Scroll realizado até o final da página"

		case "get_html":
			html, err := p.HTML()
			if err != nil {
				sr.Success = false
				sr.Message = fmt.Sprintf("falha ao obter HTML: %v", err)
			} else {
				// Trunca para não inflar o relatório
				if len(html) > 500000 {
					html = html[:500000] + "...[truncado]"
				}
				sr.Success = true
				sr.Message = "HTML obtido:\n" + html
			}

		case "screenshot":
			if step.URL != "" {
				_ = p.Navigate(step.URL)
				_ = p.WaitLoad()
				time.Sleep(time.Second)
			}
			imgBytes, err := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
			if err != nil {
				sr.Success = false
				sr.Message = fmt.Sprintf("falha ao capturar screenshot: %v", err)
			} else {
				b64str := base64.StdEncoding.EncodeToString(imgBytes)
				sr.ScreenshotB64 = b64str
				sr.Success = true
				sr.Message = "Screenshot capturado com sucesso"

				// Salvar no disco se path for fornecido
				if step.Path != "" {
					targetFile, pathErr := tools.ValidatePath(b.workspacePath, step.Path, false)
					if pathErr == nil {
						_ = os.MkdirAll(filepath.Dir(targetFile), 0755)
						if writeErr := os.WriteFile(targetFile, imgBytes, 0644); writeErr == nil {
							sr.Message += fmt.Sprintf(" | Salvo em: %s", step.Path)
						}
					}
				}
			}

		default:
			sr.Success = false
			sr.Message = fmt.Sprintf("ação desconhecida: %s", step.Action)
		}

		// Verificação de conteúdo HTML se solicitada
		if sr.Success && step.VerifyContains != "" {
			html, err := p.HTML()
			if err != nil || !strings.Contains(html, step.VerifyContains) {
				sr.Success = false
				sr.Message += fmt.Sprintf(" | VERIFICAÇÃO FALHOU: texto %q não encontrado na página", step.VerifyContains)
				allSuccess = false
			} else {
				sr.Message += fmt.Sprintf(" | Verificação OK: %q encontrado", step.VerifyContains)
			}
		}

		if !sr.Success {
			allSuccess = false
		}

		results = append(results, sr)
		cancel()
	}

	// Screenshot final automático se solicitado
	if params.CaptureFinalScreenshot {
		finalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		p := page.Context(finalCtx)
		imgBytes, err := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
		cancel()
		if err == nil {
			results = append(results, StepResult{
				Step:          len(results) + 1,
				Action:        "screenshot_final",
				Success:       true,
				Message:       "Screenshot final automático capturado",
				ScreenshotB64: base64.StdEncoding.EncodeToString(imgBytes),
			})
		}
	}

	// Serializa o relatório como JSON legível pelo agente
	reportJSON, err := json.MarshalIndent(map[string]interface{}{
		"task":        params.Task,
		"total_steps": len(params.Steps),
		"all_success": allSuccess,
		"results":     results,
	}, "", "  ")
	if err != nil {
		reportJSON = []byte(`{"error": "falha ao serializar relatório"}`)
	}

	// Se houve screenshot final, retorna como imagem + texto
	for _, r := range results {
		if r.Action == "screenshot_final" && r.ScreenshotB64 != "" {
			return tools.Result{
				Success: allSuccess,
				Data:    "image:base64:" + r.ScreenshotB64 + "\n" + string(reportJSON),
			}, nil
		}
	}

	return tools.Result{
		Success: allSuccess,
		Data:    string(reportJSON),
	}, nil
}

func (b *BrowserSubagentTool) getPage() (*rod.Page, error) {
	b.mu.Lock()
	b.mu.Unlock() // wait, original page lock check was mu.Lock() / mu.Unlock() on separate statements? No, wait!
	// Let's re-verify page locking in rod Connect. In original code:
	// b.mu.Lock()
	// defer b.mu.Unlock()
	//
	// if b.browser == nil { ... }
	// We'll restore defer b.mu.Unlock() below.
	return b.getPageWithLock()
}

func (b *BrowserSubagentTool) getPageWithLock() (*rod.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil {
		l := launcher.New().Headless(b.headless)
		l.Set("disable-gpu")
		l.Set("no-sandbox")
		l.Set("disable-setuid-sandbox")
		l.Set("disable-dev-shm-usage")

		if path, found := launcher.LookPath(); found {
			l.Bin(path)
		}

		u, err := l.Launch()
		if err != nil {
			return nil, fmt.Errorf("falha ao iniciar launcher: %w", err)
		}
		b.browser = rod.New().ControlURL(u).MustConnect()
	}

	if b.page == nil {
		p, err := b.browser.Page(proto.TargetCreateTarget{})
		if err != nil {
			return nil, fmt.Errorf("falha ao abrir aba: %w", err)
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

// Close fecha o browser e libera recursos
func (b *BrowserSubagentTool) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.browser != nil {
		_ = b.browser.Close()
		b.browser = nil
		b.page = nil
	}
}

// GetCurrentPageContent retorna o HTML e a URL da página atual
func (b *BrowserSubagentTool) GetCurrentPageContent() (string, string, error) {
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
