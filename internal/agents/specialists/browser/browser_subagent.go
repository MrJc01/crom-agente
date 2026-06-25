package browser

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

	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/llm"
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

	// Auto-registro global do agente especialista nativo
	agents.RegisterAgent("browser", func(cfg agents.Config) core.Agent {
		return NewBrowserAgent(cfg)
	})
}

// BrowserAgent executa tarefas completas de navegação de forma autônoma
type BrowserAgent struct {
	core.BaseAgent
	mu            sync.Mutex
	workspacePath string
	headless      bool
	browser       *rod.Browser
	page          *rod.Page
	onNavigate    func(url string)
	restoreURL    func() string
}

// NewBrowserAgent cria uma nova instância do especialista em browser
func NewBrowserAgent(cfg agents.Config) *BrowserAgent {
	ba := &BrowserAgent{
		workspacePath: cfg.WorkspacePath,
		headless:      cfg.BrowserHeadless,
	}
	ba.AgentName = "browser"
	ba.AgentDescription = metadata.Description
	ba.LLMProvider = cfg.LLMProvider
	ba.AllowedToolIDs = []string{"scraper", "http_client"}
	return ba
}

// Name retorna o nome do especialista
func (b *BrowserAgent) Name() string {
	return b.AgentName
}

// Description retorna a descrição
func (b *BrowserAgent) Description() string {
	return b.AgentDescription
}

// SystemPrompt retorna o prompt
func (b *BrowserAgent) SystemPrompt() string {
	return b.AgentSysPrompt
}

// SetOnNavigate define callback de navegação
func (b *BrowserAgent) SetOnNavigate(cb func(url string)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onNavigate = cb
}

// SetRestoreURL define callback de restauração de URL
func (b *BrowserAgent) SetRestoreURL(cb func() string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.restoreURL = cb
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

type StepResult struct {
	Step          int    `json:"step"`
	Action        string `json:"action"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ScreenshotB64 string `json:"screenshot_b64,omitempty"`
}

// Execute executa o subagente compilando o prompt em passos estruturados
func (b *BrowserAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	// 1. Perguntar ao LLM quais os passos (steps) necessários baseados na tarefa e memória anterior
	systemMsg := `Você é o Especialista de Navegação Web do CROM-Agente.
Sua tarefa é receber um prompt de navegação técnica e o histórico anterior, e traduzi-los em uma sequência JSON estruturada de etapas a serem executadas pelo robô do navegador.
Retorne APENAS um array JSON de etapas, sem blocos markdown extras ou explicações.

As ações suportadas na propriedade "action" são:
- "navigate": requer "url"
- "click": requer "selector"
- "type": requer "selector" e "text"
- "wait": requer "seconds"
- "scroll": rola a página
- "get_html": pega o HTML da página
- "screenshot": opcionalmente requer "path"

Exemplo de retorno esperado:
[
  {"action": "navigate", "url": "https://example.com"},
  {"action": "click", "selector": "#login-btn"}
]`

	userMsg := fmt.Sprintf("Tarefa de navegação: %s\nHistórico anterior: %s", prompt, priorSummary)

	var steps []subagentStep

	// Se o prompt for JSON contendo os passos diretamente, faz o parse deles
	trimmedPrompt := strings.TrimSpace(prompt)
	if strings.HasPrefix(trimmedPrompt, "[") || strings.HasPrefix(trimmedPrompt, "{") {
		var parseError error
		if strings.HasPrefix(trimmedPrompt, "[") {
			parseError = json.Unmarshal([]byte(trimmedPrompt), &steps)
		} else {
			var wrapper struct {
				Steps []subagentStep `json:"steps"`
			}
			if parseError = json.Unmarshal([]byte(trimmedPrompt), &wrapper); parseError == nil {
				steps = wrapper.Steps
			}
		}
	}

	if len(steps) == 0 && b.LLMProvider != nil {
		resp, err := b.LLMProvider.SendMessages(ctx, []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		}, llm.RequestOptions{})
		if err == nil && resp != nil && resp.Message.Content != "" {
			content := resp.Message.Content
			// Limpa tags de código markdown
			if idxStart := strings.Index(content, "["); idxStart != -1 {
				if idxEnd := strings.LastIndex(content, "]"); idxEnd != -1 && idxEnd > idxStart {
					content = content[idxStart : idxEnd+1]
				}
			}
			_ = json.Unmarshal([]byte(content), &steps)
		}
	}

	// Fallback se não gerou nenhum passo
	if len(steps) == 0 {
		url := "https://google.com"
		if strings.Contains(prompt, "http://") || strings.Contains(prompt, "https://") {
			fields := strings.Fields(prompt)
			for _, f := range fields {
				if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
					url = f
					break
				}
			}
		}
		steps = []subagentStep{
			{Action: "navigate", URL: url},
			{Action: "screenshot"},
		}
	}

	// 2. Executa a sequência de passos no Rod
	res, err := b.executeSteps(ctx, steps)
	if err != nil {
		return core.AgentResult{}, err
	}

	// 3. Compacta o histórico para retornar
	newSummary, _ := core.CompressHistory(ctx, b.LLMProvider, prompt, res.Data, priorSummary)

	return core.AgentResult{
		Success:        res.Success,
		Output:         res.Data,
		ContextSummary: newSummary,
	}, nil
}

func (b *BrowserAgent) executeSteps(ctx context.Context, steps []subagentStep) (tools.Result, error) {
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

	for i, step := range steps {
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
			_, _ = p.Eval(`window.scrollTo(0, document.body.scrollHeight)`)
			sr.Success = true
			sr.Message = "Scroll realizado até o final da página"

		case "get_html":
			html, err := p.HTML()
			if err != nil {
				sr.Success = false
				sr.Message = fmt.Sprintf("falha ao obter HTML: %v", err)
			} else {
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

	reportJSON, err := json.MarshalIndent(map[string]interface{}{
		"total_steps": len(steps),
		"all_success": allSuccess,
		"results":     results,
	}, "", "  ")
	if err != nil {
		reportJSON = []byte(`{"error": "falha ao serializar relatório"}`)
	}

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

func (b *BrowserAgent) getPage() (*rod.Page, error) {
	return b.getPageWithLock()
}

func (b *BrowserAgent) getPageWithLock() (*rod.Page, error) {
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
func (b *BrowserAgent) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.browser != nil {
		_ = b.browser.Close()
		b.browser = nil
		b.page = nil
	}
}

// GetCurrentPageContent retorna o HTML e a URL da página atual
func (b *BrowserAgent) GetCurrentPageContent() (string, string, error) {
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
