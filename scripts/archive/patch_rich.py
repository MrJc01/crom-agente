import sys
import re

with open("benchmark/run_all.py", "r") as f:
    code = f.read()

# 1. Inject imports and the ProgressWrapper at the top (after import threading)
wrapper_code = """
try:
    from rich.progress import Progress, SpinnerColumn, TextColumn, BarColumn, TaskProgressColumn, TimeElapsedColumn
    from rich.console import Console
    RICH_AVAILABLE = True
    console = Console()
except ImportError:
    RICH_AVAILABLE = False
    console = None

class BenchProgress:
    def __init__(self, title):
        self.title = title
        self.progress = None
        self.tasks = {}
        if RICH_AVAILABLE:
            self.progress = Progress(
                SpinnerColumn(),
                TextColumn("[bold blue]{task.description}"),
                BarColumn(bar_width=None),
                "[progress.percentage]{task.percentage:>3.0f}%",
                TimeElapsedColumn(),
                TextColumn("{task.fields[status]}"),
                expand=True
            )

    def __enter__(self):
        if self.progress:
            self.progress.start()
            self.main_task = self.progress.add_task(f"[bold green]{self.title}", total=100, status="")
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if self.progress:
            self.progress.stop()

    def set_total(self, total):
        if self.progress:
            self.progress.update(self.main_task, total=total)

    def start_task(self, tid, name=""):
        if self.progress:
            desc = f"{tid} ({name})" if name else str(tid)
            self.tasks[tid] = self.progress.add_task(desc, total=100, status="[yellow]Running...")
        else:
            n = f" ({name})" if name else ""
            print(f"  [START] {tid}{n}...")

    def update_task(self, tid, success, stats, sym, status_msg=""):
        if self.progress:
            task_id = self.tasks.get(tid)
            if task_id is not None:
                color = "green" if success else "red"
                final_status = f"[{color}]{sym} {status_msg} | turns={stats['turns']} tk={stats['tokens']} t={stats['elapsed_seconds']:.1f}s"
                self.progress.update(task_id, completed=100, status=final_status)
                self.progress.advance(self.main_task, 1)
        else:
            print(f"  [DONE] {sym} {tid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s status={status_msg}")

    def skip_task(self, tid):
        if self.progress:
            self.progress.advance(self.main_task, 1)
        else:
            print(f"  [SKIP] {tid} (already processed)")
"""

code = code.replace("from concurrent.futures import ThreadPoolExecutor", "from concurrent.futures import ThreadPoolExecutor\n" + wrapper_code)

# 2. Patch the functions: run_evalplus, run_swebench, run_terminalbench, run_livecodebench, run_bigcodebench
funcs = ["run_evalplus", "run_swebench", "run_terminalbench", "run_livecodebench", "run_bigcodebench"]

for func in funcs:
    # This is tricky because we need to wrap the ThreadPoolExecutor inside `with BenchProgress(func) as p:`
    # And replace prints.
    
    # Actually, we can just replace the specific prints inside process_task!
    pass

# We will just write a simpler approach: define a global `current_progress` 
# Or pass `prog` to `process_task`? No, since thread local isn't needed if we use global lock, but Progress instances need to be started and stopped.

# Let's replace the whole ThreadPoolExecutor block for each benchmark.
