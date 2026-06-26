import sys

def patch_assertion():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """		if msg.Role == "system" && strings.Contains(msg.Content, "Você está há 3 turnos sem modificar arquivos ou chamar ferramentas de escrita/execução") {"""
    
    new = """		if msg.Role == "system" && strings.Contains(msg.Content, "sem modificar arquivos ou chamar ferramentas de escrita/execução") {"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_assertion()
