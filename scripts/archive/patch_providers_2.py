import sys

def patch_file(path, target, param):
    with open(path, "r") as f:
        content = f.read()

    if "StreamMessages(" in content:
        return

    new_func = ""
    if "retry" in path:
        new_func = f"""
func ({param} *{target}) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {{
	// For retry, we just delegate if the underlying provider supports it
	return {param}.provider.StreamMessages(ctx, messages, opts, chunkChan)
}}
"""
    else:
        new_func = f"""
func ({param} *{target}) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {{
	defer close(chunkChan)
	return {param}.SendMessages(ctx, messages, opts)
}}
"""
    
    content += new_func
    
    with open(path, "w") as f:
        f.write(content)

patch_file("internal/llm/providers/mock_provider.go", "MockProvider", "m")
patch_file("internal/llm/providers/retry_provider.go", "RetryProvider", "r")
