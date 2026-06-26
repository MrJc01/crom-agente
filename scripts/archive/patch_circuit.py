import sys

def patch_circuit():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    # Add 2 more unique responses
    old = """	resp1 := providers.MockTextResponse("Estou pensando sobre o arquivo...", 10)
	resp2 := providers.MockTextResponse("Ainda pensando em como criar o arquivo...", 10)
	resp3 := providers.MockTextResponse("Quase terminando de planejar o arquivo...", 10)

	provider := providers.NewMockProvider(resp1, resp2, resp3)"""
    new = """	resp1 := providers.MockTextResponse("Estou pensando sobre o arquivo...", 10)
	resp2 := providers.MockTextResponse("Ainda pensando em como criar o arquivo...", 10)
	resp3 := providers.MockTextResponse("Quase terminando de planejar o arquivo...", 10)
	resp4 := providers.MockTextResponse("Decidindo a estrutura final...", 10)
	resp5 := providers.MockTextResponse("Pronto para começar...", 10)

	provider := providers.NewMockProvider(resp1, resp2, resp3, resp4, resp5)"""
    
    content = content.replace(old, new)
    
    # CircuitBreaker_ReadOnly
    old2 = """	responses := []providers.MockResponse{}

	// 5 read_file
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"/tmp/a%d.txt"}`, i), 50))
	}
	// 5 cat
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("terminal_command", fmt.Sprintf(`{"command":"cat /tmp/a%d.txt"}`, i), 50))
	}"""
    # Wait, my fix_tests.py already changed read_file, let me just add uniqueness to cat too
    new2 = """	responses := []providers.MockResponse{}

	// 5 read_file
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"/tmp/a%d.txt"}`, i), 50))
	}
	// 5 cat
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("terminal_command", fmt.Sprintf(`{"command":"cat /tmp/a%d.txt"}`, i), 50))
	}
    // 5 more unique to avoid loop detector
	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockTextResponse(fmt.Sprintf("unique %d", i), 50))
	}"""
    content = content.replace(old2, new2)

    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_circuit()
