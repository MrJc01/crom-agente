package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/state"
)

func newGateLoop(t *testing.T) (*AgenticLoop, string) {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "project.godot"), []byte("config_version=5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sm := state.NewStateManager(ws)
	al := New(providers.NewMockProvider(providers.MockTextResponse("ok", 1)), sm, &testEventHandler{}, &config.ResolvedConfig{DisablePromptOptimization: true})
	return al, ws
}

func toolMsg(name string) llm.Message { return llm.Message{Role: "tool", Name: name, Content: "ok"} }

func TestGodotGate_BuiltButNotVerified_Nudges(t *testing.T) {
	al, ws := newGateLoop(t)
	msgs := []llm.Message{toolMsg("godot_add_node"), toolMsg("godot_create_and_attach_script")}
	if !al.gateGodotVerification(&msgs, ws) {
		t.Fatal("esperava cutucada (built sem verify)")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "system" || !strings.Contains(last.Content, godotVerifyNudgeMarker) {
		t.Fatalf("esperava marcador de verificação injetado, veio: %q", last.Content)
	}
}

func TestGodotGate_Verified_NoNudge(t *testing.T) {
	al, ws := newGateLoop(t)
	msgs := []llm.Message{toolMsg("godot_add_node"), toolMsg("godot_verify_playable")}
	if al.gateGodotVerification(&msgs, ws) {
		t.Fatal("não deveria cutucar: já verificou com verify_playable")
	}
}

func TestGodotGate_NudgeOnlyOnce(t *testing.T) {
	al, ws := newGateLoop(t)
	msgs := []llm.Message{toolMsg("godot_add_node")}
	if !al.gateGodotVerification(&msgs, ws) {
		t.Fatal("primeira chamada deveria cutucar")
	}
	// segunda chamada: marcador já presente -> não cutuca de novo (sem loop)
	if al.gateGodotVerification(&msgs, ws) {
		t.Fatal("segunda chamada não deveria re-cutucar (evita loop)")
	}
}

func TestGodotGate_NotGodotProject_NoNudge(t *testing.T) {
	al, _ := newGateLoop(t)
	other := t.TempDir() // sem project.godot
	msgs := []llm.Message{toolMsg("godot_add_node")}
	if al.gateGodotVerification(&msgs, other) {
		t.Fatal("fora de projeto Godot não deveria cutucar")
	}
}
