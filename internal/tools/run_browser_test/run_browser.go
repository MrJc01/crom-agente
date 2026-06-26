package run_browser_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
		panic("falha ao carregar metadados de run_browser_test: " + err.Error())
	}
}

// RunBrowserTestTool executa testes visuais e E2E usando go-rod
type RunBrowserTestTool struct {
	workspaceRoot string
	headless      bool
}

// NewRunBrowserTestTool cria a nova ferramenta
func NewRunBrowserTestTool(workspaceRoot string, headless bool) *RunBrowserTestTool {
	return &RunBrowserTestTool{
		workspaceRoot: workspaceRoot,
		headless:      headless,
	}
}

func (t *RunBrowserTestTool) ID() string {
	return metadata.ID
}

func (t *RunBrowserTestTool) Description() string {
	return metadata.Description
}

func (t *RunBrowserTestTool) RequiresApproval() bool {
	return false
}

// Input structures
type TestStep struct {
	Action         string  `json:"action"` // navigate, click, type, hover, double_click, drag_and_drop, scroll, wait, assert_visible, assert_text, check_accessibility, screenshot
	URL            string  `json:"url,omitempty"`
	Selector       string  `json:"selector,omitempty"`
	TargetSelector string  `json:"target_selector,omitempty"` // For drag_and_drop
	Text           string  `json:"text,omitempty"`            // For type, assert_text
	Seconds        float64 `json:"seconds,omitempty"`         // For wait
	Viewport       string  `json:"viewport,omitempty"`        // mobile, tablet, desktop
}

type RunBrowserTestInput struct {
	Playbook      string     `json:"playbook,omitempty"`
	Steps         []TestStep `json:"steps,omitempty"`
	Viewport      string     `json:"viewport,omitempty"`       // mobile, tablet, desktop, all
	ScreenshotDir string     `json:"screenshot_dir,omitempty"` // Directory to save sequence of screenshots
	BaselinePath  string     `json:"baseline_path,omitempty"`  // Path of the baseline image to compare
}

type StepResult struct {
	Index         int      `json:"index"`
	Action        string   `json:"action"`
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	ScreenshotB64 string   `json:"screenshot_b64,omitempty"`
	Accessibility []string `json:"accessibility,omitempty"`
}

type TestReport struct {
	TotalSteps     int          `json:"total_steps"`
	Success        bool         `json:"success"`
	ConsoleLogs    []string     `json:"console_logs"`
	NetworkErrors  []string     `json:"network_errors"`
	StepResults    []StepResult `json:"step_results"`
	DiffSimilarity float64      `json:"diff_similarity,omitempty"`
	DiffMessage    string       `json:"diff_message,omitempty"`
}

