package loop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
)

// Regressão: caminhos planejados alucinados ("/app/...") ou com esquema Godot
// ("res://...") não devem gerar PHYSICAL_FILE_MISSING quando o arquivo existe
// no workspace (criado localmente ou no editor via MCP).
func TestVerifyExpectedFiles_ResolvesAppAndResPaths(t *testing.T) {
	ws := t.TempDir()
	write := func(p string) {
		full := filepath.Join(ws, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.godot")
	write("scenes/main_scene.tscn")
	write("scripts/game_manager.gd")

	planned := []string{
		"/app/project.godot",
		"/app/scenes/main_scene.tscn",
		"/app/scripts/game_manager.gd",
		"res://scripts/game_manager.gd",
		"scenes/main_scene.tscn",
	}
	if missing := VerifyExpectedFiles(planned, ws); len(missing) != 0 {
		t.Fatalf("esperava 0 faltando, veio: %v", missing)
	}

	if missing := VerifyExpectedFiles([]string{"/app/scripts/inexistente.gd"}, ws); len(missing) != 1 {
		t.Fatalf("esperava 1 faltando para arquivo inexistente, veio: %v", missing)
	}
}

// ParseExpectedFiles deve capturar arquivos planejados nos formatos markdown
// reais do modelo (não só o literal "[NEW]").
func TestParseExpectedFiles_MarkdownVariants(t *testing.T) {
	msgs := []llm.Message{
		{Role: "assistant", Content: "Plano:\n- **NEW**: `res://snake_game.gd` (logica)\n- [NEW] res://scenes/main.tscn\n- **MODIFY:** scripts/player.gd"},
		{Role: "assistant", Content: "Criado [x](file:///home/j/proj/hud.gd)."},
		{Role: "assistant", Content: "Tudo pronto com sucesso, sem novos arquivos aqui."},
	}
	got := ParseExpectedFiles(msgs)
	want := map[string]bool{
		"res://snake_game.gd":    true,
		"res://scenes/main.tscn": true,
		"scripts/player.gd":      true,
		"/home/j/proj/hud.gd":    true,
	}
	for _, g := range got {
		delete(want, g)
	}
	if len(want) != 0 {
		t.Fatalf("nao capturou esperados: faltaram %v (capturou %v)", want, got)
	}
}
