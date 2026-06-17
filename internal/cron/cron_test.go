package cron

import (
	"sync"
	"testing"
	"time"
)

func TestCronScheduler_AddRemoveJob(t *testing.T) {
	s := NewCronScheduler()
	s.Start()
	defer s.Stop()

	var mu sync.Mutex
	count := 0

	// Executa a cada segundo
	err := s.AddJob("test", "*/1 * * * * *", func() {
		mu.Lock()
		count++
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("erro ao agendar job: %v", err)
	}

	// Aguarda disparar pelo menos uma vez
	time.Sleep(1200 * time.Millisecond)

	mu.Lock()
	firstCount := count
	mu.Unlock()

	if firstCount == 0 {
		t.Fatal("esperava que o job tivesse rodado pelo menos uma vez")
	}

	// Remove o job
	s.RemoveJob("test")

	// Espera mais um segundo para garantir que não roda mais
	time.Sleep(1000 * time.Millisecond)

	mu.Lock()
	secondCount := count
	mu.Unlock()

	if secondCount != firstCount {
		t.Fatalf("job rodou depois de ser removido: anterior=%d, atual=%d", firstCount, secondCount)
	}
}

func TestCronScheduler_InvalidSpec(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob("invalid", "specs incorreta", func() {})
	if err == nil {
		t.Fatal("esperava erro para especificação cron inválida")
	}
}
