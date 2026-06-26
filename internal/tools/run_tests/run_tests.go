package run_tests

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de run_tests: " + err.Error())
	}
}

// RunTestsTool executa testes unitários/integração baseando-se na stack técnica detectada
type RunTestsTool struct {
	workspaceRoot string
}

// NewRunTestsTool cria a ferramenta run_tests
func NewRunTestsTool(workspaceRoot string) *RunTestsTool {
	return &RunTestsTool{workspaceRoot: workspaceRoot}
}

// ID retorna o identificador da ferramenta
func (t *RunTestsTool) ID() string {
	return metadata.ID
}

// Description retorna a descrição da ferramenta
func (t *RunTestsTool) Description() string {
	return metadata.Description
}

// ParametersSchema define os parâmetros aceitos via JSON
func (t *RunTestsTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Comando de teste customizado (se omitido, a ferramenta detecta automaticamente)"
			},
			"repeat": {
				"type": "integer",
				"description": "Número de vezes para repetir a execução do teste para detectar instabilidade/flakiness (padrão: 1)"
			}
		},
		"required": []
	}`)
}

// RequiresApproval — Executar testes exige aprovação HITL média
func (t *RunTestsTool) RequiresApproval() bool { return true }

// Execute executa os testes
func (t *RunTestsTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Command string `json:"command"`
		Repeat  int    `json:"repeat"`
	}
	_ = json.Unmarshal(args, &input)

	if input.Repeat <= 0 {
		input.Repeat = 1
	}

	cmdStr := strings.TrimSpace(input.Command)
	if cmdStr == "" {
		// Detecta comando baseado na stack
		cmdStr = t.detectTestCommand()
	}

	if cmdStr == "" {
		return tools.Result{Success: false, Error: "não foi possível detectar uma suite de testes automática no workspace. Por favor, forneça o comando explicitamente."}, nil
	}

	var finalOut strings.Builder

	for runIdx := 0; runIdx < input.Repeat; runIdx++ {
		res, err := func() (tools.Result, error) {
			// Executa em PTY
			cmdName, cmdArgs := tools.WrapCommandWithCgroup(cmdStr, 2048, 80) // 2GB memory e 80% cpu para testes
			
			// Identificação de Loops Infinitos de Teste (Timeout de 10 segundos por execução)
			runTimeout := 10 * time.Second
			runCtx, runCancel := context.WithTimeout(ctx, runTimeout)
			defer runCancel()
			
			c := exec.CommandContext(runCtx, cmdName, cmdArgs...)
			c.Dir = t.workspaceRoot

			// Start mock proxy to prevent hanging/timeout from external network requests (Task 16)
			proxy, proxyErr := StartMockProxy()
			if proxyErr == nil {
				defer proxy.Close()
				
				// Inject HTTP_PROXY and HTTPS_PROXY (and lowercase versions) into Env
				proxyURL := fmt.Sprintf("http://%s", proxy.Addr())
				env := os.Environ()
				env = append(env,
					"HTTP_PROXY="+proxyURL,
					"HTTPS_PROXY="+proxyURL,
					"http_proxy="+proxyURL,
					"https_proxy="+proxyURL,
				)
				c.Env = env
			}

			f, err := pty.Start(c)
			if err != nil {
				return tools.Result{Success: false, Error: fmt.Sprintf("erro ao iniciar runner de teste: %s", err)}, nil
			}
			defer f.Close()

			// Lê output
			outChan := make(chan string, 1)
			go func() {
				var sb strings.Builder
				buf := make([]byte, 1024)
				for {
					n, readErr := f.Read(buf)
					if n > 0 {
						sb.Write(buf[:n])
					}
					if readErr != nil {
						break
					}
				}
				outChan <- sb.String()
			}()

			var out string
			var timedOut bool

			select {
			case <-runCtx.Done():
				timedOut = true
				if c.Process != nil {
					_ = c.Process.Kill()
				}
			case out = <-outChan:
			}

			if timedOut {
				if ctx.Err() != nil {
					return tools.Result{Success: false, Error: "testes cancelados por interrupção ou timeout do usuário"}, ctx.Err()
				}
				return tools.Result{
					Success: false,
					Error:   fmt.Sprintf("timeout: a execução do teste (rodada %d/%d) ultrapassou o limite seguro de 10 segundos (loop infinito detectado)", runIdx+1, input.Repeat),
				}, nil
			}

			_ = c.Wait()
			success := c.ProcessState.Success()

			// Validador de Saída Vazia de Testes (Task 18)
			lowerOut := strings.ToLower(out)
			isEmpty := strings.Contains(lowerOut, "no tests ran") ||
				strings.Contains(lowerOut, "collected 0 items") ||
				strings.Contains(lowerOut, "no test files") ||
				strings.Contains(lowerOut, "no specs found") ||
				(strings.Contains(lowerOut, "0 passed") && !strings.Contains(lowerOut, "failed") && !strings.Contains(lowerOut, "error"))

			// Injeta os parses de diagnósticos na resposta (Task 13, 15, 16, 19, 20)
			var diagnosticSummary strings.Builder
			diagnosticSummary.WriteString("\n\n=== 📊 DIAGNÓSTICOS DE EXECUÇÃO DE TESTES ===\n")

			// Profiling de Performance (Task 19)
			maxrss := getMaxRSS(c)
			if maxrss > 0 {
				memoryMB := float64(maxrss) / 1024.0
				diagnosticSummary.WriteString(fmt.Sprintf("⚡ Pico de memória detectado: %.2f MB\n", memoryMB))
			}
			
			// Cobertura (Task 13)
			if cov, found := parseCoverage(out); found {
				diagnosticSummary.WriteString(fmt.Sprintf("✓ %s\n", cov))
			} else {
				diagnosticSummary.WriteString("ℹ️  Nenhum indicador de cobertura de código foi detectado no output.\n")
			}

			// Semântica de AssertionError (Task 15)
			if details, found := parseAssertionError(out); found {
				diagnosticSummary.WriteString(fmt.Sprintf("❌ Falha semântica detectada: %s\n", details))
			} else {
				diagnosticSummary.WriteString("✓ Nenhuma falha crítica / AssertionError explícito detectado no output.\n")
			}

			// Mocking Automático de Dependências Externas (Task 16)
			if proxy != nil {
				hosts, urls := proxy.GetRecorded()
				if len(hosts) > 0 || len(urls) > 0 {
					diagnosticSummary.WriteString("⚠️  Chamadas de rede externas interceptadas e mockadas:\n")
					for _, h := range hosts {
						diagnosticSummary.WriteString(fmt.Sprintf("   - [HTTPS] %s (rejeitado via proxy)\n", h))
					}
					for _, u := range urls {
						diagnosticSummary.WriteString(fmt.Sprintf("   - [HTTP] %s (resposta simulada injetada)\n", u))
					}
				} else {
					diagnosticSummary.WriteString("✓ Nenhuma chamada de rede externa interceptada pelo proxy.\n")
				}
			}

			// Validação de Ambiente (Task 20)
			diagnosticSummary.WriteString(checkEnvironment() + "\n")
			diagnosticSummary.WriteString("============================================\n")

			fullOutput := out + diagnosticSummary.String()

			if success && isEmpty {
				return tools.Result{
					Success: false,
					Error:   "validador: nenhum teste real foi executado (saída vazia ou 0 testes executados)",
					Data:    fullOutput,
				}, nil
			}

			if !success {
				return tools.Result{
					Success: false,
					Error:   fmt.Sprintf("falha na execução dos testes (rodada %d/%d)", runIdx+1, input.Repeat),
					Data:    fullOutput,
				}, nil
			}

			return tools.Result{
				Success: true,
				Data:    fullOutput,
			}, nil
		}()

		if err != nil {
			return tools.Result{}, err
		}
		if !res.Success {
			return res, nil
		}

		if input.Repeat > 1 {
			finalOut.WriteString(fmt.Sprintf("=== RODADA %d/%d ===\n%s\n", runIdx+1, input.Repeat, res.Data))
		} else {
			finalOut.WriteString(res.Data)
		}
	}

	return tools.Result{
		Success: true,
		Data:    finalOut.String(),
	}, nil
}

// parseCoverage tenta extrair a porcentagem de cobertura (Go ou Python) do output
func parseCoverage(out string) (string, bool) {
	// Padrão Go: "coverage: 85.5% of statements"
	goRegex := regexp.MustCompile(`coverage:\s*([0-9.]+)%`)
	if match := goRegex.FindStringSubmatch(out); len(match) > 1 {
		return match[0], true
	}

	// Padrão Python (pytest-cov): "TOTAL  100  15   85%"
	pyRegex := regexp.MustCompile(`TOTAL\s+\d+\s+\d+\s+([0-9.]+)%`)
	if match := pyRegex.FindStringSubmatch(out); len(match) > 1 {
		return fmt.Sprintf("coverage: %s%% (TOTAL)", match[1]), true
	}

	return "", false
}

// parseAssertionError procura detalhes de AssertionErrors, pânicos ou falhas no output
func parseAssertionError(out string) (string, bool) {
	var details []string

	// Padrão de AssertionError (Python) ou Panic (Go)
	assertRegex := regexp.MustCompile(`(?i)(AssertionError|panic|panic:\s*.*|Fail:\s*.*)`)
	if match := assertRegex.FindString(out); match != "" {
		details = append(details, fmt.Sprintf("Tipo de Falha: %s", match))
	}

	// Procurar por nomes de arquivo e linha no formato: arquivo.go:linha ou arquivo.py:linha
	fileLineRegex := regexp.MustCompile(`([a-zA-Z0-9_/.-]+\.(?:go|py|js|ts|java|rs)):(\d+)`)
	matches := fileLineRegex.FindAllStringSubmatch(out, 5) // limitar a 5 ocorrências para evitar ruído
	if len(matches) > 0 {
		var locations []string
		for _, m := range matches {
			locations = append(locations, fmt.Sprintf("%s:%s", m[1], m[2]))
		}
		details = append(details, fmt.Sprintf("Localizações suspeitas: %s", strings.Join(locations, ", ")))
	}

	if len(details) > 0 {
		return strings.Join(details, " | "), true
	}
	return "", false
}

// checkEnvironment analisa se variáveis de ambiente necessárias/sensíveis estão configuradas
func checkEnvironment() string {
	var checks []string

	// Alertas para chaves sensíveis reais expostas ou não definidas
	envKeys := []string{"OPENAI_API_KEY", "OPENROUTER_API_KEY", "HF_TOKEN"}
	for _, key := range envKeys {
		val := os.Getenv(key)
		if val != "" {
			checks = append(checks, fmt.Sprintf("⚠️  Aviso: Variável real %s está configurada no ambiente (cuidado para não vazar segredos).", key))
		} else {
			checks = append(checks, fmt.Sprintf("ℹ️  Info: %s não configurada (mocks/fallbacks locais de teste serão acionados se necessário).", key))
		}
	}

	return strings.Join(checks, "\n")
}

// detectTestCommand escaneia o workspace em busca de arquivos de build/config e sugere o comando de testes correspondente
func (t *RunTestsTool) detectTestCommand() string {
	// 1. Verificar Makefile para target de teste customizado
	if makefileCmd := t.detectFromMakefile(); makefileCmd != "" {
		return makefileCmd
	}

	// 2. Verificar configurações específicas de framework
	// Go
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "go.mod")); err == nil {
		// Verificar se tem tags especiais
		tags := t.detectGoTestTags()
		if tags != "" {
			return fmt.Sprintf("go test -tags %s ./...", tags)
		}
		return "go test ./..."
	}

	// Python: verificar pytest.ini, setup.cfg, pyproject.toml
	if cmd := t.detectPythonTestCommand(); cmd != "" {
		return cmd
	}

	// JavaScript/TypeScript: verificar package.json scripts
	if cmd := t.detectJSTestCommand(); cmd != "" {
		return cmd
	}

	// Rust
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "Cargo.toml")); err == nil {
		return "cargo test"
	}

	return ""
}

// detectFromMakefile busca targets de teste no Makefile
func (t *RunTestsTool) detectFromMakefile() string {
	makefilePath := filepath.Join(t.workspaceRoot, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return ""
	}

	// Buscar targets: test, tests, check, unittest
	content := string(data)
	testTargetRegex := regexp.MustCompile(`(?m)^(test|tests|check|unittest)\s*:`)
	if m := testTargetRegex.FindStringSubmatch(content); len(m) > 1 {
		return "make " + m[1]
	}
	return ""
}

// detectGoTestTags busca tags de build necessárias para testes Go
func (t *RunTestsTool) detectGoTestTags() string {
	// Verificar se existem arquivos com build tags comuns
	testTags := []string{"integration", "e2e", "headless"}
	for _, tag := range testTags {
		pattern := fmt.Sprintf("//go:build %s", tag)
		// Scan rápido nos primeiros arquivos _test.go
		_ = filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				if info != nil && info.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				data, readErr := os.ReadFile(path)
				if readErr == nil && strings.Contains(string(data), pattern) {
					return fmt.Errorf("found:%s", tag)
				}
			}
			return nil
		})
	}
	return ""
}

// detectPythonTestCommand detecta a configuração de testes Python
func (t *RunTestsTool) detectPythonTestCommand() string {
	// pytest.ini
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "pytest.ini")); err == nil {
		// Ler testpaths e flags
		data, readErr := os.ReadFile(filepath.Join(t.workspaceRoot, "pytest.ini"))
		if readErr == nil {
			content := string(data)
			addoptsRegex := regexp.MustCompile(`(?m)addopts\s*=\s*(.+)`)
			if m := addoptsRegex.FindStringSubmatch(content); len(m) > 1 {
				return "pytest " + strings.TrimSpace(m[1])
			}
		}
		return "pytest"
	}

	// setup.cfg com seção [tool:pytest]
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "setup.cfg")); err == nil {
		data, readErr := os.ReadFile(filepath.Join(t.workspaceRoot, "setup.cfg"))
		if readErr == nil && strings.Contains(string(data), "[tool:pytest]") {
			return "pytest"
		}
	}

	// pyproject.toml
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "pyproject.toml")); err == nil {
		data, readErr := os.ReadFile(filepath.Join(t.workspaceRoot, "pyproject.toml"))
		if readErr == nil {
			content := string(data)
			if strings.Contains(content, "[tool.pytest") {
				return "pytest"
			}
			if strings.Contains(content, "[tool.poetry") {
				return "poetry run pytest"
			}
		}
		return "pytest"
	}

	// requirements.txt genérico
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "requirements.txt")); err == nil {
		return "pytest"
	}

	return ""
}

// detectJSTestCommand detecta a configuração de testes JavaScript/TypeScript
func (t *RunTestsTool) detectJSTestCommand() string {
	pkgPath := filepath.Join(t.workspaceRoot, "package.json")
	if _, err := os.Stat(pkgPath); err != nil {
		return ""
	}

	data, readErr := os.ReadFile(pkgPath)
	if readErr != nil {
		return "npm test"
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "npm test"
	}

	// Verificar se existe script test customizado
	if testCmd, ok := pkg.Scripts["test"]; ok {
		if testCmd != "" && testCmd != "echo \"Error: no test specified\" && exit 1" {
			return "npm test"
		}
	}

	// Verificar jest config
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "jest.config.js")); err == nil {
		return "npx jest"
	}
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "jest.config.ts")); err == nil {
		return "npx jest"
	}

	// Verificar vitest
	if _, err := os.Stat(filepath.Join(t.workspaceRoot, "vitest.config.ts")); err == nil {
		return "npx vitest run"
	}

	return "npm test"
}

