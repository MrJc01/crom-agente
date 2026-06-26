import sys
import glob

def patch_provider(path):
    with open(path, "r") as f:
        content = f.read()
    
    if "StreamMessages(" in content:
        return
        
    # Find the SendMessages function definition
    if "func (p *MockProvider) SendMessages(" in content:
        new_func = """func (p *MockProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)
	return p.SendMessages(ctx, messages, opts)
}
"""
        content += "\n" + new_func
        
    elif "func (p *RetryProvider) SendMessages(" in content:
        new_func = """func (p *RetryProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	// Implement retry for streams if needed, fallback to direct for now
	return p.provider.StreamMessages(ctx, messages, opts, chunkChan)
}
"""
        content += "\n" + new_func
        
    elif "func (p *AnthropicProvider) SendMessages(" in content:
        new_func = """func (p *AnthropicProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)
	// Fallback to non-streaming for now
	return p.SendMessages(ctx, messages, opts)
}
"""
        content += "\n" + new_func
        
    elif "func (p *GeminiProvider) SendMessages(" in content:
        new_func = """func (p *GeminiProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)
	// Fallback to non-streaming for now
	return p.SendMessages(ctx, messages, opts)
}
"""
        content += "\n" + new_func
        
    elif "func (p *OllamaProvider) SendMessages(" in content:
        new_func = """func (p *OllamaProvider) StreamMessages(ctx context.Context, messages []llm.Message, opts llm.RequestOptions, chunkChan chan<- string) (*llm.Response, error) {
	defer close(chunkChan)
	// Fallback to non-streaming for now
	return p.SendMessages(ctx, messages, opts)
}
"""
        content += "\n" + new_func

    with open(path, "w") as f:
        f.write(content)

for file in glob.glob("internal/llm/providers/*.go"):
    if not file.endswith("_test.go") and "openai.go" not in file and "factory.go" not in file:
        patch_provider(file)
