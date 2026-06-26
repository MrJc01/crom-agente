import sys

def patch_readonly():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """	// 5 cat
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("terminal_command", fmt.Sprintf(`{"command":"cat /tmp/a%d.txt"}`, i), 50))
	}"""
    
    new = """	// 5 mais read_file
	for i := 5; i < 10; i++ {
		responses = append(responses, providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"/tmp/a%d.txt"}`, i), 50))
	}"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_readonly()
