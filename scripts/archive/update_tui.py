import sys

def process_file(path):
    with open(path, "r") as f:
        content = f.read()

    old = "func (h *tuiEventHandler) OnMessage(role string, content string) {"
    new = """func (h *tuiEventHandler) OnStreamChunk(chunk string) {
}

func (h *tuiEventHandler) OnMessage(role string, content string) {"""

    if "OnStreamChunk(" not in content:
        content = content.replace(old, new)
        with open(path, "w") as f:
            f.write(content)

process_file("pkg/cli-tui/loop_handler.go")
