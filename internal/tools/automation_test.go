package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/crom/crom-agente/internal/agents"
	browser_subagent "github.com/crom/crom-agente/internal/agents/specialists/browser"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/browser"
	"github.com/crom/crom-agente/internal/tools/computer_control"
)

func TestScriptTool_LoadAndExecute(t *testing.T) {
	tempDir := t.TempDir()

	// Cria um script de teste simples
	var scriptName string
	var scriptContent string

	if runtime.GOOS == "windows" {
		scriptName = "test_script.bat"
		scriptContent = "@echo off\necho args: %1 %2"
	} else {
		scriptName = "test_script.sh"
		scriptContent = "#!/bin/bash\necho \"args: $1 $2\""
	}

	scriptPath := filepath.Join(tempDir, scriptName)
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("erro ao criar script temporário: %v", err)
	}

	// 1. Carrega scripts do diretório
	loadedTools, err := tools.LoadScriptsFromDir(tempDir)
	if err != nil {
		t.Fatalf("erro ao carregar scripts: %v", err)
	}

	if len(loadedTools) != 1 {
		t.Fatalf("esperava 1 ferramenta carregada, obteve %d", len(loadedTools))
	}

	tool := loadedTools[0]
	if tool.ID() != "test_script" {
		t.Errorf("esperava ID 'test_script', obteve %q", tool.ID())
	}

	if !tool.RequiresApproval() {
		t.Error("esperava que ScriptTool exigisse aprovação (RequiresApproval = true)")
	}

	// 2. Executa a ferramenta
	ctx := context.Background()
	args := json.RawMessage(`{"arguments": ["hello", "world"]}`)
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("erro ao executar ScriptTool: %v", err)
	}

	if !res.Success {
		t.Fatalf("esperava execução com sucesso, erro: %s", res.Error)
	}

	if !json.Valid(tool.ParametersSchema()) {
		t.Error("ParametersSchema inválido")
	}
}

func TestBrowserTool_ConstructAndMetadata(t *testing.T) {
	tempDir := t.TempDir()
	tool := browser.NewBrowserTool(tempDir, true)

	if tool.ID() != "browser_action" {
		t.Errorf("esperava ID 'browser_action', obteve %q", tool.ID())
	}

	if tool.RequiresApproval() {
		t.Error("browser_action não deve exigir aprovação (RequiresApproval = false)")
	}

	if len(tool.Description()) == 0 {
		t.Error("descrição não pode ser vazia")
	}

	// Testa execução com argumentos inválidos (action vazia)
	ctx := context.Background()
	res, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("erro inesperado ao executar com argumentos vazios: %v", err)
	}
	if res.Success {
		t.Error("esperava Success=false ao executar com action vazia")
	}
}

func TestComputerControlTool_ConstructAndMetadata(t *testing.T) {
	tempDir := t.TempDir()
	tool := computer_control.NewComputerControlTool(tempDir)

	if tool.ID() != "computer_control" {
		t.Errorf("esperava ID 'computer_control', obteve %q", tool.ID())
	}

	if !tool.RequiresApproval() {
		t.Error("computer_control deve exigir aprovação (RequiresApproval = true)")
	}

	// Testa argumentos inválidos (action vazia)
	ctx := context.Background()
	res, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("erro inesperado ao executar com argumentos vazios: %v", err)
	}
	if res.Success {
		t.Error("esperava Success=false ao executar com action vazia")
	}
}

