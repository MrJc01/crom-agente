import re

with open("internal/cli/root.go", "r") as f:
    code = f.read()

old_event = """
        case "error":
                errMsg, _ := event.Data["message"].(string)
                fmt.Fprintf(h.out, "  ❌ Erro: %s\\n", errMsg)
"""

new_event = """
        case "error":
                errMsg, _ := event.Data["message"].(string)
                fmt.Fprintf(h.out, "  ❌ Erro: %s\\n", errMsg)
        case "finished":
                // Create a final visual dump of the diff
                fmt.Fprintln(h.out, "\\n=======================================================")
                fmt.Fprintln(h.out, "              🔍 Resumo das Modificações")
                fmt.Fprintln(h.out, "=======================================================")
                
                cmd := exec.Command("bash", "-c", "git diff --stat && echo '' && git diff --color=always")
                cmd.Stdout = h.out
                cmd.Stderr = h.out
                cmd.Run()
"""

if "case \"finished\":" not in code:
    code = code.replace(old_event.strip(), new_event.strip())

    # We might need to add os/exec to imports if not there
    if '"os/exec"' not in code:
        code = code.replace('import (', 'import (\n\t"os/exec"', 1)

    with open("internal/cli/root.go", "w") as f:
        f.write(code)
