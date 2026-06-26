import os
import sys
import json
import tempfile
import subprocess
import time
from pathlib import Path
from ..shared.agent_runner import run_agent_task

class EvalPlusAdapter:
    def __init__(self, provider, model, config_path=None):
        self.provider = provider
        self.model = model

    def load_instances(self, benchmark_type="evalplus", limit=5, mock=False):
        """
        Carrega instâncias do HumanEval/EvalPlus ou BigCodeBench do Hugging Face.
        Se offline/falhar, gera mocks locais baseados nos clássicos do HumanEval.
        """
        try:
            if mock:
                raise Exception("Mock mode bypass HF download")
            from datasets import load_dataset
            if benchmark_type == "bigcodebench":
                print("📥 Baixando BigCodeBench do Hugging Face...")
                ds = load_dataset("bigcode-bench/BigCodeBench", split="test")
                instances = []
                for i in range(min(limit, len(ds))):
                    item = ds[i]
                    instances.append({
                        "task_id": item["task_id"],
                        "prompt": item["prompt"],
                        "test": item["test"],
                        "entry_point": item["entry_point"]
                    })
                return instances
            else:
                print("📥 Baixando EvalPlus (HumanEval) do Hugging Face...")
                ds = load_dataset("evalplus/humanevalplus", split="test")
                instances = []
                for i in range(min(limit, len(ds))):
                    item = ds[i]
                    instances.append({
                        "task_id": item["task_id"],
                        "prompt": item["prompt"],
                        "test": item["test"],
                        "entry_point": item["entry_point"]
                    })
                return instances
        except Exception as e:
            print(f"⚠️ Falha ao carregar dataset ({e}). Usando HumanEval mocks locais.")
            return [
                {
                    "task_id": "HumanEval/0",
                    "prompt": "def has_close_elements(numbers: list, threshold: float) -> bool:\n    \"\"\" Check if in given list of numbers, any two numbers are closer to each other than threshold. \"\"\"\n",
                    "test": "def check(has_close_elements):\n    assert has_close_elements([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.3) == True\n    assert has_close_elements([1.0, 2.0, 3.9, 4.0, 5.0, 2.2], 0.05) == False\n",
                    "entry_point": "has_close_elements"
                },
                {
                    "task_id": "HumanEval/1",
                    "prompt": "def separate_paren_groups(paren_string: str) -> list:\n    \"\"\" Input to this function is a string containing multiple groups of nested parentheses. Separate them into individual groups. \"\"\"\n",
                    "test": "def check(separate_paren_groups):\n    assert separate_paren_groups('(a)(b)') == ['(a)', '(b)']\n",
                    "entry_point": "separate_paren_groups"
                }
            ][:limit]

    def run_instance(self, instance, max_iterations=20, timeout=90):
        """
        Executa o agente em uma instância de completude de função.
        """
        task_id = instance["task_id"]
        prompt_code = instance["prompt"]
        test_code = instance["test"]
        entry_point = instance["entry_point"]
        
        print(f"\n🚀 [EvalPlus] Iniciando tarefa: {task_id}")
        
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            
            prompt = (
                f"TAREFA: Complete a função Python abaixo e salve o resultado "
                f"em um arquivo chamado 'solucao.py' neste diretório de trabalho.\n\n"
                f"INSTRUÇÕES OBRIGATÓRIAS:\n"
                f"1. Use o comando 'cat > solucao.py << 'EOF'' ou a ferramenta write_file "
                f"para criar o arquivo solucao.py\n"
                f"2. O arquivo DEVE conter APENAS a função completa (com def e corpo implementado)\n"
                f"3. NÃO inclua imports desnecessários, apenas a função\n"
                f"4. Após criar o arquivo, confirme com 'cat solucao.py'\n\n"
                f"CÓDIGO INICIAL (assinatura e docstring):\n"
                f"```python\n{prompt_code}```\n\n"
                f"Complete o corpo da função e salve em solucao.py."
            )
            
            # Executa o agente no diretório temporário local
            stats = run_agent_task(
                prompt,
                str(temp_path),
                self.provider,
                self.model,
                max_iterations=max_iterations,
                timeout=timeout
            )
            
            # Tenta encontrar o arquivo de solução (fallback para qualquer .py)
            sol_file = temp_path / "solucao.py"
            if not sol_file.exists():
                # Busca qualquer .py criado pelo agente
                py_files = [f for f in temp_path.glob("*.py") if f.name != "run_test.py"]
                if py_files:
                    sol_file = py_files[0]
                    print(f"   ⚠️ solucao.py não encontrado, usando {sol_file.name}")
                else:
                    # Última tentativa: extrair código Python da saída do agente
                    output = stats.get("output", "")
                    code = self._extract_python_from_output(output, entry_point)
                    if code:
                        sol_file = temp_path / "solucao.py"
                        with open(sol_file, "w") as f:
                            f.write(code)
                        print(f"   ⚠️ Extraído código da saída do agente")
                    else:
                        stats["success"] = False
                        stats["status"] = "failed_no_file"
                        return stats
                
            # Executa o test suite contido no dataset (campo 'test')
            # Concatenamos o arquivo gerado com o teste e avaliamos
            success = True
            try:
                with open(sol_file) as f:
                    generated_code = f.read()
                    
                # Script de execução de teste integrado
                test_script = (
                    f"{generated_code}\n\n"
                    f"{test_code}\n\n"
                    f"if __name__ == '__main__':\n"
                    f"    check({entry_point})\n"
                    f"    print('SUCCESS_TEST')\n"
                )
                
                with open(temp_path / "run_test.py", "w") as f:
                    f.write(test_script)
                    
                res = subprocess.run(
                    [sys.executable, str(temp_path / "run_test.py")],
                    capture_output=True,
                    text=True,
                    timeout=5
                )
                
                if res.returncode != 0 or "SUCCESS_TEST" not in res.stdout:
                    success = False
            except Exception as e:
                print(f"Erro ao testar solução do EvalPlus: {e}")
                success = False
                
            stats["success"] = success
            stats["status"] = "success" if success else "failed_tests"
            return stats

    def _extract_python_from_output(self, output, entry_point):
        """
        Tenta extrair código Python da saída do agente quando ele não criou o arquivo.
        Procura por blocos de código ou pela definição da função.
        """
        import re
        
        # Tenta extrair de blocos ```python ... ```
        code_blocks = re.findall(r'```(?:python)?\s*\n(.*?)```', output, re.DOTALL)
        for block in code_blocks:
            if f"def {entry_point}" in block:
                return block.strip()
        
        # Tenta extrair diretamente a definição da função
        pattern = rf'(def {re.escape(entry_point)}\(.*?\n(?:[ \t]+.*\n)*)'
        match = re.search(pattern, output)
        if match:
            return match.group(1).strip()
        
        return None
