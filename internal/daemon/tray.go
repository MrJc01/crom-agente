//go:build !headless && cgo

package daemon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crom/crom-agente/internal/config"
	"github.com/getlantern/systray"
)

var (
	iconIdleBytes    []byte
	iconRunningBytes []byte
	iconErrorBytes   []byte
)

func generateSolidPNG(r, g, b uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for x := 0; x < 16; x++ {
		for y := 0; y < 16; y++ {
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func init() {
	iconIdleBytes = generateSolidPNG(0, 200, 0)      // verde
	iconRunningBytes = generateSolidPNG(0, 122, 255) // azul
	iconErrorBytes = generateSolidPNG(255, 59, 48)   // vermelho
}

type realTrayManager struct {
	mu                sync.Mutex
	statusItem        *systray.MenuItem
	agentItems        []*systray.MenuItem
	openLogItem       *systray.MenuItem
	stopAgentsItem    *systray.MenuItem
	openWorkspaceItem *systray.MenuItem
	stopItem          *systray.MenuItem
	quitChan          chan struct{}
	onStopAgents      func()
	onOpenWorkspace   func()
}

// NewTrayManager cria a versao real do TrayManager
func NewTrayManager() TrayManager {
	return &realTrayManager{
		quitChan: make(chan struct{}),
	}
}

func (t *realTrayManager) SetOnStopAgents(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onStopAgents = f
}

func (t *realTrayManager) SetOnOpenWorkspace(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onOpenWorkspace = f
}

func (t *realTrayManager) Run(onReady func(), onExit func()) {
	systray.Run(func() {
		systray.SetIcon(iconIdleBytes)
		systray.SetTitle("crom-agente")
		systray.SetTooltip("crom-agente - Orquestrador de Agentes")

		// Titulo
		titleItem := systray.AddMenuItem("🤖 crom-agente", "Orquestrador de Agentes")
		titleItem.Disable()

		t.statusItem = systray.AddMenuItem("Estado: 🟢 Ativo (Idle)", "Estado atual do daemon")
		t.statusItem.Disable()

		systray.AddSeparator()

		// Ações do Daemon
		t.openLogItem = systray.AddMenuItem("Abrir Log de Execução", "Abre o arquivo daemon.log")
		t.openWorkspaceItem = systray.AddMenuItem("Abrir Diretório do Workspace", "Abre a pasta do projeto ativo")
		t.stopAgentsItem = systray.AddMenuItem("Parar Todos os Agentes", "Cancela a execução de todos os loops ReAct")

		systray.AddSeparator()

		// Secao de agentes (pool de 5 itens ocultos inicialmente)
		agentsTitle := systray.AddMenuItem("Agentes Ativos:", "")
		agentsTitle.Disable()
		t.agentItems = make([]*systray.MenuItem, 5)
		for i := 0; i < 5; i++ {
			t.agentItems[i] = systray.AddMenuItem("", "")
			t.agentItems[i].Hide()
			t.agentItems[i].Disable()
		}

		systray.AddSeparator()

		t.stopItem = systray.AddMenuItem("Parar Daemon", "Encerra o daemon crom-agente")

		// Background loop para lidar com cliques
		go func() {
			for {
				select {
				case <-t.stopItem.ClickedCh:
					t.Quit()
				case <-t.openLogItem.ClickedCh:
					dir, err := config.GlobalDir()
					if err == nil {
						logPath := filepath.Join(dir, "daemon.log")
						_ = openPath(logPath)
					}
				case <-t.stopAgentsItem.ClickedCh:
					t.mu.Lock()
					callback := t.onStopAgents
					t.mu.Unlock()
					if callback != nil {
						callback()
					}
				case <-t.openWorkspaceItem.ClickedCh:
					t.mu.Lock()
					callback := t.onOpenWorkspace
					t.mu.Unlock()
					if callback != nil {
						callback()
					}
				case <-t.quitChan:
					systray.Quit()
					return
				}
			}
		}()

		// Chama callback do daemon
		onReady()
	}, func() {
		onExit()
	})
}

func (t *realTrayManager) SetStatus(status string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.statusItem != nil {
		t.statusItem.SetTitle("Estado: " + status)
	}
	if strings.Contains(status, "Executando") {
		systray.SetIcon(iconRunningBytes)
	} else if strings.Contains(status, "Erro") || strings.Contains(status, "Error") {
		systray.SetIcon(iconErrorBytes)
	} else {
		systray.SetIcon(iconIdleBytes)
	}
}

func (t *realTrayManager) UpdateRunningAgents(agents []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.agentItems) == 0 {
		return
	}
	for i := 0; i < len(t.agentItems); i++ {
		if i < len(agents) {
			t.agentItems[i].SetTitle(" 🔹 " + agents[i])
			t.agentItems[i].Show()
		} else {
			t.agentItems[i].Hide()
		}
	}
}

func (t *realTrayManager) Quit() {
	select {
	case <-t.quitChan:
		// Ja fechado
	default:
		close(t.quitChan)
	}
}

func init() {
	GlobalTray = NewTrayManager()
}
