import sys
import re
import os

def process_file(path):
    if not os.path.exists(path): return
    with open(path, "r") as f:
        content = f.read()

    # Find func (h *testEventHandler) OnMessage(role, content string) {
    # and insert OnStreamChunk right after it.
    
    if "OnStreamChunk(" not in content:
        old = "func (h *testEventHandler) OnMessage(role, content string) {"
        new = "func (h *testEventHandler) OnStreamChunk(chunk string) {}\n\nfunc (h *testEventHandler) OnMessage(role, content string) {"
        
        # also handle func (t *testEventHandler) OnMessage(string, string) {}
        if "func (t *testEventHandler) OnMessage(string, string)      {}" in content:
            old2 = "func (t *testEventHandler) OnMessage(string, string)      {}"
            new2 = "func (t *testEventHandler) OnStreamChunk(string)          {}\nfunc (t *testEventHandler) OnMessage(string, string)      {}"
            content = content.replace(old2, new2)
        
        content = content.replace(old, new)
        
        with open(path, "w") as f:
            f.write(content)

process_file("internal/loop/agentic/core/agentic_loop_test.go")
process_file("internal/loop/agentic/core/agent_event_test.go")
process_file("internal/orchestrator/manager_test.go")

