package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPermissionManager_TotalAccess(t *testing.T) {
	ws := t.TempDir()
	pm := NewPermissionManager(ws, "total_access", nil)

	approved, err := pm.Authorize("command", "rm -rf /")
	if err != nil {
		t.Fatalf("erro ao autorizar: %v", err)
	}
	if !approved {
		t.Fatal("esperava aprovação automática no modo total_access")
	}
}

func TestPermissionManager_AskEveryTime(t *testing.T) {
	ws := t.TempDir()
	called := false
	pm := NewPermissionManager(ws, "ask_every_time", func(action, target string) (bool, bool) {
		called = true
		if action == "command" && target == "git diff" {
			return true, false
		}
		return false, false
	})

	approved, err := pm.Authorize("command", "git diff")
	if err != nil {
		t.Fatalf("erro ao autorizar: %v", err)
	}
	if !approved || !called {
		t.Fatalf("esperava aprovação, chamada=%t", called)
	}

	// Tenta ação não aprovada
	called = false
	approved, _ = pm.Authorize("command", "git push")
	if approved || !called {
		t.Fatalf("esperava reprovação, chamada=%t", called)
	}
}

func TestPermissionManager_Scoped_ApprovedAndRemember(t *testing.T) {
	ws := t.TempDir()

	// 1. Primeira autorização com aprovação + remember
	called := false
	pm := NewPermissionManager(ws, "scoped", func(action, target string) (bool, bool) {
		called = true
		return true, true // aprova e manda salvar
	})

	approved, err := pm.Authorize("write_file", "/workspace/src/main.go")
	if err != nil {
		t.Fatalf("erro ao autorizar: %v", err)
	}
	if !approved || !called {
		t.Fatalf("esperava aprovação com chamada, chamada=%t", called)
	}

	// O arquivo permissions.json deve ter sido criado
	permFilePath := filepath.Join(ws, ".crom", "permissions.json")
	if _, err := os.Stat(permFilePath); os.IsNotExist(err) {
		t.Fatal("arquivo permissions.json não foi gerado")
	}

	// 2. Segunda autorização para a mesma ação (deve usar o cache de grants sem perguntar)
	called = false
	pm2 := NewPermissionManager(ws, "scoped", func(action, target string) (bool, bool) {
		called = true
		return false, false
	})

	approved, err = pm2.Authorize("write_file", "/workspace/src/main.go")
	if err != nil {
		t.Fatalf("erro ao autorizar no cache: %v", err)
	}
	if !approved || called {
		t.Fatalf("esperava aprovação do cache sem chamar askFunc, chamada=%t", called)
	}
}

func TestPermissionManager_Scoped_WildcardMatching(t *testing.T) {
	ws := t.TempDir()

	// Define grants manuais
	pm := NewPermissionManager(ws, "scoped", nil)
	pm.grants = []Grant{
		{Action: "command", Target: "git *"},
		{Action: "write_file", Target: "/workspace/src/*"},
		{Action: "read_file", Target: "*"},
	}
	_ = pm.saveGrantsLocked()

	// 1. Testa match wildcard de comando
	pm2 := NewPermissionManager(ws, "scoped", func(action, target string) (bool, bool) {
		t.Fatal("não deveria ter chamado askFunc")
		return false, false
	})

	approved, _ := pm2.Authorize("command", "git checkout main")
	if !approved {
		t.Fatal("esperado match com 'git *'")
	}

	// 2. Testa match de arquivo
	approved, _ = pm2.Authorize("write_file", "/workspace/src/utils/file.go")
	if !approved {
		t.Fatal("esperado match com '/workspace/src/*'")
	}

	// 3. Testa match universal de leitura
	approved, _ = pm2.Authorize("read_file", "/etc/passwd")
	if !approved {
		t.Fatal("esperado match com '*'")
	}

	// 4. Sem match
	pm3 := NewPermissionManager(ws, "scoped", func(action, target string) (bool, bool) {
		return true, false
	})
	approved, _ = pm3.Authorize("write_file", "/etc/passwd")
	if !approved {
		t.Fatal("esperada aprovação interativa após sem match")
	}
}
