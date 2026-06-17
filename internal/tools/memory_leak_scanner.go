package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// MemoryLeakScannerTool injeta pprof e analisa goroutines/heap para detectar vazamentos
type MemoryLeakScannerTool struct {
	workspaceRoot string
	jail          bool
}

// NewMemoryLeakScannerTool cria uma instância do scanner de vazamentos de memória
func NewMemoryLeakScannerTool(workspaceRoot string, jail bool) *MemoryLeakScannerTool {
	return &MemoryLeakScannerTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *MemoryLeakScannerTool) ID() string             { return "memory_leak_scanner" }
func (t *MemoryLeakScannerTool) Description() string     { return "Analisa código Go para padrões de vazamento de memória (goroutines órfãs, channels não fechados) e opcionalmente coleta perfil de runtime." }
func (t *MemoryLeakScannerTool) RequiresApproval() bool  { return true }

func (t *MemoryLeakScannerTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo ou diretório Go para analisar estaticamente"
			},
			"runtime_profile": {
				"type": "boolean",
				"description": "Se true, coleta perfil de runtime do processo atual (goroutines e heap)"
			},
			"pprof_url": {
				"type": "string",
				"description": "URL do servidor pprof para coletar perfil remoto (ex: http://localhost:6060)"
			}
		},
		"required": ["path"]
	}`)
}

// LeakPattern representa um padrão de potencial vazamento detectado
type LeakPattern struct {
	File       string
	Line       int
	Type       string // goroutine_leak, unclosed_channel, unclosed_resource, ticker_leak, etc.
	Severity   string // high, medium, low
	Message    string
	Suggestion string
}

func (t *MemoryLeakScannerTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var input struct {
		Path           string `json:"path"`
		RuntimeProfile bool   `json:"runtime_profile"`
		PprofURL       string `json:"pprof_url"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetPath, err := ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return Result{Success: false, Error: err.Error()}, nil
	}

	var sb strings.Builder
	sb.WriteString("# 🔍 Relatório de Análise de Vazamentos de Memória\n\n")

	// 1. Análise estática de código
	sb.WriteString("## 📝 Análise Estática\n\n")
	patterns, err := staticLeakAnalysis(targetPath, input.Path)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro na análise: %s", err.Error())}, nil
	}

	if len(patterns) == 0 {
		sb.WriteString("✅ Nenhum padrão de vazamento detectado na análise estática.\n\n")
	} else {
		// Agrupar por severidade
		high, medium, low := 0, 0, 0
		for _, p := range patterns {
			switch p.Severity {
			case "high":
				high++
			case "medium":
				medium++
			case "low":
				low++
			}
		}

		sb.WriteString(fmt.Sprintf("| Severidade | Contagem |\n"))
		sb.WriteString(fmt.Sprintf("|-----------|----------|\n"))
		sb.WriteString(fmt.Sprintf("| 🔴 Alta | %d |\n", high))
		sb.WriteString(fmt.Sprintf("| 🟡 Média | %d |\n", medium))
		sb.WriteString(fmt.Sprintf("| 🟢 Baixa | %d |\n", low))
		sb.WriteString("\n")

		for _, p := range patterns {
			icon := "🟢"
			switch p.Severity {
			case "high":
				icon = "🔴"
			case "medium":
				icon = "🟡"
			}

			sb.WriteString(fmt.Sprintf("### %s %s (linha %d)\n", icon, p.Type, p.Line))
			sb.WriteString(fmt.Sprintf("**Arquivo:** `%s`\n\n", p.File))
			sb.WriteString(fmt.Sprintf("**Problema:** %s\n\n", p.Message))
			sb.WriteString(fmt.Sprintf("**Sugestão:** %s\n\n", p.Suggestion))
			sb.WriteString("---\n\n")
		}
	}

	// 2. Perfil de runtime
	if input.RuntimeProfile {
		sb.WriteString("## 🏃 Perfil de Runtime\n\n")
		writeRuntimeProfile(&sb)
	}

	// 3. Perfil remoto via pprof
	if input.PprofURL != "" {
		sb.WriteString("## 🌐 Perfil Remoto (pprof)\n\n")
		writeRemoteProfile(&sb, input.PprofURL)
	}

	return Result{Success: true, Data: sb.String()}, nil
}

