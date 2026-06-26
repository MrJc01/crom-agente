#!/usr/bin/env python3
import os
import json
import urllib.request
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent
REPORTS_DIR = Path(__file__).resolve().parent / "reports"
PROGRESS_FILE = REPORTS_DIR / "benchmark_progress.json"
LOGS_DIR = REPORTS_DIR / "logs"

# Use OpenRouter via environment variables (which run_all.py already loads)
API_KEY = os.environ.get("OPENROUTER_API_KEY", "")

def get_classification(log_content):
    if not API_KEY:
        return "N/A (Missing OPENROUTER_API_KEY)"
        
    prompt = f"""You are an expert software engineering judge. Analyze the end of the following execution log of an AI coding agent.
Identify the primary reason for failure and classify it into ONE of these categories:
- TIMEOUT (Agent got stuck or took too long)
- SYNTAX_ERROR (Generated code has syntax errors)
- LOGIC_ERROR (Failed unit tests or incorrect logic)
- TOOL_USAGE_ERROR (Agent failed to use terminal tools correctly)
- CONTEXT_LIMIT (Agent exceeded token limit)
- CRASH (Agent process crashed)

Log tail:
...
{log_content[-2000:]}
...

Reply ONLY with the exact category name. Nothing else."""

    data = json.dumps({
        "model": "meta-llama/llama-3.1-8b-instruct",
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.0,
        "max_tokens": 10
    }).encode("utf-8")
    
    req = urllib.request.Request("https://openrouter.ai/api/v1/chat/completions", data=data, headers={
        "Content-Type": "application/json",
        "Authorization": f"Bearer {API_KEY}"
    })
    
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            result = json.loads(response.read().decode())
            return result["choices"][0]["message"]["content"].strip()
    except Exception as e:
        return f"ERROR_CLASSIFYING: {e}"

def main():
    if not PROGRESS_FILE.exists():
        print("Progress file not found. Run benchmark first.")
        return

    with open(PROGRESS_FILE, "r") as f:
        progress = json.load(f)

    changed = False
    for bench_name, tasks in progress.items():
        for task_id, result in tasks.items():
            if not result.get("success", False) and "failure_reason" not in result:
                safe_tid = str(task_id).replace("/", "_")
                log_file = LOGS_DIR / f"{safe_tid}.log"
                if log_file.exists():
                    print(f"Classifying failure for {task_id}...")
                    with open(log_file, "r", errors="replace") as lf:
                        content = lf.read()
                        
                    reason = get_classification(content)
                    print(f" -> {reason}")
                    result["failure_reason"] = reason
                    changed = True

    if changed:
        with open(PROGRESS_FILE, "w") as f:
            json.dump(progress, f, indent=2)
        print("Progress file updated with failure classifications.")
    else:
        print("No new failures to classify.")

if __name__ == "__main__":
    main()
