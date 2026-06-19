package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// ComputerControlTool permite simular ações físicas de teclado e mouse e capturar telas
type ComputerControlTool struct {
	workspacePath string
}

// NewComputerControlTool cria uma nova instância da ferramenta
func NewComputerControlTool(workspacePath string) *ComputerControlTool {
	return &ComputerControlTool{
		workspacePath: workspacePath,
	}
}

// ID retorna o identificador único
func (c *ComputerControlTool) ID() string {
	return "computer_control"
}

// Description descreve a ferramenta para o LLM
func (c *ComputerControlTool) Description() string {
	return "Controla a interface gráfica do sistema operacional local (simula mouse, cliques, digitação e captura tela/screenshots)."
}

// ParametersSchema define o JSON Schema dos parâmetros aceitos
func (c *ComputerControlTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["mouse_move", "mouse_click", "key_type", "key_press", "screenshot"],
				"description": "Ação de interface gráfica a executar"
			},
			"x": {
				"type": "integer",
				"description": "Coordenada X na tela (necessário para mouse_move e mouse_click)"
			},
			"y": {
				"type": "integer",
				"description": "Coordenada Y na tela (necessário para mouse_move e mouse_click)"
			},
			"text": {
				"type": "string",
				"description": "Texto para digitar (para key_type) ou tecla para pressionar (ex: 'Return', 'space' para key_press)"
			},
			"path": {
				"type": "string",
				"description": "Caminho opcional do arquivo para salvar a captura de tela (ex: 'screenshot.png'). Se especificado, grava a imagem diretamente no disco no caminho informado."
			}
		},
		"required": ["action"]
	}`)
}

// RequiresApproval indica que ações que tomam controle da máquina do usuário exigem HITL por segurança.
func (c *ComputerControlTool) RequiresApproval() bool {
	return true
}

// Execute despacha a ação conforme o sistema operacional hospedeiro
func (c *ComputerControlTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Action string `json:"action"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Text   string `json:"text"`
		Path   string `json:"path"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return Result{Success: false, Error: "argumentos inválidos"}, err
	}

	switch params.Action {
	case "screenshot":
		return c.takeScreenshot(ctx, params.Path)
	case "mouse_move":
		return c.mouseMove(ctx, params.X, params.Y)
	case "mouse_click":
		return c.mouseClick(ctx, params.X, params.Y)
	case "key_type":
		if params.Text == "" {
			return Result{Success: false, Error: "parâmetro 'text' é obrigatório para a ação 'key_type'"}, nil
		}
		return c.keyType(ctx, params.Text)
	case "key_press":
		if params.Text == "" {
			return Result{Success: false, Error: "parâmetro 'text' (tecla) é obrigatório para a ação 'key_press'"}, nil
		}
		return c.keyPress(ctx, params.Text)
	default:
		return Result{Success: false, Error: fmt.Sprintf("ação de GUI desconhecida: %s", params.Action)}, nil
	}
}

func (c *ComputerControlTool) takeScreenshot(ctx context.Context, savePath string) (Result, error) {
	tempFile := filepath.Join(os.TempDir(), "crom_screenshot.png")
	defer os.Remove(tempFile)

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		// Tenta xwd, scrot ou gnome-screenshot
		if _, err := exec.LookPath("scrot"); err == nil {
			cmd = exec.CommandContext(ctx, "scrot", "-z", tempFile)
		} else if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			cmd = exec.CommandContext(ctx, "gnome-screenshot", "-f", tempFile)
		} else {
			return Result{Success: false, Error: "utilitários 'scrot' ou 'gnome-screenshot' não encontrados. Instale-os para habilitar capturas no Linux."}, nil
		}
	case "darwin":
		cmd = exec.CommandContext(ctx, "screencapture", "-x", tempFile)
	case "windows":
		psCommand := fmt.Sprintf(`
			Add-Type -AssemblyName System.Windows.Forms
			Add-Type -AssemblyName System.Drawing
			$screen = [System.Windows.Forms.Screen]::PrimaryScreen
			$bounds = $screen.Bounds
			$bitmap = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
			$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
			$graphics.CopyFromScreen($bounds.X, $bounds.Y, 0, 0, $bounds.Size)
			$bitmap.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)
			$graphics.Dispose()
			$bitmap.Dispose()
		`, tempFile)
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCommand)
	default:
		return Result{Success: false, Error: fmt.Sprintf("captura de tela não suportada no SO: %s", runtime.GOOS)}, nil
	}

	if err := cmd.Run(); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao rodar utilitário de captura: %v", err)}, nil
	}

	imgBytes, err := os.ReadFile(tempFile)
	if err != nil {
		return Result{Success: false, Error: fmt.Sprintf("erro ao ler arquivo de captura: %v", err)}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	if savePath != "" {
		targetFile, err := ValidatePath(c.workspacePath, savePath, false)
		if err != nil {
			return Result{Success: false, Error: fmt.Sprintf("caminho de destino inválido: %v", err)}, nil
		}
		if err := os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao criar diretórios pai: %v", err)}, nil
		}
		if err := os.WriteFile(targetFile, imgBytes, 0644); err != nil {
			return Result{Success: false, Error: fmt.Sprintf("falha ao salvar screenshot no disco: %v", err)}, nil
		}
		return Result{Success: true, Data: "image:base64:" + b64 + "\n✓ Screenshot salvo em: " + savePath}, nil
	}

	return Result{Success: true, Data: "image:base64:" + b64}, nil
}

func (c *ComputerControlTool) mouseMove(ctx context.Context, x, y int) (Result, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("xdotool"); err != nil {
			return Result{Success: false, Error: "utilitário 'xdotool' necessário no Linux. Instale-o para controle de GUI."}, nil
		}
		cmd = exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	case "darwin":
		// Script em AppleScript para mover o mouse via CoreGraphics
		osaCommand := fmt.Sprintf("tell application \"System Events\" to click at {%d, %d}", x, y)
		cmd = exec.CommandContext(ctx, "osascript", "-e", osaCommand)
	case "windows":
		psCommand := fmt.Sprintf(`
			$assembly = Add-Type -MemberDefinition @'
			[DllImport("user32.dll")]
			public static extern bool SetCursorPos(int X, int Y);
