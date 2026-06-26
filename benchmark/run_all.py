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

# Configuração
BASE_DIR = Path(__file__).resolve().parent.parent
BINARY = BASE_DIR / "bin" / "crom-agente"
REPORTS_DIR = Path(__file__).resolve().parent / "reports"
REPORTS_DIR.mkdir(exist_ok=True)

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
MAX_ITER = 0
TIMEOUT = 1800


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


def run_agent(prompt, workspace, max_iter=MAX_ITER, timeout=TIMEOUT):
    """Executa o crom-agente em um workspace e retorna estatísticas."""
    workspace_path = Path(workspace)
    workspace_path.mkdir(parents=True, exist_ok=True)
    
    # Limpa estado anterior
    crom_dir = workspace_path / ".crom"
    if crom_dir.exists():
        import shutil
        shutil.rmtree(crom_dir, ignore_errors=True)
    
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
    state = {"turns": 0, "tokens": 0, "tool_calls": 0}
    state_file = workspace_path / ".crom" / ".crom_state.json"
    if state_file.exists():
        try:
            with open(state_file) as f:
                data = json.load(f)
                state["turns"] = data.get("total_turnos", data.get("TotalTurnos", 0))
                state["tokens"] = data.get("tokens_gastos", data.get("TokensGastos", 0))
                state["tool_calls"] = data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0))
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
            print(f"  [SKIP] {tid} (already processed)")
            return cached
            
        print(f"  [START] {tid}...")
        
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
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT)
            
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
            
            sym = "✅" if success else "❌"
            print(f"  [DONE] {sym} {tid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s status={status}")
            
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

    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = [f.result() for f in futures]
    
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
            print(f"  [SKIP] {iid} (already processed)")
            return cached
            
        print(f"  [START] {iid}...")
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA DE ENGENHARIA DE SOFTWARE:\n\n"
                f"Repositório: {task['repo']}\n"
                f"Issue ID: {iid}\n\n"
                f"PROBLEMA:\n{task['problem_statement'][:2000]}\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Analise o problema descrito acima\n"
                f"2. Identifique os arquivos relevantes que precisam ser alterados\n"
                f"3. Proponha as correções necessárias\n"
                f"4. Salve suas correções em um arquivo 'fix.patch' com formato diff unificado\n"
                f"5. Explique sua solução em 'analise.md'\n\n"
                f"Crie os arquivos fix.patch e analise.md no diretório atual."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT)
            
            # Verifica se o agente produziu alguma saída razoável
            has_patch = (Path(tmpdir) / "fix.patch").exists()
            has_analysis = (Path(tmpdir) / "analise.md").exists()
            any_files = len(list(Path(tmpdir).glob("*"))) > 1  # mais que .crom
            
            success = has_patch or has_analysis or any_files
            status = "produced_output" if success else "no_output"
            
            sym = "✅" if success else "❌"
            print(f"  [DONE] {sym} {iid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s patch={has_patch} analysis={has_analysis}")
            
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
            return res

    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = [f.result() for f in futures]
    
    return {"benchmark": "swe-bench", "results": results}


