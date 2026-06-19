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

// BrowserSubagentTool executa tarefas completas de navegação de forma autônoma,
// combinando múltiplas ações (navegar, clicar, digitar, verificar resultado via screenshot)
// em um único loop de autodecisão visual.
type BrowserSubagentTool struct {
	mu            sync.Mutex
	workspacePath string
	headless      bool
	browser       *rod.Browser
	page          *rod.Page
}

// NewBrowserSubagentTool cria uma nova instância da ferramenta de subagente de navegador
func NewBrowserSubagentTool(workspacePath string, headless bool) *BrowserSubagentTool {
	return &BrowserSubagentTool{
		workspacePath: workspacePath,
		headless:      headless,
	}
}

// ID retorna o identificador único da ferramenta
func (b *BrowserSubagentTool) ID() string {
	return "browser_subagent"
}

// Description descreve a ferramenta para o LLM
func (b *BrowserSubagentTool) Description() string {
	return "Executa uma sequência estruturada de ações no navegador com auto-verificação visual. " +
		"Cada ação pode ser: 'navigate' (navegar para URL), 'click' (clicar em seletor CSS), " +
		"'type' (digitar texto em seletor), 'wait' (aguardar N segundos), 'screenshot' (capturar e verificar resultado). " +
		"O subagente executa todas as ações em ordem e retorna um relatório estruturado com o resultado de cada etapa, " +
		"incluindo capturas de tela codificadas em base64 para validação visual."
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
	Step        int    `json:"step"`
	Action      string `json:"action"`
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	ScreenshotB64 string `json:"screenshot_b64,omitempty"` // presente apenas em etapas de screenshot
}

// Execute executa a sequência de etapas definidas no subagente
func (b *BrowserSubagentTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params subagentParams
	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	if len(params.Steps) == 0 {
		return Result{Success: false, Error: "nenhuma etapa definida em 'steps'"}, nil
	}

	page, err := b.getPage()
	if err != nil {
		return Result{Success: false, Error: "falha ao iniciar navegador: " + err.Error()}, nil
	}

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
				el, err := p.Element(step.Selector)
				if err != nil {
					sr.Success = false
					sr.Message = fmt.Sprintf("elemento %q não encontrado: %v", step.Selector, err)
				} else if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
					sr.Success = false
					sr.Message = fmt.Sprintf("falha ao clicar em %q: %v", step.Selector, err)
				} else {
					// Auto-verificação visual: tira screenshot após clique para confirmar resultado
					imgBytes, screenshotErr := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
					if screenshotErr == nil {
						sr.ScreenshotB64 = base64.StdEncoding.EncodeToString(imgBytes)
					}
					sr.Success = true
					sr.Message = fmt.Sprintf("Elemento %q clicado com sucesso", step.Selector)
				}
			}

		case "type":
			if step.Selector == "" || step.Text == "" {
				sr.Success = false
				sr.Message = "selector e text são obrigatórios para 'type'"
			} else {
				el, err := p.Element(step.Selector)
				if err != nil {
					sr.Success = false
					sr.Message = fmt.Sprintf("elemento %q não encontrado: %v", step.Selector, err)
				} else if err := el.Input(step.Text); err != nil {
					sr.Success = false
					sr.Message = fmt.Sprintf("falha ao digitar em %q: %v", step.Selector, err)
				} else {
					sr.Success = true
					sr.Message = fmt.Sprintf("Texto digitado com sucesso em %q", step.Selector)
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
				if len(html) > 2000 {
					html = html[:2000] + "...[truncado]"
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
					targetFile, pathErr := ValidatePath(b.workspacePath, step.Path, false)
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
			return Result{
				Success: allSuccess,
				Data:    "image:base64:" + r.ScreenshotB64 + "\n" + string(reportJSON),
			}, nil
		}
	}

	return Result{
		Success: allSuccess,
		Data:    string(reportJSON),
	}, nil
}

func (b *BrowserSubagentTool) getPage() (*rod.Page, error) {
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
