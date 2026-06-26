import sys

def process_file(path, target):
    with open(path, "r") as f:
        content = f.read()

    old = f"func (h *{target}) OnMessage(role string, content string) {{"
    new = f"""func (h *{target}) OnStreamChunk(chunk string) {{
}}

func (h *{target}) OnMessage(role string, content string) {{"""

    if "OnStreamChunk(" not in content:
        content = content.replace(old, new)
        with open(path, "w") as f:
            f.write(content)

process_file("pkg/sdk/agent.go", "sdkEventWaitHandler")
process_file("pkg/cli-tui/ui.go", "tuiEventHandler")
