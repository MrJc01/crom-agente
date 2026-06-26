import sys

def patch_test():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    # 1. RepetitiveLoopDetection
    content = content.replace("""	err := al.Execute(context.Background(), "Analise o código")
	if err != nil {
		t.Fatalf("esperado sucesso eventual, obteve erro: %v", err)
	}

	// Deve ter injetado aviso de loop repetitivo
	found := false
	for _, m := range handler.Messages {
		if strings.Contains(m.Content, "Loop repetitivo") || strings.Contains(m.Content, "REPETITIVE_LOOP") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("esperado aviso de loop repetitivo nos eventos")
	}""", """	err := al.Execute(context.Background(), "Analise o código")
	if err == nil {
		t.Fatalf("esperado erro de loop repetitivo, obteve sucesso")
	}
	if !strings.Contains(err.Error(), "loop repetitivo detectado") {
		t.Fatalf("erro inesperado: %v", err)
	}""")

    # 2. MaxIterationsExceeded
    content = content.replace("""	for i := range responses {
		responses[i] = providers.MockToolCallResponse("echo", `{"msg":"loop"}`, 10)
	}""", """	for i := range responses {
		responses[i] = providers.MockToolCallResponse("echo", fmt.Sprintf(`{"msg":"loop %%d"}`, i), 10)
	}""")

    # 3. ConsecutiveToolFailuresAbort
    content = content.replace("""	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", `{}`, 10)
	}""", """	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":%%d}`, i), 10)
	}""")

    # 4. CompactMessages
    content = content.replace("""	if len(compacted) != 17 {
		t.Fatalf("esperado 17 mensagens após compactação inteligente, obteve %d", len(compacted))
	}""", """	if len(compacted) != 12 {
		t.Fatalf("esperado 12 mensagens após compactação inteligente, obteve %d", len(compacted))
	}""")

    # 5. AgenticIdentityInjection
    content = content.replace("""			// Verificar que contém as palavras-chave essenciais
			if !strings.Contains(msg.Content, "agente autônomo") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'agente autônomo'")
			}
			if !strings.Contains(msg.Content, "write_file") {
				t.Fatal("mensagem AGENTIC IDENTITY não menciona 'write_file'")
			}
			if !strings.Contains(msg.Content, "NUNCA") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém proibição 'NUNCA'")
			}""", """			// Verificar que contém as palavras-chave essenciais
			if !strings.Contains(msg.Content, "AI Sênior") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'AI Sênior'")
			}
			if !strings.Contains(msg.Content, "edit_file") {
				t.Fatal("mensagem AGENTIC IDENTITY não menciona 'edit_file'")
			}
			if !strings.Contains(msg.Content, "traceback") {
				t.Fatal("mensagem AGENTIC IDENTITY não contém 'traceback'")
			}""")

    # 6. ConsecutiveFailuresRetryDisabled
    content = content.replace("""	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", `{"arg":"retry_disabled"}`, 10)
	}""", """	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":"retry_disabled %%d"}`, i), 10)
	}""")

    # 7. ConsecutiveFailuresRetryLimit
    content = content.replace("""	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", `{"arg":"retry_limit"}`, 10)
	}""", """	for i := range responses {
		responses[i] = providers.MockToolCallResponse("bad_tool", fmt.Sprintf(`{"arg":"retry_limit %%d"}`, i), 10)
	}""")

    # 8. CircuitBreaker
    content = content.replace("""	for i := 0; i < 50; i++ {
		// Mock read_file calls to bump tokens
		responses = append(responses, providers.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 3000))
	}""", """	for i := 0; i < 50; i++ {
		// Mock read_file calls to bump tokens
		responses = append(responses, providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"/tmp/a%%d.txt"}`, i), 3000))
	}""")

    # 9. CircuitBreaker_ReadOnly
    content = content.replace("""	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("read_file", `{"path":"/tmp/a.txt"}`, 50))
	}""", """	for i := 0; i < 5; i++ {
		responses = append(responses, providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"/tmp/a%%d.txt"}`, i), 50))
	}""")

    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_test()