'@ -Name "User32Mouse" -Namespace "Win32" -PassThru
			[Win32.User32Mouse]::SetCursorPos(%d, %d)
		`, x, y)
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCommand)
	default:
		return Result{Success: false, Error: fmt.Sprintf("mouse_move não suportado no SO: %s", runtime.GOOS)}, nil
	}

	if err := cmd.Run(); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao mover mouse: %v", err)}, nil
	}
	return Result{Success: true, Data: fmt.Sprintf("Mouse movido para (%d, %d)", x, y)}, nil
}

func (c *ComputerControlTool) mouseClick(ctx context.Context, x, y int) (Result, error) {
	// Primeiro move, depois clica
	if _, err := c.mouseMove(ctx, x, y); err != nil {
		return Result{Success: false, Error: err.Error()}, err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.CommandContext(ctx, "xdotool", "click", "1")
	case "darwin":
		// Darwin clica direto no mouseMove (tell application System Events to click at)
		return Result{Success: true, Data: fmt.Sprintf("Mouse clicado em (%d, %d)", x, y)}, nil
	case "windows":
		psCommand := `
			$assembly = Add-Type -MemberDefinition @'
			[DllImport("user32.dll")]
			public static extern void mouse_event(int dwFlags, int dx, int dy, int cButtons, int dwExtraInfo);
'@ -Name "User32Click" -Namespace "Win32" -PassThru
			# MOUSEEVENTF_LEFTDOWN = 0x0002, MOUSEEVENTF_LEFTUP = 0x0004
			[Win32.User32Click]::mouse_event(0x0002, 0, 0, 0, 0)
			Start-Sleep -Milliseconds 50
			[Win32.User32Click]::mouse_event(0x0004, 0, 0, 0, 0)
		`
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCommand)
	default:
		return Result{Success: false, Error: fmt.Sprintf("mouse_click não suportado no SO: %s", runtime.GOOS)}, nil
	}

	if err := cmd.Run(); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao clicar mouse: %v", err)}, nil
	}
	return Result{Success: true, Data: fmt.Sprintf("Mouse clicado com sucesso em (%d, %d)", x, y)}, nil
}

func (c *ComputerControlTool) keyType(ctx context.Context, text string) (Result, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("xdotool"); err != nil {
			return Result{Success: false, Error: "utilitário 'xdotool' necessário no Linux. Instale-o para controle de GUI."}, nil
		}
		cmd = exec.CommandContext(ctx, "xdotool", "type", text)
	case "darwin":
		osaCommand := fmt.Sprintf("tell application \"System Events\" to keystroke %q", text)
		cmd = exec.CommandContext(ctx, "osascript", "-e", osaCommand)
	case "windows":
		psCommand := fmt.Sprintf("[System.Windows.Forms.SendKeys]::SendWait('%s')", text)
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCommand)
	default:
		return Result{Success: false, Error: fmt.Sprintf("key_type não suportado no SO: %s", runtime.GOOS)}, nil
	}

	if err := cmd.Run(); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao digitar teclas: %v", err)}, nil
	}
	return Result{Success: true, Data: fmt.Sprintf("Texto %q digitado com sucesso", text)}, nil
}

func (c *ComputerControlTool) keyPress(ctx context.Context, key string) (Result, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("xdotool"); err != nil {
			return Result{Success: false, Error: "utilitário 'xdotool' necessário no Linux. Instale-o para controle de GUI."}, nil
		}
		cmd = exec.CommandContext(ctx, "xdotool", "key", key)
	case "darwin":
		// Mapeia chaves comuns para AppleScript
		keyName := key
		if key == "Return" || key == "enter" {
			keyName = "return"
		}
		osaCommand := fmt.Sprintf("tell application \"System Events\" to key code (key code of %s)", keyName)
		if keyName == "return" || keyName == "space" {
			osaCommand = fmt.Sprintf("tell application \"System Events\" to keystroke %s", keyName)
		}
		cmd = exec.CommandContext(ctx, "osascript", "-e", osaCommand)
	case "windows":
		// Mapeia chaves comuns para SendKeys do .NET
		sendKey := key
		if key == "Return" || key == "enter" {
			sendKey = "{ENTER}"
		} else if key == "space" {
			sendKey = " "
		}
		psCommand := fmt.Sprintf("[System.Windows.Forms.SendKeys]::SendWait('%s')", sendKey)
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCommand)
	default:
		return Result{Success: false, Error: fmt.Sprintf("key_press não suportado no SO: %s", runtime.GOOS)}, nil
	}

	if err := cmd.Run(); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("falha ao pressionar tecla: %v", err)}, nil
	}
	return Result{Success: true, Data: fmt.Sprintf("Tecla %s pressionada com sucesso", key)}, nil
}
