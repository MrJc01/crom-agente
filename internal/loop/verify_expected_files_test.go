package loop

import (
	"os"
	"path/filepath"
	"testing"
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
