import sys

def patch_err():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()
    
    new = """        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        fmt.Printf("DEBUG ERR: %v\\n", err)
        for _, m := range handler.Messages {
            fmt.Printf("DEBUG HANDLER MSG: %s - %s\\n", m.Role, m.Content)
        }
        if err == nil {
                t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
        }"""
    
    content = content.replace("""        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        if err == nil {
                t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
        }
        fmt.Printf("DEBUG ERR: %v\\n", err)""", new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_err()
