#!/usr/bin/env python3
"""
Runner unificado que executa TODOS os benchmarks do crom-agente sequencialmente ou em paralelo
e consolida os resultados para a análise de benchmark.

Uso: python3 benchmark/run_all.py [--limit N] [--workers W]
"""
import os
import sys
import json
import time
import tempfile
import subprocess
import re
import threading
from pathlib import Path
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor

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
            print(f"  [{sym}] {tid} | turns={stats['turns']} tk={stats['tokens']} t={stats['elapsed_seconds']:.1f}s | {status_msg}")

    def skip_task(self, tid):
        if self.progress:
            self.progress.advance(self.main_task, 1)
        else:
            print(f"  [SKIP] {tid}")


# Configuração
BASE_DIR = Path(__file__).resolve().parent.parent
BINARY = BASE_DIR / "bin" / "crom-agente"
REPORTS_DIR = Path(__file__).resolve().parent / "reports"
REPORTS_DIR.mkdir(exist_ok=True)

def cleanup_reports_if_needed():
    """Limpa a pasta reports automaticamente se estourar 1GB."""
    total_size = sum(f.stat().st_size for f in REPORTS_DIR.glob('**/*') if f.is_file())
    max_size = 1 * 1024 * 1024 * 1024  # 1GB em bytes
    if total_size > max_size:
        print(f"🧹 A pasta reports ultrapassou 1GB ({total_size / 1024 / 1024:.1f} MB). Limpando arquivos antigos...")
        files = sorted([f for f in REPORTS_DIR.glob('**/*') if f.is_file()], key=lambda x: x.stat().st_mtime)
        bytes_to_free = total_size - (max_size * 0.5)  # Libera até ficar com 500MB
        freed = 0
        for f in files:
            if freed >= bytes_to_free:
                break
            size = f.stat().st_size
            f.unlink(missing_ok=True)
            freed += size

cleanup_reports_if_needed()


# Configura a API key
ENV_FILE = BASE_DIR / "tests" / ".home" / ".crom" / ".env"
if ENV_FILE.exists():
    with open(ENV_FILE) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, val = line.split("=", 1)
                os.environ.setdefault(key.strip(), val.strip())

PROVIDER = "openrouter"
MODEL = "meta-llama/llama-3.1-8b-instruct"
MAX_ITER = 30   # Limite máximo de turnos (evita loops infinitos)
TIMEOUT = 1800  # 30 minutos por tarefa
MAX_TOKENS_PER_TASK = 200_000  # Hard-cap de tokens para evitar desperdício extremo


class ProgressManager:
    def __init__(self, filepath):
        self.filepath = Path(filepath)
        self.lock = threading.Lock()
        self.state = {}
        if self.filepath.exists():
            try:
                with open(self.filepath) as f:
                    self.state = json.load(f)
            except Exception:
                pass

    def get_result(self, bench_name, task_id):
        return self.state.get(bench_name, {}).get(task_id)

    def save_result(self, bench_name, task_id, result):
        with self.lock:
            if bench_name not in self.state:
                self.state[bench_name] = {}
            self.state[bench_name][task_id] = result
            tmp = self.filepath.with_suffix('.tmp')
            with open(tmp, 'w') as f:
                json.dump(self.state, f, indent=2)
            tmp.rename(self.filepath)

    def clear(self):
        with self.lock:
            self.state = {}
            if self.filepath.exists():
                self.filepath.unlink()

progress = ProgressManager(REPORTS_DIR / "benchmark_progress.json")


def build_agent():
    """Compila o binário Go."""
    print("🔨 Compilando crom-agente...")
    res = subprocess.run(
        ["go", "build", "-tags", "headless", "-o", str(BINARY), "./cmd/crom-agente"],
        cwd=str(BASE_DIR), capture_output=True, text=True
    )
    if res.returncode != 0:
        print(f"❌ Erro: {res.stderr}")
        return False
    print("✓ Compilado com sucesso")
    return True