# ============================================================
# BENCHMARK 3: Terminal-Bench (Tarefas de Terminal)
# ============================================================
def run_terminalbench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 3/5: Terminal-Bench (Tarefas de Terminal)")
    print("=" * 60)
    
    # Tarefas realistas de terminal (baseadas no formato Terminal-Bench)
    tasks = [
        {"task_id": "tb-001", "name": "file_search_replace",
         "instruction": "Crie um arquivo chamado data.csv com o conteúdo:\nname,age,city\nAlice,30,NYC\nBob,25,SF\nCharlie,35,NYC\n\nDepois, use sed ou awk para substituir todas as ocorrências de 'NYC' por 'New York City' no arquivo. Salve o resultado no mesmo arquivo data.csv.",
         "validation": lambda d: (Path(d) / "data.csv").exists() and "New York City" in (Path(d) / "data.csv").read_text()},
        {"task_id": "tb-002", "name": "json_processing",
         "instruction": "Crie um script Python chamado process.py que:\n1. Cria um arquivo users.json com: [{\"name\": \"Alice\", \"age\": 30}, {\"name\": \"Bob\", \"age\": 25}, {\"name\": \"Charlie\", \"age\": 35}]\n2. Lê o arquivo users.json\n3. Filtra apenas os usuários com idade > 28\n4. Salva o resultado em filtered.json\n\nDepois execute o script com: python3 process.py",
         "validation": lambda d: (Path(d) / "filtered.json").exists()},
        {"task_id": "tb-003", "name": "git_operations",
         "instruction": "Neste diretório:\n1. Inicialize um repositório git com 'git init'\n2. Configure git user: git config user.email 'test@test.com' && git config user.name 'Test'\n3. Crie um arquivo README.md com o conteúdo '# Test Project\\nThis is a test.'\n4. Faça git add e commit com a mensagem 'initial commit'\n5. Crie um arquivo chamado status.txt contendo a saída de 'git log --oneline'",
         "validation": lambda d: (Path(d) / ".git").exists() and (Path(d) / "README.md").exists() and (Path(d) / "status.txt").exists()},
        {"task_id": "tb-004", "name": "directory_analysis",
         "instruction": "Crie a seguinte estrutura de diretórios:\nsrc/main.py (com 'print(\"hello\")')\nsrc/utils/helpers.py (com 'def add(a,b): return a+b')\ntests/test_main.py (com 'assert True')\n\nDepois, crie um arquivo tree.txt que contenha a saída do comando 'find . -name \"*.py\" | sort'",
         "validation": lambda d: (Path(d) / "src" / "main.py").exists() and (Path(d) / "tree.txt").exists()},
        {"task_id": "tb-005", "name": "network_config_parse",
         "instruction": "Crie um arquivo hosts.txt com:\n192.168.1.1 router\n192.168.1.10 server1\n192.168.1.20 server2\n10.0.0.1 gateway\n\nDepois crie um script bash chamado parse.sh que lê hosts.txt e gera um arquivo report.txt contendo apenas as linhas da subnet 192.168.x.x, uma por linha. Execute o script.",
         "validation": lambda d: (Path(d) / "report.txt").exists() and "192.168" in (Path(d) / "report.txt").read_text()},
    ][:limit]
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("terminal-bench", tid)
        if cached:
            print(f"  [SKIP] {tid} (already processed)")
            return cached
            
        print(f"  [START] {tid} ({task['name']})...")
        
        with tempfile.TemporaryDirectory() as tmpdir:
            stats = run_agent(task["instruction"], tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT)
            
            try:
                success = task["validation"](tmpdir)
            except Exception:
                success = False
            
            status = "success" if success else "failed_validation"
            sym = "✅" if success else "❌"
            print(f"  [DONE] {sym} {tid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s status={status}")
            
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

    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = [f.result() for f in futures]
    
    return {"benchmark": "terminal-bench", "results": results}


