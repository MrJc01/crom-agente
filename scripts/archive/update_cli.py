import sys

def process_file(path):
    with open(path, "r") as f:
        content = f.read()

    old = "func (h *cliEventHandler) OnMessage(role string, content string) {"
    new = """func (h *cliEventHandler) OnStreamChunk(chunk string) {
	fmt.Print(chunk)
}

func (h *cliEventHandler) OnMessage(role string, content string) {"""

    if "OnStreamChunk(" not in content:
        content = content.replace(old, new)
        
        with open(path, "w") as f:
            f.write(content)

process_file("internal/cli/root.go")
