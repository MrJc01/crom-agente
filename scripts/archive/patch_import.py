import sys
import re

def patch_import():
    with open("internal/loop/agentic/core/agent_event_test.go", "r") as f:
        content = f.read()

    # Find the imports block and add fmt if not there
    if '"fmt"' not in content:
        content = re.sub(r'import \(', 'import (\n\t"fmt"', content, count=1)
        
    with open("internal/loop/agentic/core/agent_event_test.go", "w") as f:
        f.write(content)

patch_import()