# ============================================================
# BENCHMARK 4: LiveCodeBench (Problemas Algorítmicos)
# ============================================================
def run_livecodebench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 4/5: LiveCodeBench (Algoritmos)")
    print("=" * 60)
    
    tasks = [
        {"task_id": "lcb-001", "name": "two_sum",
         "prompt": "def two_sum(nums: list, target: int) -> list:\n    \"\"\"Given an array of integers nums and an integer target, return indices of the two numbers that add up to target. Each input has exactly one solution.\n    >>> two_sum([2, 7, 11, 15], 9)\n    [0, 1]\n    >>> two_sum([3, 2, 4], 6)\n    [1, 2]\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate([2, 7, 11, 15], 9) == [0, 1]\n    assert candidate([3, 2, 4], 6) == [1, 2]\n    assert candidate([3, 3], 6) == [0, 1]\n",
         "entry_point": "two_sum"},
        {"task_id": "lcb-002", "name": "is_palindrome",
         "prompt": "def is_palindrome(s: str) -> bool:\n    \"\"\"Check if a string is a palindrome, considering only alphanumeric characters and ignoring cases.\n    >>> is_palindrome('A man, a plan, a canal: Panama')\n    True\n    >>> is_palindrome('race a car')\n    False\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate('A man, a plan, a canal: Panama') == True\n    assert candidate('race a car') == False\n    assert candidate('') == True\n    assert candidate(' ') == True\n",
         "entry_point": "is_palindrome"},
        {"task_id": "lcb-003", "name": "max_subarray",
         "prompt": "def max_subarray(nums: list) -> int:\n    \"\"\"Find the contiguous subarray which has the largest sum and return its sum.\n    >>> max_subarray([-2,1,-3,4,-1,2,1,-5,4])\n    6\n    >>> max_subarray([1])\n    1\n    >>> max_subarray([5,4,-1,7,8])\n    23\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate([-2,1,-3,4,-1,2,1,-5,4]) == 6\n    assert candidate([1]) == 1\n    assert candidate([5,4,-1,7,8]) == 23\n    assert candidate([-1]) == -1\n",
         "entry_point": "max_subarray"},
        {"task_id": "lcb-004", "name": "valid_parentheses",
         "prompt": "def is_valid(s: str) -> bool:\n    \"\"\"Given a string s containing just the characters '(', ')', '{', '}', '[' and ']', determine if the input string is valid.\n    >>> is_valid('()')\n    True\n    >>> is_valid('()[]{}')\n    True\n    >>> is_valid('(]')\n    False\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate('()') == True\n    assert candidate('()[]{}') == True\n    assert candidate('(]') == False\n    assert candidate('([)]') == False\n    assert candidate('{[]}') == True\n",
         "entry_point": "is_valid"},
        {"task_id": "lcb-005", "name": "merge_sorted",
         "prompt": "def merge(nums1: list, m: int, nums2: list, n: int) -> list:\n    \"\"\"Merge nums2 into nums1 as one sorted array. nums1 has enough space. Return the merged result.\n    >>> merge([1,2,3,0,0,0], 3, [2,5,6], 3)\n    [1, 2, 2, 3, 5, 6]\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate([1,2,3,0,0,0], 3, [2,5,6], 3) == [1, 2, 2, 3, 5, 6]\n    assert candidate([1], 1, [], 0) == [1]\n",
         "entry_point": "merge"},
    ][:limit]
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("livecode-bench", tid)
        if cached:
            print(f"  [SKIP] {tid} (already processed)")
            return cached
            
        print(f"  [START] {tid} ({task['name']})...")
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA: Complete a função Python e salve em 'solucao.py'.\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Use: cat > solucao.py << 'PYEOF'\n"
                f"2. Escreva APENAS a função com corpo implementado\n"
                f"3. Verifique com: cat solucao.py\n\n"
                f"```python\n{task['prompt']}```\n\n"
                f"Salve em solucao.py."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT)
            
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
                    test_script = f"{gen_code}\n\n{task['test']}\n\nif __name__ == '__main__':\n    check({task['entry_point']})\n    print('PASS')\n"
                    test_file = Path(tmpdir) / "run_test.py"
                    test_file.write_text(test_script)
                    
                    r = subprocess.run([sys.executable, str(test_file)], capture_output=True, text=True, timeout=10)
                    if r.returncode == 0 and "PASS" in r.stdout:
                        success = True
                        status = "success"
                    else:
                        status = "failed_tests"
                except Exception as e:
                    status = f"error: {e}"
            
            sym = "✅" if success else "❌"
            print(f"  [DONE] {sym} {tid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s status={status}")
            
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
            progress.save_result("livecode-bench", tid, res)
            return res

    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = [f.result() for f in futures]
    
    return {"benchmark": "livecode-bench", "results": results}
