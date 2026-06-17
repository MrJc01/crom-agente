package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
	loadedTools, err := LoadScriptsFromDir(tempDir)
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
	tool := NewBrowserTool(tempDir, true)

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
	tool := NewComputerControlTool(tempDir)

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
