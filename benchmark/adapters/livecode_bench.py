import os
import sys
import json
import tempfile
import subprocess
import time
from pathlib import Path
from ..shared.agent_runner import run_agent_task

class LiveCodeBenchAdapter:
    def __init__(self, provider, model, config_path=None):
        self.provider = provider
        self.model = model

    def load_instances(self, limit=5, mock=False):
        """
        Carrega instâncias do LiveCodeBench do dataset do Hugging Face.
        Se falhar, carrega desafios clássicos pré-cadastrados (LeetCode mock).
        """
        try:
            if mock:
                raise Exception("Mock mode bypass HF download")
            from datasets import load_dataset
            print("📥 Baixando LiveCodeBench do Hugging Face...")
            ds = load_dataset("livecodebench/code_generation", split="test")
            instances = []
            for i in range(min(limit, len(ds))):
                item = ds[i]
                instances.append({
                    "task_id": f"lcb-{item.get('question_id', i)}",
                    "name": item.get("question_title", f"Challenge {i}"),
                    "description": item.get("question_content", ""),
                    "tests": item.get("test_cases", [])
                })
            return instances
        except Exception as e:
            print(f"⚠️ Falha ao carregar dataset remoto ({e}). Usando desafios mocks locais.")
            return [
                {
                    "task_id": "lcb-01",
                    "name": "Two Sum",
                    "description": "Dado um array de inteiros 'nums' e um inteiro 'target', retorne os índices dos dois números cuja soma seja igual a 'target'. Salve em um arquivo 'solucao.py' uma função 'two_sum(nums, target) -> list'.",
                    "tests": [
                        {"input": "([2, 7, 11, 15], 9)", "output": "[0, 1]"},
                        {"input": "([3, 2, 4], 6)", "output": "[1, 2]"}
                    ],
                    "func_name": "two_sum"
                },
                {
                    "task_id": "lcb-02",
                    "name": "Fibonacci Number",
                    "description": "Escreva uma função 'fib(n) -> int' que retorna o n-ésimo número de Fibonacci. Salve em um arquivo 'solucao.py'.",
                    "tests": [
                        {"input": "(2)", "output": "1"},
                        {"input": "(4)", "output": "3"}
                    ],
                    "func_name": "fib"
                }
            ][:limit]

    def run_instance(self, instance, max_iterations=30, timeout=120):
        """
        Executa uma tarefa do LiveCodeBench criando um workspace local temporário.
        """
        task_id = instance["task_id"]
        name = instance["name"]
        description = instance["description"]
        tests = instance["tests"]
        func_name = instance.get("func_name", "solve")
        
        print(f"\n🚀 [LiveCodeBench] Iniciando tarefa: {name} ({task_id})")
        
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            
            prompt = (
                f"Escreva um script Python em 'solucao.py' contendo uma função chamada '{func_name}' que resolva o seguinte problema:\n\n"
                f"{description}\n\n"
                f"Certifique-se de que a função aceite os parâmetros descritos e retorne o resultado correto."
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
            
            # Valida o resultado rodando os casos de teste contra solucao.py
            sol_file = temp_path / "solucao.py"
            if not sol_file.exists():
                stats["success"] = False
                stats["status"] = "failed_no_output_file"
                return stats
                
            # Executa validação de casos de teste
            success = True
            try:
                # Carrega dinamicamente a função
                import sys
                sys.path.insert(0, str(temp_path))
                
                # Para evitar conflito ou poluição de imports, executamos via script Python em subprocesso
                for t in tests:
                    t_input = t["input"]
                    t_expected = t["output"]
                    
                    eval_code = (
                        f"import sys; sys.path.insert(0, '{temp_path}'); "
                        f"from solucao import {func_name}; "
                        f"res = {func_name}{t_input}; "
                        f"print(repr(res))"
                    )
                    
                    res = subprocess.run(
                        [sys.executable, "-c", eval_code],
                        capture_output=True,
                        text=True,
                        timeout=5
                    )
                    
                    if res.returncode != 0:
                        success = False
                        break
                        
                    output_got = res.stdout.strip()
                    # Compara saídas de forma genérica removendo espaços
                    if output_got.replace(" ", "") != t_expected.replace(" ", ""):
                        success = False
                        break
            except Exception as e:
                print(f"Erro ao testar a solução: {e}")
                success = False
                
            stats["success"] = success
            stats["status"] = "success" if success else "failed_test_cases"
            return stats
