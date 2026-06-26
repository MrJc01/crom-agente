import os

# Patch subagent_spawner.go
with open("internal/loop/agentic/core/subagent_spawner.go", "r") as f:
    content = f.read()
content = content.replace("rollbackGit(workspaceDir)", "tooling.RollbackGit(workspaceDir)")
content = content.replace('"github.com/crom/crom-agente/internal/tools"', '"github.com/crom/crom-agente/internal/tools"\n\t"github.com/crom/crom-agente/internal/loop/agentic/tooling"')
with open("internal/loop/agentic/core/subagent_spawner.go", "w") as f:
    f.write(content)

# Patch execute.go to add truncateStr
with open("internal/loop/agentic/core/execute.go", "r") as f:
    content = f.read()
if "func truncateStr" not in content:
    content += "\n// truncateStr trunca uma string\nfunc truncateStr(s string, max int) string {\n\tif len(s) > max {\n\t\treturn s[:max] + \"...\"\n\t}\n\treturn s\n}\n"
with open("internal/loop/agentic/core/execute.go", "w") as f:
    f.write(content)

# Patch agentic_loop_test.go
with open("internal/loop/agentic/core/agentic_loop_test.go", "r") as f:
    content = f.read()
content = content.replace("al.compactMessages(ctx, messages)", "prompting.CompactMessages(ctx, al.provider, al.config.MaxMessageHistory, al.handler, messages)")
content = content.replace("al.compactMessages(context.Background(), messages)", "prompting.CompactMessages(context.Background(), al.provider, al.config.MaxMessageHistory, al.handler, messages)")
content = content.replace("detectRepetitiveLoop(messages)", "DetectRepetitiveLoop(messages)")
content = content.replace("rollbackGit(wsDir)", "tooling.RollbackGit(wsDir)")
content = content.replace('"github.com/crom/crom-agente/internal/state"', '"github.com/crom/crom-agente/internal/state"\n\t"github.com/crom/crom-agente/internal/loop/agentic/prompting"\n\t"github.com/crom/crom-agente/internal/loop/agentic/tooling"')
with open("internal/loop/agentic/core/agentic_loop_test.go", "w") as f:
    f.write(content)