def run_agent(prompt, workspace, max_iter=MAX_ITER, timeout=TIMEOUT, use_docker=False, task_id="unknown"):
    """Executa o crom-agente em um workspace e retorna estatísticas."""
    workspace_path = Path(workspace)
    workspace_path.mkdir(parents=True, exist_ok=True)
    
    # Limpa estado anterior
    crom_dir = workspace_path / ".crom"
    if crom_dir.exists():
        import shutil
        shutil.rmtree(crom_dir, ignore_errors=True)
    
    if use_docker:
        api_key = os.environ.get(f"{PROVIDER.upper()}_API_KEY", "")
        cmd = [
            "docker", "run", "--rm",
            "-v", f"{workspace_path}:/workspace",
            "-v", f"{BINARY}:/crom-agente",
            "-e", f"{PROVIDER.upper()}_API_KEY={api_key}",
            "python:3.10-slim",
            "/crom-agente", "run", prompt,
            "--provider", PROVIDER,
            "--model", MODEL,
            "--workspace", "/workspace",
            "--permission-mode", "total_access",
            "--max-iterations", str(max_iter),
            "--disable-prompt-optimization"
        ]
    else:
        cmd = [
            str(BINARY), "run", prompt,
            "--provider", PROVIDER,
            "--model", MODEL,
            "--workspace", str(workspace_path),
            "--permission-mode", "total_access",
            "--max-iterations", str(max_iter),
            "--disable-prompt-optimization"
        ]
    
    start = time.time()
    try:
        res = subprocess.run(
            cmd, env=os.environ.copy(), cwd=str(workspace_path),
            capture_output=True, text=True, errors="replace", timeout=timeout
        )
        elapsed = time.time() - start
        output = res.stdout + "\n" + res.stderr
        exit_ok = res.returncode == 0
    except subprocess.TimeoutExpired as te:
        elapsed = time.time() - start
        output = f"TIMEOUT after {timeout}s\n{te.stdout or ''}\n{te.stderr or ''}"
        exit_ok = False
    
    # Lê telemetria do .crom_state.json
    state = {"turns": 0, "tokens": 0, "tool_calls": 0, "limit_exceeded": False}
    state_file = workspace_path / ".crom" / ".crom_state.json"
    if state_file.exists():
        try:
            with open(state_file) as f:
                data = json.load(f)
                state["turns"] = data.get("total_turnos", data.get("TotalTurnos", 0))
                state["tokens"] = data.get("tokens_gastos", data.get("TokensGastos", 0))
                state["tool_calls"] = data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0))
                if state["tokens"] > MAX_TOKENS_PER_TASK:
                    state["limit_exceeded"] = True
        except Exception:
            pass
    else:
        # Tenta contar turnos a partir das sessões
        sessions_dir = workspace_path / ".crom" / "sessions"
        if sessions_dir.exists():
            sessions = sorted(list(sessions_dir.glob("session-*")), key=os.path.getmtime)
            if sessions:
                for fname in [".crom_state.json", "session.json"]:
                    sfile = sessions[-1] / fname
                    if sfile.exists():
                        try:
                            with open(sfile) as f:
                                data = json.load(f)
                                state["turns"] = data.get("total_turnos", data.get("TotalTurnos", 0))
                                state["tokens"] = data.get("tokens_gastos", data.get("TokensGastos", 0))
                                state["tool_calls"] = data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0))
                        except Exception:
                            pass
                        break
    
    state["elapsed_seconds"] = elapsed
    state["exit_ok"] = exit_ok
    state["output"] = output
    
    logs_dir = REPORTS_DIR / "logs"
    logs_dir.mkdir(exist_ok=True)
    safe_tid = str(task_id).replace("/", "_")
    log_path = logs_dir / f"{safe_tid}.log"
    with open(log_path, "w") as lf:
        lf.write(output)
        
    return state


def extract_code(output, entry_point):
    """Extrai código Python da saída do agente."""
    # Tenta blocos ```python
    blocks = re.findall(r'```(?:python)?\s*\n(.*?)```', output, re.DOTALL)
    for block in blocks:
        if f"def {entry_point}" in block:
            return block.strip()
    # Tenta definição direta
    pattern = rf'(def {re.escape(entry_point)}\(.*?\n(?:[ \t]+.*\n)*)'
    match = re.search(pattern, output)
    if match:
        return match.group(1).strip()
    return None


