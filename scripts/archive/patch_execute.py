import sys

def patch_execute():
    with open("internal/loop/agentic/core/execute.go", "r") as f:
        content = f.read()

    old = """		finalMsgs := FormatMessagesForModel(compactedMsgs, al.provider)
		resp, err := al.provider.SendMessages(ctx, finalMsgs, opts)"""

    new = """		finalMsgs := FormatMessagesForModel(compactedMsgs, al.provider)
		
		var resp *llm.Response
		var err error
		
		if true { // Padrão: tentar usar streaming sempre
			chunkChan := make(chan string, 100)
			go func() {
				for chunk := range chunkChan {
					al.handler.OnStreamChunk(chunk)
				}
			}()
			resp, err = al.provider.StreamMessages(ctx, finalMsgs, opts, chunkChan)
		} else {
			resp, err = al.provider.SendMessages(ctx, finalMsgs, opts)
		}"""

    if "chunkChan :=" not in content:
        content = content.replace(old, new)
        with open("internal/loop/agentic/core/execute.go", "w") as f:
            f.write(content)

patch_execute()
