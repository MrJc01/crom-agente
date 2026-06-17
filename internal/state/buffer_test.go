package state

import (
	"sync"
	"testing"
	"time"
)

func TestBufferManager_UpdatesAndSubscription(t *testing.T) {
	mgr := NewBufferManager()

	// 1. Cria assinatura
	subCh := mgr.Subscribe()
	defer mgr.Unsubscribe(subCh)

	// 2. Atualiza buffer e valida canal
	mgr.UpdateBuffer("main.go", "package main", 1)

	select {
	case upd := <-subCh:
		if upd.Path != "main.go" || upd.Content != "package main" || upd.Version != 1 {
			t.Fatalf("recebeu atualizacao invalida: %+v", upd)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout aguardando atualizacao no canal")
	}

	// 3. Valida GetBuffer
	buf, ok := mgr.GetBuffer("main.go")
	if !ok || buf.Content != "package main" {
		t.Fatalf("GetBuffer falhou, ok: %v, buf: %+v", ok, buf)
	}
}

func TestBufferManager_ApplyPatch(t *testing.T) {
	mgr := NewBufferManager()

	mgr.UpdateBuffer("app.js", "const a = 1;\nconsole.log(a);", 1)

	// Aplica patch
	err := mgr.ApplyPatch("app.js", "const a = 1;", "const a = 42;")
	if err != nil {
		t.Fatalf("erro ao aplicar patch: %v", err)
	}

	buf, _ := mgr.GetBuffer("app.js")
	if buf.Content != "const a = 42;\nconsole.log(a);" {
		t.Fatalf("conteudo pos patch incorreto: %q", buf.Content)
	}
	if buf.Version != 2 {
		t.Fatalf("esperava versao 2, obteve %d", buf.Version)
	}

	// Patch inexistente (deve falhar)
	err = mgr.ApplyPatch("app.js", "let x = 9;", "let x = 10;")
	if err == nil {
		t.Fatal("esperava erro ao aplicar patch que nao existe")
	}

	// Patch ambíguo (deve falhar)
	mgr.UpdateBuffer("dup.txt", "abc abc abc", 1)
	err = mgr.ApplyPatch("dup.txt", "abc", "def")
	if err == nil {
		t.Fatal("esperava erro de ambiguidade com multiplas ocorrencias")
	}
}

func TestBufferManager_Concurrency(t *testing.T) {
	mgr := NewBufferManager()
	mgr.UpdateBuffer("concorrente.txt", "valor inicial", 1)

	var wg sync.WaitGroup
	workers := 10
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for v := 1; v <= 5; v++ {
				mgr.UpdateBuffer("concorrente.txt", "atualizado", v)
				_, _ = mgr.GetBuffer("concorrente.txt")
			}
		}(i)
	}

	wg.Wait()

	buf, ok := mgr.GetBuffer("concorrente.txt")
	if !ok || buf.Version < 5 {
		t.Fatalf("esperava versao pelo menos 5 pós concorrencia, obteve: %+v", buf)
	}
}
