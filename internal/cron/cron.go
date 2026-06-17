package cron

import (
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

// CronScheduler gerencia tarefas agendadas em expressões Cron
type CronScheduler struct {
	mu        sync.Mutex
	scheduler *cron.Cron
	entries   map[string]cron.EntryID
}

// NewCronScheduler cria um novo agendador cron com suporte a segundos
func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		scheduler: cron.New(cron.WithSeconds()),
		entries:   make(map[string]cron.EntryID),
	}
}

// Start inicia a execução do agendador em background
func (s *CronScheduler) Start() {
	s.scheduler.Start()
}

// Stop finaliza a execução do agendador
func (s *CronScheduler) Stop() {
	s.scheduler.Stop()
}

// AddJob agenda um job sob o formato de especificação cron
func (s *CronScheduler) AddJob(name, spec string, jobFunc func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, exists := s.entries[name]; exists {
		s.scheduler.Remove(id)
	}

	id, err := s.scheduler.AddFunc(spec, jobFunc)
	if err != nil {
		return fmt.Errorf("especificação cron inválida '%s': %w", spec, err)
	}

	s.entries[name] = id
	return nil
}

// RemoveJob remove um job agendador por nome
func (s *CronScheduler) RemoveJob(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, exists := s.entries[name]; exists {
		s.scheduler.Remove(id)
		delete(s.entries, name)
	}
}