func (t *RunBrowserTestTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"playbook": {
				"type": "string",
				"enum": ["login", "signup", "checkout"],
				"description": "Playbook E2E predefinido a ser executado"
			},
			"steps": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"action": {
							"type": "string",
							"enum": ["navigate", "click", "type", "hover", "double_click", "drag_and_drop", "scroll", "wait", "assert_visible", "assert_text", "check_accessibility", "screenshot"]
						},
						"url": { "type": "string" },
						"selector": { "type": "string" },
						"target_selector": { "type": "string" },
						"text": { "type": "string" },
						"seconds": { "type": "number" },
						"viewport": { "type": "string", "enum": ["mobile", "tablet", "desktop"] }
					},
					"required": ["action"]
				}
			},
			"viewport": {
				"type": "string",
				"enum": ["mobile", "tablet", "desktop", "all"],
				"default": "desktop"
			},
			"screenshot_dir": {
				"type": "string",
				"description": "Diretório opcional dentro do workspace para salvar a sequência de prints (Task 26)"
			},
			"baseline_path": {
				"type": "string",
				"description": "Caminho da imagem base para comparação de regressão visual (Task 28)"
			}
		}
	}`)
}

func (t *RunBrowserTestTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input RunBrowserTestInput
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	// Se for informado um playbook predefinido, expande os passos
	if input.Playbook != "" {
		input.Steps = getPlaybookSteps(input.Playbook)
	}

	if len(input.Steps) == 0 {
		return tools.Result{Success: false, Error: "nenhuma etapa informada para execução do teste de browser"}, nil
	}

	// Configuração do launcher
	l := launcher.New().Headless(t.headless)
	l.Set("disable-gpu")
	l.Set("no-sandbox")
	l.Set("disable-setuid-sandbox")
	l.Set("disable-dev-shm-usage")

	if path, found := launcher.LookPath(); found {
		l.Bin(path)
	}

	u, err := l.Launch()
	if err != nil {
		return tools.Result{Success: false, Error: "falha ao abrir navegador: " + err.Error()}, nil
	}

	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.Close()

	page := browser.MustPage()

	// Coletores de eventos de console e rede (Task 22 e 24)
	var consoleMu sync.Mutex
	var consoleLogs []string
	var networkErrors []string

	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		consoleMu.Lock()
		defer consoleMu.Unlock()
		var args []string
		for _, arg := range e.Args {
			args = append(args, fmt.Sprintf("%v", arg.Value))
		}
		consoleLogs = append(consoleLogs, fmt.Sprintf("[%s] %s", e.Type, strings.Join(args, " ")))
	})()

	go page.EachEvent(func(e *proto.NetworkResponseReceived) {
		if e.Response.Status >= 400 {
			consoleMu.Lock()
			defer consoleMu.Unlock()
			networkErrors = append(networkErrors, fmt.Sprintf("HTTP %d para %s", e.Response.Status, e.Response.URL))
		}
	})()

	// Configura screenshot_dir se informado
	var saveDir string
	if input.ScreenshotDir != "" {
		var pathErr error
		saveDir, pathErr = tools.ValidatePath(t.workspaceRoot, input.ScreenshotDir, false)
		if pathErr == nil {
			_ = os.MkdirAll(saveDir, 0755)
		}
	}

	// Executa os passos
	var stepResults []StepResult
	allSuccess := true

	for idx, step := range input.Steps {
		sr := StepResult{
			Index:  idx + 1,
			Action: step.Action,
		}

		// Configura viewport do passo (Task 25)
		vp := step.Viewport
		if vp == "" {
			vp = input.Viewport
		}
		setViewport(page, vp)

		// Executa ação específica
		stepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		errAction := t.executeStepAction(stepCtx, page, step, &sr)
		cancel()

		if errAction != nil {
			sr.Success = false
			sr.Message = errAction.Error()
			allSuccess = false
		} else {
			sr.Success = true
		}

		// Captura screenshot (Task 22/26)
		imgBytes, errScr := page.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
		if errScr == nil {
			sr.ScreenshotB64 = base64.StdEncoding.EncodeToString(imgBytes)
			if saveDir != "" {
				scrPath := filepath.Join(saveDir, fmt.Sprintf("step_%02d_%s.png", idx+1, step.Action))
				_ = os.WriteFile(scrPath, imgBytes, 0644)
				sr.Message += fmt.Sprintf(" (Screenshot salvo em: %s)", scrPath)
			}
		}

		stepResults = append(stepResults, sr)
		if !sr.Success {
			break
		}
	}

	report := TestReport{
		TotalSteps:    len(input.Steps),
		Success:       allSuccess,
		StepResults:   stepResults,
		ConsoleLogs:   consoleLogs,
		NetworkErrors: networkErrors,
	}

	// Diferenciação Visual (Task 28)
	if allSuccess && input.BaselinePath != "" && len(stepResults) > 0 {
		baselineFile, errBase := tools.ValidatePath(t.workspaceRoot, input.BaselinePath, false)
		if errBase == nil {
			baseBytes, errRead := os.ReadFile(baselineFile)
			if errRead == nil && len(stepResults[len(stepResults)-1].ScreenshotB64) > 0 {
				currBytes, _ := base64.StdEncoding.DecodeString(stepResults[len(stepResults)-1].ScreenshotB64)
				similarity, errComp := compareImages(baseBytes, currBytes)
				if errComp == nil {
					report.DiffSimilarity = similarity
					report.DiffMessage = fmt.Sprintf("Similiaridade com baseline visual: %.2f%%", similarity)
				} else {
					report.DiffMessage = "Falha ao comparar imagens: " + errComp.Error()
				}
			}
		}
	}

	reportJSON, _ := json.MarshalIndent(report, "", "  ")

	// Se houver screenshot no último passo e for solicitado, podemos incluir a representação visual direta
	dataOut := string(reportJSON)
	if len(stepResults) > 0 && stepResults[len(stepResults)-1].ScreenshotB64 != "" {
		dataOut = "image:base64:" + stepResults[len(stepResults)-1].ScreenshotB64 + "\n" + dataOut
	}

	return tools.Result{
		Success: allSuccess,
		Data:    dataOut,
	}, nil
}

func setViewport(page *rod.Page, vp string) {
	width, height := 1280, 800 // Default desktop
	switch strings.ToLower(vp) {
	case "mobile":
		width, height = 375, 812
	case "tablet":
		width, height = 768, 1024
	case "desktop":
		width, height = 1920, 1080
	}
	_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  width,
		Height: height,
	})
}

func (t *RunBrowserTestTool) executeStepAction(ctx context.Context, page *rod.Page, step TestStep, sr *StepResult) error {
	p := page.Context(ctx)

	switch step.Action {
	case "navigate":
		if step.URL == "" {
			return fmt.Errorf("URL ausente para 'navigate'")
		}
		if err := p.Navigate(step.URL); err != nil {
			return fmt.Errorf("navegação falhou: %w", err)
		}
		_ = p.WaitLoad()
		sr.Message = "Navegação concluída com sucesso"

	case "click":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'click'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não encontrado: %w", step.Selector, err)
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return fmt.Errorf("clique falhou: %w", err)
		}
		time.Sleep(500 * time.Millisecond) // Espera transição/render
		sr.Message = "Elemento clicado com sucesso"

	case "type":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'type'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não encontrado: %w", step.Selector, err)
		}
		if err := el.Input(step.Text); err != nil {
			return fmt.Errorf("digitação falhou: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
		sr.Message = "Texto digitado com sucesso"

	case "hover":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'hover'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não encontrado: %w", step.Selector, err)
		}
		if err := el.Hover(); err != nil {
			return fmt.Errorf("hover falhou: %w", err)
		}
		time.Sleep(300 * time.Millisecond)
		sr.Message = "Hover simulado com sucesso"

	case "double_click":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'double_click'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não encontrado: %w", step.Selector, err)
		}
		if err := el.Click(proto.InputMouseButtonLeft, 2); err != nil {
			return fmt.Errorf("clique duplo falhou: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
		sr.Message = "Clique duplo realizado"

	case "drag_and_drop":
		if step.Selector == "" || step.TargetSelector == "" {
			return fmt.Errorf("seletor de origem ou destino ausente para 'drag_and_drop'")
		}
		elSrc, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento de origem %q não encontrado: %w", step.Selector, err)
		}
		elDst, err := p.Element(step.TargetSelector)
		if err != nil {
			return fmt.Errorf("elemento de destino %q não encontrado: %w", step.TargetSelector, err)
		}
		_, errDrag := elSrc.Eval(`(src, dst) => {
			const dataTransfer = new DataTransfer();
			const dragStartEvent = new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer });
			src.dispatchEvent(dragStartEvent);
			const dropEvent = new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer });
			dst.dispatchEvent(dropEvent);
			const dragEndEvent = new DragEvent('dragend', { bubbles: true, cancelable: true, dataTransfer });
			src.dispatchEvent(dragEndEvent);
		}`, elDst)
		if errDrag != nil {
			return fmt.Errorf("drag and drop falhou: %w", errDrag)
		}
		time.Sleep(500 * time.Millisecond)
		sr.Message = "Drag and drop simulado"

	case "scroll":
		_, err := p.Eval(`window.scrollTo(0, document.body.scrollHeight)`)
		if err != nil {
			return fmt.Errorf("scroll falhou: %w", err)
		}
		sr.Message = "Página rolada até o fim"

	case "wait":
		secs := step.Seconds
		if secs <= 0 {
			secs = 1
		}
		time.Sleep(time.Duration(secs * float64(time.Second)))
		sr.Message = fmt.Sprintf("Aguardou por %.1f segundos", secs)

	case "assert_visible":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'assert_visible'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não existe: %w", step.Selector, err)
		}
		vis, err := el.Visible()
		if err != nil || !vis {
			return fmt.Errorf("elemento %q não está visível", step.Selector)
		}
		sr.Message = fmt.Sprintf("Assert: elemento %q está visível", step.Selector)

	case "assert_text":
		if step.Selector == "" {
			return fmt.Errorf("seletor ausente para 'assert_text'")
		}
		el, err := p.Element(step.Selector)
		if err != nil {
			return fmt.Errorf("elemento %q não encontrado: %w", step.Selector, err)
		}
		txt, err := el.Text()
		if err != nil {
			return fmt.Errorf("falha ao ler texto de %q: %w", step.Selector, err)
		}
		if !strings.Contains(txt, step.Text) {
			return fmt.Errorf("texto esperado %q não encontrado, obteve: %q", step.Text, txt)
		}
		sr.Message = fmt.Sprintf("Assert: texto %q presente no elemento %q", step.Text, step.Selector)

	case "check_accessibility":
		res, err := p.Eval(accessibilityAuditJS)
		if err != nil {
			return fmt.Errorf("auditoria de acessibilidade falhou: %w", err)
		}
		var issues []string
		for _, issue := range res.Value.Arr() {
			issues = append(issues, issue.Str())
		}
		sr.Accessibility = issues
		if len(issues) > 0 {
			sr.Message = fmt.Sprintf("Acessibilidade: detectados %d problemas", len(issues))
		} else {
			sr.Message = "Acessibilidade: nenhum problema simples detectado"
		}

	case "screenshot":
		sr.Message = "Screenshot capturado explicitamente"

	default:
		return fmt.Errorf("ação desconhecida: %s", step.Action)
	}

	return nil
}

const accessibilityAuditJS = `() => {
	const issues = [];
	
	// 1. Imagens sem alt
	document.querySelectorAll('img').forEach(img => {
		if (!img.hasAttribute('alt') || img.getAttribute('alt').trim() === '') {
			issues.push('Imagem sem atributo alt: ' + img.src);
		}
	});

	// 2. Inputs sem label/aria-label
	document.querySelectorAll('input, select, textarea').forEach(input => {
		if (input.type === 'submit' || input.type === 'button' || input.type === 'hidden') return;
		let hasLabel = false;
		if (input.hasAttribute('aria-label') || input.hasAttribute('aria-labelledby')) {
			hasLabel = true;
		} else {
			if (input.id) {
				const label = document.querySelector('label[for="' + input.id + '"]');
				if (label && label.innerText.trim() !== '') {
					hasLabel = true;
				}
			}
			if (!hasLabel) {
				const parentLabel = input.closest('label');
				if (parentLabel && parentLabel.innerText.trim() !== '') {
					hasLabel = true;
				}
			}
		}
		if (!hasLabel) {
			issues.push('Elemento de entrada (' + input.tagName.toLowerCase() + ') sem label ou aria-label: ID=' + input.id + ' Name=' + input.name);
		}
	});

	// 3. Botões sem texto/aria-label
	document.querySelectorAll('button').forEach(btn => {
		const text = btn.innerText.trim();
		const hasAria = btn.hasAttribute('aria-label') || btn.hasAttribute('aria-labelledby');
		if (text === '' && !hasAria) {
			issues.push('Botão vazio sem aria-label ou texto descritivo');
		}
	});

	return issues;
};`

func getPlaybookSteps(playbook string) []TestStep {
	switch strings.ToLower(playbook) {
	case "login":
		return []TestStep{
			{Action: "navigate", URL: "http://localhost:8080/login"},
			{Action: "type", Selector: "#email", Text: "user@example.com"},
			{Action: "type", Selector: "#password", Text: "password123"},
			{Action: "click", Selector: "button[type='submit']"},
			{Action: "assert_visible", Selector: "#dashboard"},
		}
	case "signup":
		return []TestStep{
			{Action: "navigate", URL: "http://localhost:8080/signup"},
			{Action: "type", Selector: "#name", Text: "John Doe"},
			{Action: "type", Selector: "#email", Text: "newuser@example.com"},
			{Action: "type", Selector: "#password", Text: "password123"},
			{Action: "click", Selector: "button[type='submit']"},
			{Action: "assert_text", Selector: ".welcome-msg", Text: "Bem-vindo"},
		}
	case "checkout":
		return []TestStep{
			{Action: "navigate", URL: "http://localhost:8080/cart"},
			{Action: "click", Selector: "#btn-checkout"},
			{Action: "type", Selector: "#address", Text: "Rua das Flores, 123"},
			{Action: "click", Selector: "#btn-confirm-order"},
			{Action: "assert_text", Selector: "#order-status", Text: "Confirmado"},
		}
	}
	return nil
}

func compareImages(img1Bytes, img2Bytes []byte) (float64, error) {
	img1, _, err := image.Decode(bytes.NewReader(img1Bytes))
	if err != nil {
		return 0, fmt.Errorf("erro ao decodificar imagem 1: %w", err)
	}
	img2, _, err := image.Decode(bytes.NewReader(img2Bytes))
	if err != nil {
		return 0, fmt.Errorf("erro ao decodificar imagem 2: %w", err)
	}

	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()

	if bounds1.Dx() != bounds2.Dx() || bounds1.Dy() != bounds2.Dy() {
		return 0, fmt.Errorf("dimensões diferentes: %dx%d vs %dx%d", bounds1.Dx(), bounds1.Dy(), bounds2.Dx(), bounds2.Dy())
	}

	diffCount := 0
	totalPixels := bounds1.Dx() * bounds1.Dy()

	for y := bounds1.Min.Y; y < bounds1.Max.Y; y++ {
		for x := bounds1.Min.X; x < bounds1.Max.X; x++ {
			r1, g1, b1, a1 := img1.At(x, y).RGBA()
			r2, g2, b2, a2 := img2.At(x, y).RGBA()

			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diffCount++
			}
		}
	}

	similarity := float64(totalPixels-diffCount) / float64(totalPixels) * 100
	return similarity, nil
}