# ============================================================
# BENCHMARK 1: EvalPlus (HumanEval)
# ============================================================
def run_evalplus(limit=5, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 1/5: EvalPlus (HumanEval)")
    print("=" * 60)
    
    # Carrega tarefas reais do HumanEval
    tasks = []
    try:
        from datasets import load_dataset
        ds = load_dataset("evalplus/humanevalplus", split="test")
        for i in range(min(limit, len(ds))):
            item = ds[i]
            tasks.append({
                "task_id": item["task_id"],
                "prompt": item["prompt"],
                "test": item["test"],
                "entry_point": item["entry_point"]
            })
        print(f"✓ {len(tasks)} tarefas HumanEval carregadas do Hugging Face")
    except Exception as e:
        print(f"⚠️ Fallback para mocks: {e}")
        tasks = [
            {"task_id": "HumanEval/0", "prompt": 'from typing import List\n\n\ndef has_close_elements(numbers: List[float], threshold: float) -> bool:\n    """ Check if in given list of numbers, are any two numbers closer to each other than\n    given threshold.\n    >>> has_close_elements([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.3)\n    True\n    >>> has_close_elements([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.05)\n    False\n    """\n', "test": "def check(candidate):\n    assert candidate([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.3) == True\n    assert candidate([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.05) == False\n    assert candidate([1.0, 2.0, 5.9, 4.0, 5.0], 0.95) == True\n    assert candidate([1.0, 2.0, 5.9, 4.0, 5.0], 0.8) == False\n    assert candidate([1.0, 2.0, 3.0, 4.0, 5.0, 2.0], 0.1) == True\n    assert candidate([1.1, 2.2, 3.1, 4.1, 5.1], 1.0) == True\n    assert candidate([1.1, 2.2, 3.1, 4.1, 5.1], 0.5) == False\n", "entry_point": "has_close_elements"},
        ][:limit]
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("evalplus", tid)
        if cached:
            prog.skip_task(tid)
            return cached
            
        prog.start_task(tid)
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA: Complete a função Python abaixo e salve em um arquivo 'solucao.py'.\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Crie o arquivo solucao.py usando o terminal: cat > solucao.py << 'PYEOF'\n"
                f"2. Escreva APENAS a função completa (com def e corpo)\n"
                f"3. Verifique com: cat solucao.py\n\n"
                f"```python\n{task['prompt']}```\n\n"
                f"Salve em solucao.py agora."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT, task_id=tid)
            
            # Tenta encontrar a solução
            sol_file = Path(tmpdir) / "solucao.py"
            success = False
            status = "failed_no_file"
            
            if not sol_file.exists():
                # Fallback: qualquer .py
                py_files = [f for f in Path(tmpdir).glob("*.py")]
                if py_files:
                    sol_file = py_files[0]
            
            if not sol_file.exists():
                # Fallback: extrair da saída
                code = extract_code(stats.get("output", ""), task["entry_point"])
                if code:
                    sol_file = Path(tmpdir) / "solucao.py"
                    with open(sol_file, "w") as f:
                        f.write(code)
            
            if sol_file.exists():
                try:
                    with open(sol_file) as f:
                        gen_code = f.read()
                    
                    test_script = (
                        f"{gen_code}\n\n"
                        f"{task['test']}\n\n"
                        f"if __name__ == '__main__':\n"
                        f"    check({task['entry_point']})\n"
                        f"    print('PASS')\n"
                    )
                    
                    test_file = Path(tmpdir) / "run_test.py"
                    with open(test_file, "w") as f:
                        f.write(test_script)
                    
                    r = subprocess.run(
                        [sys.executable, str(test_file)],
                        capture_output=True, text=True, timeout=10
                    )
                    if r.returncode == 0 and "PASS" in r.stdout:
                        success = True
                        status = "success"
                    else:
                        status = "failed_tests"
                        if r.stderr and workers == 1:
                            print(f"    stderr: {r.stderr[:200]}")
                except Exception as e:
                    status = f"error: {e}"
            
            if not stats.get("exit_ok"):
                success = False
                status = "crashed"
            
            if stats.get("limit_exceeded"):
                success = False
                status = "failed_token_limit"
            
            sym = "✅" if success else "❌"
            prog.update_task(tid, success, stats, sym, f"status={status}")
            
            res = {
                "task_id": tid,
                "success": success,
                "turns": stats["turns"],
                "tokens": stats["tokens"],
                "elapsed": stats["elapsed_seconds"],
                "tool_calls": stats["tool_calls"],
                "status": status
            }
            progress.save_result("evalplus", tid, res)
            return res

    with BenchProgress("EvalPlus (HumanEval)") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = []
        for f in futures:
            try:
                results.append(f.result(timeout=TIMEOUT + 60))
            except Exception as e:
                print(f"  [ERROR] Future timed out or failed: {e}")
                results.append({"error": str(e)})
    
    return {"benchmark": "evalplus", "results": results}


