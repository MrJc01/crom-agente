import re

with open("internal/loop/agentic/core/execute.go", "r") as f:
    code = f.read()

old_data = """
                        Data: map[string]interface{}{
                                "provider": al.provider.Name(),
                                "model":    al.config.Model,
                        },
"""

new_data = """
                        Data: map[string]interface{}{
                                "provider": al.provider.Name(),
                                "model":    al.config.Model,
                                "cost":     al.stateManager.GetState().CustoTotalUSD,
                        },
"""

code = code.replace(old_data, new_data)
with open("internal/loop/agentic/core/execute.go", "w") as f:
    f.write(code)

with open("internal/cli/root.go", "r") as f:
    code_root = f.read()

old_think = """
        case "thinking":
                provider, _ := event.Data["provider"].(string)
                model, _ := event.Data["model"].(string)
                fmt.Fprintf(h.out, "  💭 [iter %d] Pensando... (%s/%s)\\n", event.Iteration, provider, model)
"""

new_think = """
        case "thinking":
                provider, _ := event.Data["provider"].(string)
                model, _ := event.Data["model"].(string)
                cost, _ := event.Data["cost"].(float64)
                fmt.Fprintf(h.out, "  💭 [iter %d] Pensando... (%s/%s) - %sCost: $%.4f%s\\n", event.Iteration, provider, model, ColorYellow, cost, ColorReset)
"""

code_root = code_root.replace(old_think.strip(), new_think.strip())

with open("internal/cli/root.go", "w") as f:
    f.write(code_root)

