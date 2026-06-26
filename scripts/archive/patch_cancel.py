import sys

def process_file(path):
    with open(path, "r") as f:
        content = f.read()

    old = """func (h *cancelOnRetryHandler) OnMessage(role, content string) {"""
    new = """func (h *cancelOnRetryHandler) OnStreamChunk(chunk string) {}
func (h *cancelOnRetryHandler) OnMessage(role, content string) {"""

    if "OnStreamChunk(" not in content:
        content = content.replace(old, new)
        with open(path, "w") as f:
            f.write(content)

process_file("internal/loop/agentic/core/agentic_loop_test.go")
process_file("internal/loop/agentic/core/agent_event_test.go")
