package daemon

import (
	"fmt"

	"github.com/gen2brain/beeep"
)

// DesktopNotifier gerencia o envio de notificações do sistema
type DesktopNotifier struct {
	appName string
}

// NewDesktopNotifier cria um novo notifier
func NewDesktopNotifier(appName string) *DesktopNotifier {
	return &DesktopNotifier{appName: appName}
}

// Notify envia uma notificação push desktop
func (n *DesktopNotifier) Notify(title, message string) error {
	err := beeep.Notify(title, message, "")
	if err != nil {
		return fmt.Errorf("falha ao enviar notificacao: %w", err)
	}
	return nil
}
