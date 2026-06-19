package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/state"
)

func TestHandleSlashCommand_Help(t *testing.T) {
	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   t.TempDir(),
			SessionName:   "test-session",
		},
	}
	msg, handled := HandleSlashCommand("/help", model)
	if !handled {
		t.Fatal("esperado comando /help tratado")
	}
	if !strings.Contains(msg, "Comandos de Barra") {
		t.Errorf("mensagem inesperada para /help: %q", msg)
	}
}

func TestHandleSlashCommand_Exit(t *testing.T) {
	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   t.TempDir(),
			SessionName:   "test-session",
		},
	}
	msg, handled := HandleSlashCommand("/exit", model)
	if !handled {
		t.Fatal("esperado comando /exit tratado")
	}
	if !model.shouldExit {
		t.Error("esperado model.shouldExit como true")
	}
	if !strings.Contains(msg, "Encerrando sessão") {
		t.Errorf("mensagem inesperada para /exit: %q", msg)
	}
}

func TestHandleSlashCommand_Clear(t *testing.T) {
	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   t.TempDir(),
			SessionName:   "test-session",
		},
	}
	msg, handled := HandleSlashCommand("/clear", model)
	if !handled {
		t.Fatal("esperado comando /clear tratado")
	}
	if !strings.Contains(msg, "Tela limpa") {
		t.Errorf("mensagem inesperada para /clear: %q", msg)
	}
}

func TestHandleSlashCommand_Cost(t *testing.T) {
	tempStorage := t.TempDir()
	sm := state.NewSessionStateManager(tempStorage, "test-session")
	_ = sm.LoadState()

	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   tempStorage,
			SessionName:   "test-session",
		},
		stateManager: sm,
	}

	msg, handled := HandleSlashCommand("/cost", model)
	if !handled {
		t.Fatal("esperado comando /cost tratado")
	}
	if !strings.Contains(msg, "Tokens gastos") {
		t.Errorf("mensagem inesperada para /cost: %q", msg)
	}
}

func TestHandleSlashCommand_Color(t *testing.T) {
	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   t.TempDir(),
			SessionName:   "test-session",
		},
	}

	msg, handled := HandleSlashCommand("/color red", model)
	if !handled {
		t.Fatal("esperado comando /color tratado")
	}
	if model.promptColor != "\033[1;31m" {
		t.Errorf("cor de prompt inesperada: %q", model.promptColor)
	}
	if !strings.Contains(msg, "atualizada para 'red'") {
		t.Errorf("mensagem inesperada para /color red: %q", msg)
	}

	// Cor inválida
	msg, handled = HandleSlashCommand("/color invalid-color", model)
	if !handled {
		t.Fatal("esperado comando /color tratado")
	}
	if !strings.Contains(msg, "Cor desconhecida") {
		t.Errorf("mensagem inesperada para cor inválida: %q", msg)
	}
}

func TestHandleSlashCommand_Add(t *testing.T) {
	tempWorkspace := t.TempDir()
	model := &TUIModel{
		options: Options{
			WorkspacePath: tempWorkspace,
			StoragePath:   t.TempDir(),
			SessionName:   "test-session",
		},
		attachments: make(map[string]string),
	}

	// Testa sem arquivo
	msg, handled := HandleSlashCommand("/add", model)
	if !handled {
		t.Fatal("esperado comando /add tratado")
	}
	if !strings.Contains(msg, "Erro: Especifique o arquivo") {
		t.Errorf("mensagem inesperada para /add sem args: %q", msg)
	}

	// Cria arquivo de teste no workspace
	testFile := "hello.txt"
	testContent := "hello world"
	err := os.WriteFile(filepath.Join(tempWorkspace, testFile), []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("erro ao criar arquivo de teste: %v", err)
	}

	msg, handled = HandleSlashCommand("/add "+testFile, model)
	if !handled {
		t.Fatal("esperado comando /add tratado")
	}
	if !strings.Contains(msg, "adicionado com sucesso") {
		t.Errorf("mensagem inesperada para /add com arquivo: %q", msg)
	}
	if model.attachments[testFile] != testContent {
		t.Errorf("conteúdo anexado incorreto: %q", model.attachments[testFile])
	}
}

func TestHandleSlashCommand_Session(t *testing.T) {
	tempStorage := t.TempDir()
	model := &TUIModel{
		options: Options{
			WorkspacePath: t.TempDir(),
			StoragePath:   tempStorage,
			SessionName:   "test-session",
		},
	}

	// Testa sem nome de sessão
	msg, handled := HandleSlashCommand("/session", model)
	if !handled {
		t.Fatal("esperado comando /session tratado")
	}
	if !strings.Contains(msg, "Erro: Especifique o nome") {
		t.Errorf("mensagem inesperada para /session sem args: %q", msg)
	}

	// Testa alternar sessão
	msg, handled = HandleSlashCommand("/session new-session", model)
	if !handled {
		t.Fatal("esperado comando /session tratado")
	}
	if !strings.Contains(msg, "Chaveado para a sessão 'new-session'") {
		t.Errorf("mensagem inesperada para /session com args: %q", msg)
	}
	if model.options.SessionName != "new-session" {
		t.Errorf("nome da sessão não atualizado: %q", model.options.SessionName)
	}
	if model.stateManager == nil {
		t.Error("stateManager deveria ter sido inicializado")
	}
}