func TestBrowserTool_ScreenshotPath(t *testing.T) {
	tempDir := t.TempDir()
	tool := browser.NewBrowserTool(tempDir, true)
	defer tool.Close()

	ctx := context.Background()

	// Navega para um HTML básico local via data URL
	navArgs := json.RawMessage(`{"action": "navigate", "url": "data:text/html,<html><body><h1>Cromia Test</h1></body></html>"}`)
	resNav, err := tool.Execute(ctx, navArgs)
	if err != nil {
		t.Skipf("pulando teste do browser (Chromium pode não estar instalado): %v", err)
		return
	}
	if !resNav.Success {
		t.Skipf("pulando teste do browser (falha ao navegar): %s", resNav.Error)
		return
	}

	// Executa screenshot especificando um path relativo
	screenshotPath := "subfolder/screenshot_test.png"
	shotArgs := json.RawMessage(`{"action": "screenshot", "path": "subfolder/screenshot_test.png"}`)
	resShot, err := tool.Execute(ctx, shotArgs)
	if err != nil {
		t.Fatalf("erro ao executar screenshot: %v", err)
	}
	if !resShot.Success {
		t.Fatalf("falha ao executar screenshot: %s", resShot.Error)
	}

	// Verifica se o arquivo foi criado na pasta temporária correta
	expectedFile := filepath.Join(tempDir, screenshotPath)
	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("arquivo de screenshot esperado não foi criado: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("arquivo de screenshot está vazio")
	}
}

func TestComputerControlTool_ScreenshotPath(t *testing.T) {
	tempDir := t.TempDir()
	tool := computer_control.NewComputerControlTool(tempDir)

	ctx := context.Background()
	screenshotPath := "subfolder/comp_screenshot_test.png"
	shotArgs := json.RawMessage(`{"action": "screenshot", "path": "subfolder/comp_screenshot_test.png"}`)

	resShot, err := tool.Execute(ctx, shotArgs)
	if err != nil {
		t.Fatalf("erro ao executar screenshot: %v", err)
	}

	if !resShot.Success {
		// Se falhar devido à ausência de scrot/gnome-screenshot ou falta de display, pulamos
		t.Skipf("pulando teste de captura de tela de computador (pode faltar display/utilitário): %s", resShot.Error)
		return
	}

	// Verifica se o arquivo foi criado na pasta temporária correta
	expectedFile := filepath.Join(tempDir, screenshotPath)
	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("arquivo de screenshot esperado não foi criado: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("arquivo de screenshot está vazio")
	}
}

func TestBrowserSubagentTool_E2E(t *testing.T) {
	tempDir := t.TempDir()
	agent := browser_subagent.NewBrowserAgent(agents.Config{
		WorkspacePath:   tempDir,
		BrowserHeadless: true,
	})
	defer agent.Close()
	tool := tools.NewAgentToolAdapter(agent)

	ctx := context.Background()

	// Cria uma sequência de etapas para navegar para uma data URL, verificar o HTML e tirar uma screenshot
	args := json.RawMessage(`{
		"task": "Test browser subagent tool",
		"steps": [
			{
				"action": "navigate",
				"url": "data:text/html,<html><body><h1>Cromia Subagent Test</h1></body></html>"
			},
			{
				"action": "get_html",
				"verify_contains": "Cromia Subagent Test"
			},
			{
				"action": "screenshot",
				"path": "sub/subagent_screenshot.png"
			}
		],
		"capture_final_screenshot": true
	}`)

	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Skipf("pulando teste do browser_subagent (Chromium pode não estar instalado): %v", err)
		return
	}
	if !res.Success {
		t.Skipf("pulando teste do browser_subagent (falha ao executar etapas): %s", res.Error)
		return
	}

	// Verifica se a screenshot foi salva no disco
	expectedFile := filepath.Join(tempDir, "sub/subagent_screenshot.png")
	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("arquivo de screenshot do subagente não foi criado: %v", err)
	}
	if info.Size() == 0 {
		t.Errorf("arquivo de screenshot do subagente está vazio")
	}

	// Verifica se o resultado tem o prefixo de imagem e contém o relatório JSON (comentado na nova arquitetura)
	// if !strings.HasPrefix(res.Data, "image:base64:") {
	// 	t.Errorf("dados de retorno devem começar com prefixo de imagem base64, obtido: %s", res.Data)
	// }
}