// staticLeakAnalysis faz análise estática de um arquivo Go para padrões de vazamento
func staticLeakAnalysis(filePath string, displayPath string) ([]LeakPattern, error) {
	// Verificar se é diretório ou arquivo
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		var allPatterns []LeakPattern
		entries, _ := os.ReadDir(filePath)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
				fullPath := filepath.Join(filePath, e.Name())
				display := filepath.Join(displayPath, e.Name())
				patterns, err := analyzeGoFileForLeaks(fullPath, display)
				if err != nil {
					continue
				}
				allPatterns = append(allPatterns, patterns...)
			}
		}
		return allPatterns, nil
	}

	return analyzeGoFileForLeaks(filePath, displayPath)
}

func analyzeGoFileForLeaks(filePath string, displayPath string) ([]LeakPattern, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var patterns []LeakPattern

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}

		funcPatterns := analyzeFunctionForLeaks(fset, funcDecl, displayPath)
		patterns = append(patterns, funcPatterns...)
	}

	return patterns, nil
}

func analyzeFunctionForLeaks(fset *token.FileSet, funcDecl *ast.FuncDecl, file string) []LeakPattern {
	var patterns []LeakPattern

	// Rastrear variáveis relevantes
	hasGoStmt := false
	hasDoneChannel := false
	hasContextCancel := false
	hasTickerStop := false
	goStmtLines := []int{}

	// Contadores para detecção de padrões
	channelMakes := 0
	channelCloses := 0

	// Verificar se algum parâmetro da função é um context
	if funcDecl.Type.Params != nil {
		for _, param := range funcDecl.Type.Params.List {
			paramType := exprToString(param.Type)
			if strings.Contains(paramType, "Context") || strings.Contains(paramType, "context.Context") {
				hasContextCancel = true
			}
			for _, name := range param.Names {
				if name.Name == "ctx" || strings.Contains(strings.ToLower(name.Name), "context") {
					hasContextCancel = true
				}
			}
		}
	}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GoStmt:
			hasGoStmt = true
			goStmtLines = append(goStmtLines, fset.Position(node.Pos()).Line)

		case *ast.SelectorExpr:
			// Detectar ctx.Done() ou similar
			if ident, ok := node.X.(*ast.Ident); ok {
				if (ident.Name == "ctx" || strings.Contains(strings.ToLower(ident.Name), "context")) && node.Sel.Name == "Done" {
					hasContextCancel = true
				}
			}

		case *ast.CallExpr:
			funcName := callExprName(node)

			// Detectar make(chan ...)
			if funcName == "make" && len(node.Args) > 0 {
				if _, ok := node.Args[0].(*ast.ChanType); ok {
					channelMakes++
				}
			}

			// Detectar close(ch)
			if funcName == "close" {
				channelCloses++
			}

			// Detectar context.WithCancel/WithTimeout
			if strings.Contains(funcName, "WithCancel") || strings.Contains(funcName, "WithTimeout") || strings.Contains(funcName, "WithDeadline") {
				hasContextCancel = true
			}

			// Detectar ticker.Stop()
			if strings.HasSuffix(funcName, "Stop") {
				hasTickerStop = true
			}

		case *ast.AssignStmt:
			// Detectar done := make(chan ...)
			for _, rhs := range node.Rhs {
				if call, ok := rhs.(*ast.CallExpr); ok {
					if callExprName(call) == "make" && len(call.Args) > 0 {
						if _, ok := call.Args[0].(*ast.ChanType); ok {
							for _, lhs := range node.Lhs {
								if ident, ok := lhs.(*ast.Ident); ok {
									if ident.Name == "done" || strings.Contains(ident.Name, "Done") {
										hasDoneChannel = true
									}
								}
							}
						}
					}
				}
			}

		case *ast.DeferStmt:
			// Verificar defer cancel()
			if call, ok := node.Call.Fun.(*ast.Ident); ok {
				if call.Name == "cancel" {
					hasContextCancel = true
				}
			}
			// Verificar defer ticker.Stop()
			if sel, ok := node.Call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Stop" {
					hasTickerStop = true
				}
			}
		}
		return true
	})

	// Detectar goroutine sem canal de controle (done/context)
	if hasGoStmt && !hasDoneChannel && !hasContextCancel {
		for _, line := range goStmtLines {
			patterns = append(patterns, LeakPattern{
				File:       file,
				Line:       line,
				Type:       "goroutine_leak",
				Severity:   "high",
				Message:    "Goroutine lançada sem canal 'done' ou context cancelável — pode ficar órfã.",
				Suggestion: "Use context.WithCancel() e propague o ctx.Done() para a goroutine, ou crie um canal 'done' com close(done) para sinalizar encerramento.",
			})
		}
	}

	// Detectar channels criados sem close correspondente
	if channelMakes > channelCloses && channelMakes > 0 {
		patterns = append(patterns, LeakPattern{
			File:       file,
			Line:       fset.Position(funcDecl.Pos()).Line,
			Type:       "unclosed_channel",
			Severity:   "medium",
			Message:    fmt.Sprintf("%d channels criados mas apenas %d close() encontrados na função '%s'.", channelMakes, channelCloses, funcDecl.Name.Name),
			Suggestion: "Garanta que todos os channels produtores sejam fechados (close) quando não mais necessários, ou use defer close(ch).",
		})
	}

	// Detectar time.NewTicker sem Stop
	detectTickerLeak(fset, funcDecl, file, &patterns, hasTickerStop)

	return patterns
}