# ============================================================
# BENCHMARK 2: SWE-bench Lite (Local, sem Docker)
# ============================================================
def run_swebench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 2/5: SWE-bench Lite (Local)")
    print("=" * 60)
    
    tasks = []
    try:
        from datasets import load_dataset
        ds = load_dataset("princeton-nlp/SWE-bench_Lite", split="test")
        for i in range(min(limit, len(ds))):
            item = ds[i]
            tasks.append({
                "instance_id": item["instance_id"],
                "repo": item["repo"],
                "base_commit": item["base_commit"],
                "problem_statement": item["problem_statement"],
                "patch": item.get("patch", ""),
            })
        print(f"✓ {len(tasks)} tarefas SWE-bench carregadas")
    except Exception as e:
        print(f"⚠️ Usando mock: {e}")
        tasks = [
            {"instance_id": "astropy__astropy-12907", "repo": "astropy/astropy", "base_commit": "abc123", 
             "problem_statement": "Modeling compound model with shared parameters raises error.", "patch": ""},
        ][:limit]
    
    def process_task(task, idx):
        iid = task["instance_id"]
        cached = progress.get_result("swe-bench", iid)
        if cached:
            prog.skip_task(iid)
            return cached
            
        prog.start_task(iid)
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA DE ENGENHARIA DE SOFTWARE:\n\n"
                f"Repositório: {task['repo']}\n"
                f"Commit Base: {task['base_commit']}\n"
                f"Issue ID: {iid}\n\n"
                f"PROBLEMA:\n{task['problem_statement'][:2000]}\n\n"
                f"INSTRUÇÕES CRÍTICAS:\n"
                f"0. Você está em um diretório vazio. PRIMEIRA COISA A FAZER: execute um comando no terminal para clonar o repositório (git clone https://github.com/{task['repo']}.git repo) e fazer o checkout do commit base (cd repo && git checkout {task['base_commit']}).\n"
                f"1. Analise o problema descrito acima lendo os arquivos do repositório clonado\n"
                f"2. Identifique os arquivos relevantes que precisam ser alterados e faça as modificações necessárias neles\n"
                f"3. Após testar suas alterações, gere um arquivo de diff executando 'git diff > ../fix.patch' de dentro do diretório do repositório clonado\n"
                f"4. Escreva uma explicação da sua solução no arquivo '../analise.md'\n"
                f"5. O arquivo fix.patch final DEVE ter o formato unificado de diff gerado pelo git e DEVE estar no diretório raiz do seu ambiente (junto com analise.md)."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT, use_docker=True, task_id=iid)
            
            # Validação honesta: patch precisa existir E conter marcadores de diff reais
            has_patch = (Path(tmpdir) / "fix.patch").exists()
            has_analysis = (Path(tmpdir) / "analise.md").exists()
            
            # Verificar se o patch é um diff válido (não apenas um arquivo vazio)
            valid_patch = False
            if has_patch:
                patch_content = (Path(tmpdir) / "fix.patch").read_text()
                valid_patch = ("---" in patch_content and "+++" in patch_content) or ("diff" in patch_content.lower())
            if has_patch:
                try:
                    patch_content = (Path(tmpdir) / "fix.patch").read_text()
                    pred = {
                        "instance_id": iid,
                        "model_name_or_path": "crom-agente",
                        "model_patch": patch_content
                    }
                    pred_file = REPORTS_DIR / "predictions.jsonl"
                    with open(pred_file, "a") as pf:
                        import json
                        pf.write(json.dumps(pred) + "\n")
                except Exception as e:
                    print(f"Error saving prediction for {iid}: {e}")

            
            success = valid_patch  # Só conta como sucesso se produziu um patch válido
            if valid_patch:
                status = "valid_patch"
            elif has_patch:
                status = "invalid_patch"  # Arquivo existe mas não é um diff real
            elif has_analysis:
                status = "analysis_only"  # Só fez análise, sem patch
            else:
                status = "no_output"
            
            if not stats.get("exit_ok"):
                success = False
                status = "crashed"
            
            if stats.get("limit_exceeded"):
                success = False
                status = "failed_token_limit"
            
            sym = "✅" if success else "❌"
            prog.update_task(iid, success, stats, sym, f"patch={has_patch} analysis={has_analysis}")
            
            res = {
                "task_id": iid,
                "success": success,
                "turns": stats["turns"],
                "tokens": stats["tokens"],
                "elapsed": stats["elapsed_seconds"],
                "tool_calls": stats["tool_calls"],
                "status": status,
                "has_patch": has_patch,
                "has_analysis": has_analysis
            }
            progress.save_result("swe-bench", iid, res)
            # Limpa arquivos criados como root pelo Docker para evitar erro de permissão no context manager
            try:
                subprocess.run([
                    "docker", "run", "--rm",
                    "-v", f"{tmpdir}:/workspace",
                    "alpine", "rm", "-rf", "/workspace/.crom"
                ], capture_output=True)
            except Exception:
                pass
            return res

    with BenchProgress("EvalPlus (HumanEval)") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = []
        for f in futures:
            try:
                results.append(f.result(timeout=TIMEOUT + 60))
            except Exception as e:
                print(f"  [ERROR] Future timed out or failed: {e}")
                results.append({"error": str(e)})
    

    # Run SWE-bench evaluation harness if possible
    pred_file = REPORTS_DIR / "predictions.jsonl"
    if pred_file.exists():
        print("\n🚀 Iniciando avaliação nativa do SWE-bench a partir de predictions.jsonl...")
        import subprocess
        try:
            # Check if swebench is installed
            res = subprocess.run(["python3", "-m", "swebench.harness.run_evaluation", "--help"], capture_output=True)
            if res.returncode == 0:
                print("⏳ Executando swebench harness (isso pode demorar MUITO tempo para buildar as imagens docker)...")
                subprocess.Popen(
                    ["python3", "-m", "swebench.harness.run_evaluation", 
                     "--dataset_name", "princeton-nlp/SWE-bench_Lite", 
                     "--predictions_path", str(pred_file),
                     "--max_workers", str(workers),
                     "--run_id", "crom_agente_run"],
                    cwd=str(BASE_DIR)
                )
            else:
                print("⚠️ Pacote 'swebench' não está instalado no ambiente. Ignorando avaliação nativa.")
                print("   Instale com: pip install swebench docker")
        except Exception as e:
            print(f"⚠️ Erro ao tentar rodar SWE-bench harness: {e}")
            
    return {"benchmark": "swe-bench", "results": results}


