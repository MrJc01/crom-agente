import sys

def patch():
    with open("internal/loop/agentic/core/execute.go", "r") as f:
        content = f.read()
    
    debug_code = """
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			fmt.Printf(">>> DEBUG: Empty response detected! consecutiveFailures before: %d\\n", consecutiveFailures)
"""
    
    content = content.replace(
        "if strings.TrimSpace(msg.Content) == \"\" && len(msg.ToolCalls) == 0 {",
        "if strings.TrimSpace(msg.Content) == \"\" && len(msg.ToolCalls) == 0 {\n\t\t\tfmt.Printf(\">>> DEBUG: Empty response detected! consecutiveFailures before: %d\\n\", consecutiveFailures)"
    )
    
    with open("internal/loop/agentic/core/execute.go", "w") as f:
        f.write(content)

patch()
