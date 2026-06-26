import sys

def process_file(path):
    with open(path, "r") as f:
        content = f.read()

    old = "func (h *daemonAPIEventHandler) OnMessage(role string, content string) {"
    new = """func (h *daemonAPIEventHandler) OnStreamChunk(chunk string) {
	evt := AgentEventPayload{
		Type:    "stream_chunk",
		Content: chunk,
	}
	h.sendEvent(evt)
}

func (h *daemonAPIEventHandler) OnMessage(role string, content string) {"""

    if "OnStreamChunk(" not in content:
        content = content.replace(old, new)
        
        with open(path, "w") as f:
            f.write(content)

process_file("internal/daemon/server.go")
