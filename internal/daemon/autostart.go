package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ConfigureAutostart adiciona ou remove o binario do autostart do sistema operacional
func ConfigureAutostart(enable bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("falha ao obter caminho do executavel: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("falha ao obter home dir: %w", err)
		}

		autostartDir := filepath.Join(home, ".config", "autostart")
		desktopFilePath := filepath.Join(autostartDir, "crom-agente.desktop")

		if !enable {
			if err := os.Remove(desktopFilePath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("falha ao remover autostart: %w", err)
			}
			return nil
		}

		content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=Crom Agente Daemon
Comment=Daemon persistente do orquestrador crom-agente
Exec=%s daemon start
Icon=utilities-terminal
Terminal=false
Categories=Utility;Development;
StartupNotify=false
`, execPath)

		if err := os.MkdirAll(autostartDir, 0755); err != nil {
			return fmt.Errorf("falha ao criar pasta autostart: %w", err)
		}

		if err := os.WriteFile(desktopFilePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("falha ao gravar autostart .desktop: %w", err)
		}

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("falha ao obter home dir: %w", err)
		}

		agentsDir := filepath.Join(home, "Library", "LaunchAgents")
		plistPath := filepath.Join(agentsDir, "com.crom.agente.plist")

		if !enable {
			if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("falha ao remover autostart plist: %w", err)
			}
			return nil
		}

		content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.crom.agente</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`, execPath)

		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return fmt.Errorf("falha ao criar pasta LaunchAgents: %w", err)
		}

		if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("falha ao gravar autostart plist: %w", err)
		}

	case "windows":
		if !enable {
			cmd := exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "crom-agente", "/f")
			if err := cmd.Run(); err != nil {
				// Se a chave não existia, ignora
				return nil
			}
			return nil
		}

		val := fmt.Sprintf(`"%s" daemon start`, execPath)
		cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "crom-agente", "/t", "REG_SZ", "/d", val, "/f")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("falha ao adicionar chave no registro do Windows: %w", err)
		}

	default:
		return fmt.Errorf("sistema operacional %s nao suportado para autostart", runtime.GOOS)
	}

	return nil
}
