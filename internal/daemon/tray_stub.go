//go:build headless || !cgo

package daemon

import (
	"log"
)

type stubTrayManager struct {
	quitChan chan struct{}
}

// NewTrayManager cria a versao stub do TrayManager
func NewTrayManager() TrayManager {
	return &stubTrayManager{
		quitChan: make(chan struct{}),
	}
}

func (s *stubTrayManager) Run(onReady func(), onExit func()) {
	log.Println("[Daemon] Rodando em modo headless (sem bandeja do sistema)")
	go onReady()
	<-s.quitChan
	onExit()
}

func (s *stubTrayManager) SetStatus(status string)            {}
func (s *stubTrayManager) UpdateRunningAgents(agents []string) {}
func (s *stubTrayManager) SetOnStopAgents(f func())            {}
func (s *stubTrayManager) SetOnOpenWorkspace(f func())         {}
func (s *stubTrayManager) Quit() {
	select {
	case <-s.quitChan:
		// Ja fechado
	default:
		close(s.quitChan)
	}
}

func init() {
	GlobalTray = NewTrayManager()
}
