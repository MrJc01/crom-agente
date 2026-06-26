import sys

def patch_loop():
    with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
        content = f.read()

    old = """func TestAgenticLoop_CircuitBreaker_ReadOnly(t *testing.T) {
        // 3 responses with tool calls, but they are all read-only (e.g. read_file)
        resp1 := providers.MockToolCallResponse("read_file", `{"path":"somefile1.txt"}`, 10)
        resp2 := providers.MockToolCallResponse("read_file", `{"path":"somefile2.txt"}`, 10)
        resp3 := providers.MockToolCallResponse("read_file", `{"path":"somefile3.txt"}`, 10)
        resp4 := providers.MockToolCallResponse("read_file", `{"path":"somefile4.txt"}`, 10)
        resp5 := providers.MockToolCallResponse("read_file", `{"path":"somefile5.txt"}`, 10)

        provider := providers.NewMockProvider(resp1, resp2, resp3, resp4, resp5)"""
    
    new = """func TestAgenticLoop_CircuitBreaker_ReadOnly(t *testing.T) {
        responses := make([]providers.MockResponse, 10)
        for i := range responses {
                responses[i] = providers.MockToolCallResponse("read_file", fmt.Sprintf(`{"path":"somefile%d.txt"}`, i), 10)
        }
        provider := providers.NewMockProvider(responses...)"""
    
    content = content.replace(old, new)
    
    with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
        f.write(content)

patch_loop()
