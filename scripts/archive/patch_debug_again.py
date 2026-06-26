import sys

def patch_debug_again():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()
    
    old = "foundSystemWarning := false"
    new = """fmt.Printf("MESSAGES DUMP:\\n")
        for _, msg := range sm.GetMessages() {
            fmt.Printf("Role: %s, Content: %s\\n", msg.Role, msg.Content)
        }
        foundSystemWarning := false"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_debug_again()
