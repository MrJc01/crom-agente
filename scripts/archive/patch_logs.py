import re

with open('benchmark/run_all.py', 'r') as f:
    content = f.read()

# Modify run_agent definition
content = content.replace(
    'def run_agent(prompt, workspace, max_iter=MAX_ITER, timeout=TIMEOUT, use_docker=False):',
    'def run_agent(prompt, workspace, max_iter=MAX_ITER, timeout=TIMEOUT, use_docker=False, task_id="unknown"):'
)

# Modify run_agent return
content = content.replace(
    '    state["elapsed_seconds"] = elapsed\n    state["exit_ok"] = exit_ok\n    state["output"] = output\n    return state',
    '    state["elapsed_seconds"] = elapsed\n    state["exit_ok"] = exit_ok\n    state["output"] = output\n    \n    logs_dir = REPORTS_DIR / "logs"\n    logs_dir.mkdir(exist_ok=True)\n    safe_tid = str(task_id).replace("/", "_")\n    log_path = logs_dir / f"{safe_tid}.log"\n    with open(log_path, "w") as lf:\n        lf.write(output)\n        \n    return state'
)

# Fix calls to run_agent
content = re.sub(
    r'run_agent\((.*?)(timeout=TIMEOUT)(.*?)\)',
    r'run_agent(\1\2\3, task_id=tid)',
    content
)

with open('benchmark/run_all.py', 'w') as f:
    f.write(content)