# ============================================================
# BENCHMARK 3: Terminal-Tasks (Custom/Interno — NÃO é benchmark público)
# ============================================================
def run_terminalbench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 3/5: Terminal-Tasks (Custom)")
    print("=" * 60)
    
    # Tarefas realistas de terminal (baseadas no formato Terminal-Bench)
    tasks = []
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("terminal-bench", tid)
        if cached:
            prog.skip_task(tid)
            return cached
            
        prog.start_task(tid, task["name"])
        
        with tempfile.TemporaryDirectory() as tmpdir:
            stats = run_agent(task["instruction"], tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT, task_id=tid)
            
            try:
                success = task["validation"](tmpdir)
            except Exception:
                success = False
            
            status = "success" if success else "failed_validation"
            if not stats.get("exit_ok"):
                success = False
                status = "crashed"
            
            if stats.get("limit_exceeded"):
                success = False
                status = "failed_token_limit"
            
            sym = "✅" if success else "❌"
            prog.update_task(tid, success, stats, sym, f"status={status}")
            
            res = {
                "task_id": tid,
                "name": task["name"],
                "success": success,
                "turns": stats["turns"],
                "tokens": stats["tokens"],
                "elapsed": stats["elapsed_seconds"],
                "tool_calls": stats["tool_calls"],
                "status": status
            }
            progress.save_result("terminal-bench", tid, res)
            return res

    with BenchProgress("EvalPlus (HumanEval)") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = []
        for f in futures:
            try:
                results.append(f.result(timeout=TIMEOUT + 60))
            except Exception as e:
                print(f"  [ERROR] Future timed out or failed: {e}")
                results.append({"error": str(e)})
    
    return {"benchmark": "terminal-bench", "results": results}


