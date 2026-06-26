import re

with open("internal/cli/root.go", "r") as f:
    code = f.read()

colors_decl = """
const (
        ColorReset  = "\\033[0m"
        ColorRed    = "\\033[31m"
        ColorGreen  = "\\033[32m"
        ColorYellow = "\\033[33m"
        ColorBlue   = "\\033[34m"
        ColorCyan   = "\\033[36m"
)
"""

if "ColorReset" not in code:
    # Insert right before cliEventHandler type
    code = code.replace("type cliEventHandler struct", colors_decl + "\ntype cliEventHandler struct")

old_onstatus = """func (h *cliEventHandler) OnStatusChange(status string) {
        fmt.Fprintf(h.out, " [status] %s...\\n", status)
}"""

new_onstatus = """func (h *cliEventHandler) OnStatusChange(status string) {
        color := ColorCyan
        if strings.Contains(strings.ToLower(status), "falh") || strings.Contains(strings.ToLower(status), "err") {
                color = ColorRed
        } else if strings.Contains(strings.ToLower(status), "sucesso") || strings.Contains(strings.ToLower(status), "ok") {
                color = ColorGreen
        } else if strings.Contains(strings.ToLower(status), "loop") || strings.Contains(strings.ToLower(status), "retry") {
                color = ColorYellow
        }
        fmt.Fprintf(h.out, "%s [status] %s...%s\\n", color, status, ColorReset)
}"""

code = code.replace(old_onstatus, new_onstatus)

# Now update OnEvent
old_onevent = """
        case "tool_result":
                toolName, _ := event.Data["tool"].(string)
                success, _ := event.Data["success"].(bool)
                if success {
                        fmt.Fprintf(h.out, "  ✓ [iter %d] Ferramenta '%s' retornou sucesso\n", event.Iteration, toolName)
                } else {
                        fmt.Fprintf(h.out, "  ✗ [iter %d] Ferramenta '%s' FALHOU\n", event.Iteration, toolName)
                }
"""

new_onevent = """
        case "tool_result":
                toolName, _ := event.Data["tool"].(string)
                success, _ := event.Data["success"].(bool)
                if success {
                        fmt.Fprintf(h.out, "%s  ✓ [iter %d] Ferramenta '%s' retornou sucesso%s\\n", ColorGreen, event.Iteration, toolName, ColorReset)
                } else {
                        fmt.Fprintf(h.out, "%s  ✗ [iter %d] Ferramenta '%s' FALHOU%s\\n", ColorRed, event.Iteration, toolName, ColorReset)
                }
"""
code = code.replace(old_onevent.strip(), new_onevent.strip())

# also update loop threshold warning
old_loop = """
        case "loop_threshold_warning":
                fmt.Fprintf(h.out, "  ⚠️  [iter %d] ALERTA: Modelo parece estar em loop. Temperatura aumentada.\n", event.Iteration)
"""

new_loop = """
        case "loop_threshold_warning":
                fmt.Fprintf(h.out, "%s  ⚠️  [iter %d] ALERTA: Modelo parece estar em loop. Temperatura aumentada.%s\\n", ColorYellow, event.Iteration, ColorReset)
"""
code = code.replace(old_loop.strip(), new_loop.strip())

with open("internal/cli/root.go", "w") as f:
    f.write(code)