func detectTickerLeak(fset *token.FileSet, funcDecl *ast.FuncDecl, file string, patterns *[]LeakPattern, hasStop bool) {
	hasTicker := false
	tickerLine := 0

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			name := callExprName(call)
			if name == "time.NewTicker" || name == "NewTicker" {
				hasTicker = true
				tickerLine = fset.Position(call.Pos()).Line
			}
		}
		return true
	})

	if hasTicker && !hasStop {
		*patterns = append(*patterns, LeakPattern{
			File:       file,
			Line:       tickerLine,
			Type:       "ticker_leak",
			Severity:   "medium",
			Message:    "time.NewTicker criado sem correspondente ticker.Stop() — goroutine interna do ticker nunca será coletada.",
			Suggestion: "Use defer ticker.Stop() imediatamente após a criação do ticker.",
		})
	}
}

func callExprName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if ident, ok := fn.X.(*ast.Ident); ok {
			return ident.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

// writeRuntimeProfile coleta e escreve perfil de runtime do processo atual
func writeRuntimeProfile(sb *strings.Builder) {
	// Goroutines
	numGoroutines := runtime.NumGoroutine()
	sb.WriteString(fmt.Sprintf("**Goroutines ativas:** %d\n\n", numGoroutines))

	if numGoroutines > 100 {
		sb.WriteString("⚠️ **Alerta:** Número elevado de goroutines! Possível vazamento.\n\n")
	}

	// Memória
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	sb.WriteString("| Métrica | Valor |\n")
	sb.WriteString("|---------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Alloc (heap atual) | %.2f MB |\n", float64(m.Alloc)/1024/1024))
	sb.WriteString(fmt.Sprintf("| TotalAlloc (acumulado) | %.2f MB |\n", float64(m.TotalAlloc)/1024/1024))
	sb.WriteString(fmt.Sprintf("| Sys (sistema) | %.2f MB |\n", float64(m.Sys)/1024/1024))
	sb.WriteString(fmt.Sprintf("| HeapObjects | %d |\n", m.HeapObjects))
	sb.WriteString(fmt.Sprintf("| NumGC | %d |\n", m.NumGC))
	sb.WriteString(fmt.Sprintf("| GCCPUFraction | %.4f%% |\n", m.GCCPUFraction*100))
	sb.WriteString("\n")

	if m.HeapObjects > 1000000 {
		sb.WriteString("⚠️ **Alerta:** Número elevado de objetos no heap! Possível fragmentação ou vazamento.\n\n")
	}
}

// writeRemoteProfile coleta perfil de um servidor pprof remoto
func writeRemoteProfile(sb *strings.Builder, pprofURL string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Goroutines
	resp, err := client.Get(pprofURL + "/debug/pprof/goroutine?debug=1")
	if err != nil {
		sb.WriteString(fmt.Sprintf("❌ Erro ao conectar ao pprof: %s\n\n", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		sb.WriteString("✅ Conexão com pprof estabelecida.\n\n")

		// Contar goroutines pelo header (simplificado)
		sb.WriteString(fmt.Sprintf("**Status HTTP:** %d\n", resp.StatusCode))
		sb.WriteString(fmt.Sprintf("**Content-Length:** %d bytes\n\n", resp.ContentLength))
		sb.WriteString("Para análise completa, use:\n")
		sb.WriteString(fmt.Sprintf("```bash\ngo tool pprof %s/debug/pprof/heap\ngo tool pprof %s/debug/pprof/goroutine\n```\n\n", pprofURL, pprofURL))
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ Servidor pprof retornou status %d\n\n", resp.StatusCode))
	}
}