# ============================================================
# BENCHMARK 4: MBPP Sanitized (Substitui LiveCodeBench fake)
# Dataset real: google-research-datasets/mbpp (257 tasks)
# ============================================================
def run_livecodebench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 4/5: MBPP Sanitized (Código Real)")
    print("=" * 60)
    
    tasks = []
    try:
        from datasets import load_dataset
        ds = load_dataset("google-research-datasets/mbpp", "sanitized", split="test")
        for i in range(min(limit, len(ds))):
            item = ds[i]
            # Extrair nome da função do código de referência
            entry_point = "solution"
            import re as _re
            fn_match = _re.search(r'def (\w+)\(', item.get("code", ""))
            if fn_match:
                entry_point = fn_match.group(1)
            
            # Construir teste executável a partir da test_list
            test_assertions = "\n".join(item.get("test_list", []))
            test_code = f"def check(candidate):\n"
            # Substituir nome da função real por 'candidate' nos asserts
            for assertion in item.get("test_list", []):
                test_code += f"    {assertion.replace(entry_point, 'candidate')}\n"
            
            tasks.append({
                "task_id": f"MBPP/{item['task_id']}",
                "name": entry_point,
                "prompt": item["prompt"],
                "test": test_code,
                "test_raw": test_assertions,
                "entry_point": entry_point,
            })
        print(f"✓ {len(tasks)} tarefas MBPP carregadas do Hugging Face")
    except Exception as e:
        print(f"⚠️ Erro ao carregar MBPP: {e}")
        tasks = []
    
    if not tasks:
        return {"benchmark": "mbpp", "results": []}
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("mbpp", tid)
        if cached:
            prog.skip_task(tid)
            return cached
            
        prog.start_task(tid, task["name"])
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA: Escreva uma função Python e salve em 'solucao.py'.\n\n"
                f"DESCRIÇÃO:\n{task['prompt']}\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Use: cat > solucao.py << 'PYEOF'\n"
                f"2. A função deve se chamar '{task['entry_point']}'\n"
                f"3. Verifique com: cat solucao.py\n\n"
                f"Salve em solucao.py."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT, task_id=tid)
            
            sol_file = Path(tmpdir) / "solucao.py"
            success = False
            status = "failed_no_file"
            
            if not sol_file.exists():
                py_files = [f for f in Path(tmpdir).glob("*.py")]
                if py_files:
                    sol_file = py_files[0]
            
            if not sol_file.exists():
                code = extract_code(stats.get("output", ""), task["entry_point"])
                if code:
                    with open(Path(tmpdir) / "solucao.py", "w") as f:
                        f.write(code)
                    sol_file = Path(tmpdir) / "solucao.py"
            
            if sol_file.exists():
                try:
                    gen_code = sol_file.read_text()
                    # Usar os asserts originais do MBPP (sem wrapper check())
                    test_script = f"{gen_code}\n\n{task['test_raw']}\nprint('PASS')\n"
                    test_file = Path(tmpdir) / "run_test.py"
                    test_file.write_text(test_script)
                    
                    r = subprocess.run([sys.executable, str(test_file)], capture_output=True, text=True, timeout=15)
                    if r.returncode == 0 and "PASS" in r.stdout:
                        success = True
                        status = "success"
                    else:
                        status = "failed_tests"
                except Exception as e:
                    status = f"error: {e}"
            
            if not stats.get("exit_ok"):
                success = False
                status = "crashed"
            
            if stats.get("limit_exceeded"):
                success = False
                status = "failed_token_limit"
            
            sym = "✅" if success else "❌"
            prog.update_task(tid, success, stats, sym, f"status={status}")
            
            res = {
                "task_id": tid,
                "name": task["name"],
                "success": success,
                "turns": stats["turns"],
                "tokens": stats["tokens"],
                "elapsed": stats["elapsed_seconds"],
                "tool_calls": stats["tool_calls"],
                "status": status
            }
            progress.save_result("mbpp", tid, res)
            return res

    with BenchProgress("EvalPlus (HumanEval)") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = []
        for f in futures:
            try:
                results.append(f.result(timeout=TIMEOUT + 60))
            except Exception as e:
                print(f"  [ERROR] Future timed out or failed: {e}")
                results.append({"error": str(e)})
    
    return {"benchmark": "mbpp", "results": results}


