import sys

def patch_debug_test():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """        for _, msg := range sm.GetMessages() {
                if msg.Role == "system" && strings.Contains(msg.Content, "sem modificar arquivos ou chamar ferramentas de escrita/execução") {
                        foundSystemWarning = true
                        break
                }
        }"""
    
    new = """        for _, msg := range sm.GetMessages() {
                if msg.Role == "system" {
                        fmt.Printf("DEBUG SYSTEM MSG: %s\\n", msg.Content)
                }
                if msg.Role == "system" && strings.Contains(msg.Content, "sem modificar arquivos ou chamar ferramentas de escrita/execução") {
                        foundSystemWarning = true
                        break
                }
        }"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_debug_test()
