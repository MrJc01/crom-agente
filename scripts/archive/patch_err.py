import sys

def patch_err():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        if err == nil {
                t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
        }"""
    
    new = """        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        if err == nil {
                t.Fatal("esperava erro de limite de iterações atingido, mas o loop concluiu com sucesso")
        }
        fmt.Printf("DEBUG ERR: %v\\n", err)"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_err()