# ============================================================
# BENCHMARK 5: BigCodeBench (Dataset Real)
# Dataset real: bigcode/bigcodebench (1140 tasks)
# ============================================================
def run_bigcodebench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 5/5: BigCodeBench (Dataset Real)")
    print("=" * 60)
    
    tasks = []
    try:
        from datasets import load_dataset
        ds = load_dataset("bigcode/bigcodebench", split="v0.1.2")
        for i in range(min(limit, len(ds))):
            item = ds[i]
            tasks.append({
                "task_id": item["task_id"],
                "name": item["entry_point"],
                "prompt": item["instruct_prompt"],
                "code_prompt": item.get("code_prompt", ""),
                "test": item["test"],
                "entry_point": item["entry_point"],
            })
        print(f"✓ {len(tasks)} tarefas BigCodeBench carregadas do Hugging Face")
    except Exception as e:
        print(f"⚠️ Erro ao carregar BigCodeBench: {e}")
        tasks = []
    
    if not tasks:
        return {"benchmark": "bigcodebench", "results": []}
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("bigcodebench", tid)
        if cached:
            prog.skip_task(tid)
            return cached
            
        prog.start_task(tid, task["name"])
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA: Escreva uma função Python e salve em 'solucao.py'.\n\n"
                f"DESCRIÇÃO:\n{task['prompt'][:2000]}\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Use: cat > solucao.py << 'PYEOF'\n"
                f"2. Inclua todos os imports necessários\n"
                f"3. A função deve se chamar '{task['entry_point']}'\n"
                f"4. Verifique com: cat solucao.py\n\n"
                f"Salve em solucao.py."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT, task_id=tid)
            
            sol_file = Path(tmpdir) / "solucao.py"
            success = False
            status = "failed_no_file"
            
            if not sol_file.exists():
                py_files = [f for f in Path(tmpdir).glob("*.py")]
                if py_files:
                    sol_file = py_files[0]
            
            if not sol_file.exists():
                code = extract_code(stats.get("output", ""), task["entry_point"])
                if code:
                    with open(Path(tmpdir) / "solucao.py", "w") as f:
                        f.write(code)
                    sol_file = Path(tmpdir) / "solucao.py"
            
            if sol_file.exists():
                try:
                    gen_code = sol_file.read_text()
                    test_script = f"{gen_code}\n\n{task['test']}\n\nif __name__ == '__main__':\n    import unittest\n    unittest.main()\n"
                    test_file = Path(tmpdir) / "run_test.py"
                    test_file.write_text(test_script)
                    
                    r = subprocess.run([sys.executable, str(test_file)], capture_output=True, text=True, timeout=30)
                    if r.returncode == 0:
                        success = True
                        status = "success"
                    else:
                        status = "failed_tests"
                except Exception as e:
                    status = f"error: {e}"
            
            if not stats.get("exit_ok"):
                success = False
                status = "crashed"
            
            if stats.get("limit_exceeded"):
                success = False
                status = "failed_token_limit"
            
            sym = "✅" if success else "❌"
            prog.update_task(tid, success, stats, sym, f"status={status}")
            
            res = {
                "task_id": tid,
                "name": task["name"],
                "success": success,
                "turns": stats["turns"],
                "tokens": stats["tokens"],
                "elapsed": stats["elapsed_seconds"],
                "tool_calls": stats["tool_calls"],
                "status": status
            }
            progress.save_result("bigcodebench", tid, res)
            return res

    with BenchProgress("EvalPlus (HumanEval)") as prog:
        prog.set_total(len(tasks))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = []
        for f in futures:
            try:
                results.append(f.result(timeout=TIMEOUT + 60))
            except Exception as e:
                print(f"  [ERROR] Future timed out or failed: {e}")
                results.append({"error": str(e)})
    
    return {"benchmark": "bigcodebench", "results": results}


# CONSOLIDAÇÃO
# ============================================================
def calculate_costs(tokens, price_input=0.075, price_output=0.3):
    """Estima custo baseado no split 70/30 input/output."""
    input_tokens = tokens * 0.7
    output_tokens = tokens * 0.3
    return (input_tokens / 1_000_000 * price_input) + (output_tokens / 1_000_000 * price_output)


