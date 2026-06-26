import sys

def patch_err():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        if err == nil {"""
    
    new = """        err := al.Execute(context.Background(), "Modifique o arquivo.py")
        fmt.Printf("DEBUG ERR RETURNED: %v\\n", err)
        if err == nil {"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_err()
