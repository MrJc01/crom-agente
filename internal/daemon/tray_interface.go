package daemon

// TrayManager define a interface para gerenciar a bandeja do sistema
type TrayManager interface {
	Run(onReady func(), onExit func())
	SetStatus(status string)
	UpdateRunningAgents(agents []string)
	Quit()
	SetOnStopAgents(func())
	SetOnOpenWorkspace(func())
}

// GlobalTray eh a instancia ativa do gerenciador de bandeja
var GlobalTray TrayManager