def consolidate(all_results, elapsed_total):
    """Gera relatório consolidado."""
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    
    report = {
        "timestamp": datetime.now().isoformat(),
        "model": MODEL,
        "provider": PROVIDER,
        "max_iterations": MAX_ITER,
        "total_elapsed_seconds": elapsed_total,
        "benchmarks": {}
    }
    
    print("\n" + "=" * 60)
    print("📊 RESULTADOS CONSOLIDADOS")
    print("=" * 60)
    print(f"Modelo: {MODEL}")
    print(f"Provedor: {PROVIDER}")
    print(f"Max Iterações: {MAX_ITER}")
    print(f"Tempo Total: {elapsed_total:.0f}s ({elapsed_total/60:.1f}min)")
    print("-" * 60)
    
    for bench_data in all_results:
        name = bench_data["benchmark"]
        results = bench_data["results"]
        
        total = len(results)
        valid_results = [r for r in results if isinstance(r, dict) and "error" not in r]
        passed = sum(1 for r in valid_results if r.get("success", False))
        rate = (passed / total * 100) if total > 0 else 0
        avg_turns = sum(r.get("turns", 0) for r in valid_results) / len(valid_results) if valid_results else 0
        avg_tokens = sum(r.get("tokens", 0) for r in valid_results) / len(valid_results) if valid_results else 0
        avg_elapsed = sum(r.get("elapsed", 0.0) for r in valid_results) / len(valid_results) if valid_results else 0
        total_tokens = sum(r.get("tokens", 0) for r in valid_results)
        total_cost = calculate_costs(total_tokens)
        
        report["benchmarks"][name] = {
            "total_tasks": total,
            "passed": passed,
            "success_rate": round(rate, 1),
            "avg_turns": round(avg_turns, 1),
            "avg_tokens": round(avg_tokens, 0),
            "avg_elapsed": round(avg_elapsed, 1),
            "total_tokens": total_tokens,
            "total_cost_usd": round(total_cost, 6),
            "results": results
        }
        
        print(f"\n{'📋 ' + name.upper():}")
        print(f"  Tarefas: {passed}/{total} ({rate:.1f}%)")
        print(f"  Turnos médios: {avg_turns:.1f}")
        print(f"  Tokens médios: {avg_tokens:.0f}")
        print(f"  Tempo médio: {avg_elapsed:.1f}s")
        print(f"  Custo total: ${total_cost:.6f}")
    
    # Salva JSON
    json_file = REPORTS_DIR / f"full_benchmark_{timestamp}.json"
    with open(json_file, "w") as f:
        json.dump(report, f, indent=2, default=str)
    print(f"\n✓ JSON salvo: {json_file}")
    
    return report


def main():
    import argparse
    parser = argparse.ArgumentParser(description="Runner de todos os benchmarks")
    parser.add_argument("--limit", type=int, default=5, help="Limite de tarefas por benchmark")
    parser.add_argument("--workers", type=int, default=3, help="Número de threads/workers para execução paralela")
    parser.add_argument("--clear-progress", action="store_true", help="Limpa o progresso salvo antes de iniciar")
    parser.add_argument("--api-key", type=str, default="", help="API Key dinâmica (sobrepõe o .env)")
    parser.add_argument("--temp", type=float, default=0.0, help="Temperatura do LLM")
    parser.add_argument("--provider", type=str, default="", help="Provedor de LLM")
    parser.add_argument("--model", type=str, default="", help="Modelo de LLM")
    args = parser.parse_args()
    
    global PROVIDER, MODEL
    if args.provider:
        PROVIDER = args.provider
    if args.model:
        MODEL = args.model
    
    if args.api_key:
        os.environ[f"{PROVIDER.upper()}_API_KEY"] = args.api_key
        print("🔑 Usando API Key passada via flag.")
    if args.temp > 0.0:
        os.environ["CROM_TEMPERATURE"] = str(args.temp)
        print(f"🌡️ Configurando Temperature para {args.temp}")

    if args.clear_progress:
        progress.clear()
        print("🗑️ Progresso anterior limpo.")
        
    if not build_agent():
        sys.exit(1)
    
    start_total = time.time()
    all_results = []
    
    # Roda todos os benchmarks
    all_results.append(run_evalplus(limit=args.limit, workers=args.workers))
    all_results.append(run_swebench(limit=args.limit, workers=args.workers))
    all_results.append(run_terminalbench(limit=args.limit, workers=args.workers))
    all_results.append(run_livecodebench(limit=args.limit, workers=args.workers))
    all_results.append(run_bigcodebench(limit=args.limit, workers=args.workers))
    
    elapsed_total = time.time() - start_total
    consolidate(all_results, elapsed_total)
    
    print("\n✅ Todos os benchmarks concluídos!")


if __name__ == "__main__":
    main()