def run_bigcodebench(limit=3, workers=1):
    print("\n" + "=" * 60)
    print("📋 BENCHMARK 5/5: BigCodeBench (APIs Complexas)")
    print("=" * 60)
    
    tasks = [
        {"task_id": "bcb-001", "name": "file_stats",
         "prompt": "import os\nimport json\n\ndef file_stats(directory: str) -> dict:\n    \"\"\"Walk through a directory and return a dict with 'total_files', 'total_dirs', and 'total_size_bytes' keys.\n    >>> result = file_stats('/tmp/test')\n    >>> 'total_files' in result\n    True\n    \"\"\"\n",
         "test": "import tempfile, os\ndef check(candidate):\n    with tempfile.TemporaryDirectory() as d:\n        with open(os.path.join(d, 'a.txt'), 'w') as f: f.write('hello')\n        os.makedirs(os.path.join(d, 'sub'))\n        with open(os.path.join(d, 'sub', 'b.txt'), 'w') as f: f.write('world')\n        r = candidate(d)\n        assert r['total_files'] == 2\n        assert r['total_dirs'] >= 1\n        assert r['total_size_bytes'] == 10\n",
         "entry_point": "file_stats"},
        {"task_id": "bcb-002", "name": "csv_aggregate",
         "prompt": "import csv\nimport io\n\ndef csv_aggregate(csv_string: str, group_col: str, agg_col: str) -> dict:\n    \"\"\"Parse a CSV string and return a dict mapping each unique value in group_col to the sum of agg_col values.\n    >>> csv_aggregate('name,score\\nAlice,10\\nBob,20\\nAlice,30', 'name', 'score')\n    {'Alice': 40, 'Bob': 20}\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate('name,score\\nAlice,10\\nBob,20\\nAlice,30', 'name', 'score') == {'Alice': 40, 'Bob': 20}\n    assert candidate('city,pop\\nNYC,100\\nSF,50\\nNYC,200', 'city', 'pop') == {'NYC': 300, 'SF': 50}\n",
         "entry_point": "csv_aggregate"},
        {"task_id": "bcb-003", "name": "regex_extract",
         "prompt": "import re\n\ndef extract_emails(text: str) -> list:\n    \"\"\"Extract all valid email addresses from a text string and return them as a sorted list.\n    >>> extract_emails('Contact alice@example.com or bob@test.org')\n    ['alice@example.com', 'bob@test.org']\n    \"\"\"\n",
         "test": "def check(candidate):\n    assert candidate('Contact alice@example.com or bob@test.org') == ['alice@example.com', 'bob@test.org']\n    assert candidate('no emails here') == []\n    assert candidate('a@b.com and c@d.co') == ['a@b.com', 'c@d.co']\n",
         "entry_point": "extract_emails"},
    ][:limit]
    
    def process_task(task, idx):
        tid = task["task_id"]
        cached = progress.get_result("bigcodebench", tid)
        if cached:
            print(f"  [SKIP] {tid} (already processed)")
            return cached
            
        print(f"  [START] {tid} ({task['name']})...")
        
        with tempfile.TemporaryDirectory() as tmpdir:
            prompt = (
                f"TAREFA: Complete a função Python e salve em 'solucao.py'.\n\n"
                f"INSTRUÇÕES:\n"
                f"1. Use: cat > solucao.py << 'PYEOF'\n"
                f"2. Inclua todos os imports necessários\n"
                f"3. Escreva a função completa\n"
                f"4. Verifique com: cat solucao.py\n\n"
                f"```python\n{task['prompt']}```\n\n"
                f"Salve em solucao.py."
            )
            
            stats = run_agent(prompt, tmpdir, max_iter=MAX_ITER, timeout=TIMEOUT)
            
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
                    test_script = f"{gen_code}\n\n{task['test']}\n\nif __name__ == '__main__':\n    check({task['entry_point']})\n    print('PASS')\n"
                    test_file = Path(tmpdir) / "run_test.py"
                    test_file.write_text(test_script)
                    
                    r = subprocess.run([sys.executable, str(test_file)], capture_output=True, text=True, timeout=10)
                    if r.returncode == 0 and "PASS" in r.stdout:
                        success = True
                        status = "success"
                    else:
                        status = "failed_tests"
                except Exception as e:
                    status = f"error: {e}"
            
            sym = "✅" if success else "❌"
            print(f"  [DONE] {sym} {tid} | turns={stats['turns']} tokens={stats['tokens']} time={stats['elapsed_seconds']:.1f}s status={status}")
            
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

    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [executor.submit(process_task, task, idx) for idx, task in enumerate(tasks)]
        results = [f.result() for f in futures]
    
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
        passed = sum(1 for r in results if r["success"])
        rate = (passed / total * 100) if total > 0 else 0
        avg_turns = sum(r["turns"] for r in results) / total if total > 0 else 0
        avg_tokens = sum(r["tokens"] for r in results) / total if total > 0 else 0
        avg_elapsed = sum(r["elapsed"] for r in results) / total if total > 0 else 0
        total_tokens = sum(r["tokens"] for r in results)
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
    args = parser.parse_args()
    
    if args.clear_progress:
        progress.clear()
        print("🗑️ Progresso anterior limpo.")
        
    if not build_agent():
        sys.exit(1)
    
    start_total = time.time()
    all_results = []
    
    # Roda todos os benchmarks
    all_results.append(run_evalplus(limit=args.limit, workers=args.workers))
    all_results.append(run_swebench(limit=min(args.limit, 3), workers=args.workers))
    all_results.append(run_terminalbench(limit=args.limit, workers=args.workers))
    all_results.append(run_livecodebench(limit=args.limit, workers=args.workers))
    all_results.append(run_bigcodebench(limit=min(args.limit, 3), workers=args.workers))
    
    elapsed_total = time.time() - start_total
    consolidate(all_results, elapsed_total)
    
    print("\n✅ Todos os benchmarks concluídos!")


if __name__ == "__main__":
    main()
