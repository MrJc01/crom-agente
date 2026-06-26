import re

with open("benchmark/run_all.py", "r") as f:
    code = f.read()

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

if "BenchProgress:" not in code:
    code = code.replace("from concurrent.futures import ThreadPoolExecutor", "from concurrent.futures import ThreadPoolExecutor\n" + wrapper_code)

benchmarks = [
    ("evalplus", "EvalPlus (HumanEval)"),
    ("swebench", "SWE-bench Lite (Local)"),
    ("terminalbench", "Terminal-Tasks (Custom)"),
    ("livecodebench", "MBPP Sanitized (Código Real)"),
    ("bigcodebench", "BigCodeBench (Dataset Real)")
]

for b_id, b_title in benchmarks:
    # 1. Substitute the inner prints
    if b_id == "evalplus":
        code = re.sub(r'print\(f"  \[SKIP\] \{tid\} \(already processed\)"\)', r'prog.skip_task(tid)', code)
        code = re.sub(r'print\(f"  \[START\] \{tid\}\.\.\."\)', r'prog.start_task(tid)', code)
    elif b_id == "swebench":
        code = re.sub(r'print\(f"  \[SKIP\] \{iid\} \(already processed\)"\)', r'prog.skip_task(iid)', code)
        code = re.sub(r'print\(f"  \[START\] \{iid\}\.\.\."\)', r'prog.start_task(iid)', code)
    else:
        code = re.sub(r'print\(f"  \[SKIP\] \{tid\} \(already processed\)"\)', r'prog.skip_task(tid)', code)
        code = re.sub(r'print\(f"  \[START\] \{tid\} \(\{task\[\'name\'\]\}\)\.\.\."\)', r'prog.start_task(tid, task["name"])', code)

    # Done print
    code = re.sub(
        r'print\(f"  \[DONE\] \{sym\} \{(tid|iid)\} \| turns=\{stats\[\'turns\'\]\} tokens=\{stats\[\'tokens\'\]\} time=\{stats\[\'elapsed_seconds\'\]:\.1f\}s (.*?)"\)',
        r'prog.update_task(\1, success, stats, sym, f"\2")',
        code
    )

    # Wrap the executor
    old_exec = """    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]"""
    
    new_exec = f"""    with BenchProgress("{b_title}") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]"""
            
    code = code.replace(old_exec, new_exec)

with open("benchmark/run_all.py", "w") as f:
    f.write(code)

