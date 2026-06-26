import re
with open('benchmark/run_all.py', 'r') as f:
    content = f.read()

content = content.replace(
    'if stats.get("limit_exceeded"):',
    'if not stats.get("exit_ok"):\n                success = False\n                status = "crashed"\n            \n            if stats.get("limit_exceeded"):'
)

with open('benchmark/run_all.py', 'w') as f:
    f.write(content)
