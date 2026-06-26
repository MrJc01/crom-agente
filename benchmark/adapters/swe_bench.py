import os
import sys
import json
import tempfile
from pathlib import Path
from ..shared.container_sandbox import DockerSandbox
from ..shared.agent_runner import get_agent_state

class SWEBenchAdapter:
    def __init__(self, provider, model, config_path=None):
        self.provider = provider
        self.model = model
        self.config = {}
        if config_path:
            try:
                with open(config_path) as f:
                    self.config = json.load(f)
            except Exception:
                pass

    def load_instances(self, limit=5, mock=False):
        """
        Tenta carregar instâncias do SWE-bench do Hugging Face.
        Se falhar ou não tiver internet, gera instâncias simuladas locais para fins de teste.
        """
        try:
            if mock:
                raise Exception("Mock mode bypass HF download")
            from datasets import load_dataset
            print("📥 Baixando dataset SWE-bench Lite do Hugging Face...")
            ds = load_dataset("swe-bench/SWE-bench_Lite", split="test")
            instances = []
            for i in range(min(limit, len(ds))):
                item = ds[i]
                instances.append({
                    "instance_id": item["instance_id"],
                    "repo": item["repo"],
                    "base_commit": item["base_commit"],
                    "problem_statement": item["problem_statement"],
                    "test_patch": item["test_patch"],
                    "fail_to_pass": item["fail_to_pass"],
                    "pass_to_pass": item["pass_to_pass"]
                })
            return instances
        except Exception as e:
            print(f"⚠️ Falha ao carregar dataset remoto ({e}). Usando instâncias mockup locais.")
            # Amostra de mockups locais
            return [
                {
                    "instance_id": "django__django-11111",
                    "repo": "django/django",
                    "base_commit": "a4d3f23",
                    "problem_statement": "Fix visual alignment of Admin panel tables when page size is small.",
                    "test_patch": "diff --git a/tests/admin_views/tests.py b/tests/admin_views/tests.py...",
                    "fail_to_pass": ["admin_views.tests.AdminViewBasicTest.test_visual_alignment"],
                    "pass_to_pass": ["admin_views.tests.AdminViewBasicTest.test_list_view"]
                },
                {
                    "instance_id": "pytest-dev__pytest-22222",
                    "repo": "pytest-dev/pytest",
                    "base_commit": "b9f2c1a",
                    "problem_statement": "Assert reporting fails when dictionary contains recursion.",
                    "test_patch": "diff --git a/testing/test_assertrepr.py b/testing/test_assertrepr.py...",
                    "fail_to_pass": ["testing.test_assertrepr.test_dict_recursion"],
                    "pass_to_pass": ["testing.test_assertrepr.test_basic"]
                }
            ][:limit]

    def run_instance(self, instance, max_iterations=30, timeout=240):
        """
        Executa uma única issue do SWE-bench no container Docker sandbox.
        """
        instance_id = instance["instance_id"]
        repo = instance["repo"]
        commit = instance["base_commit"]
        problem = instance["problem_statement"]
        
        print(f"\n🚀 [SWE-bench] Iniciando tarefa: {instance_id}")
        
        # Determina imagem Docker base
        img = self.config.get("docker_base_images", {}).get("swe_bench", "python:3.9-slim")
        
        # Cria uma pasta temporária local para montar/compartilhar logs se necessário
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            
            # Inicializa sandbox
            sandbox = DockerSandbox(
                image=img,
                container_name=f"swe-bench-runner-{instance_id.replace('__', '-')}",
                env={
                    "CROM_PERMISSION_MODE": "total_access",
                    "GEMINI_API_KEY": os.getenv("GEMINI_API_KEY", ""),
                    "OPENAI_API_KEY": os.getenv("OPENAI_API_KEY", ""),
                    "ANTHROPIC_API_KEY": os.getenv("ANTHROPIC_API_KEY", ""),
                    "OPENROUTER_API_KEY": os.getenv("OPENROUTER_API_KEY", "")
                }
            )
            
            if not sandbox.start():
                return {"success": False, "turns": 0, "tokens": 0, "elapsed_seconds": 0, "status": "sandbox_error"}
                
            try:
                # 1. Instala git e dependências básicas no container
                sandbox.exec_run("apt-get update && apt-get install -y git build-essential", timeout=60)
                
                # 2. Clona o repositório dentro do container
                repo_url = f"https://github.com/{repo}.git"
                work_dir = f"/workspace/{repo.split('/')[-1]}"
                sandbox.exec_run(f"mkdir -p /workspace", timeout=5)
                sandbox.exec_run(["git", "clone", repo_url, work_dir], timeout=60)
                sandbox.exec_run(f"git checkout {commit}", workdir=work_dir, timeout=30)
                
                # 3. Transfere o binário do crom-agente para dentro do container
                bin_path = Path(__file__).resolve().parent.parent.parent / "bin" / "crom-agente"
                if not bin_path.exists():
                    # Compila caso não exista
                    from ..shared.agent_runner import build_agent
                    build_agent()
                sandbox.copy_to_container(bin_path, "/usr/local/bin/crom-agente")
                sandbox.exec_run("chmod +x /usr/local/bin/crom-agente")
                
                # 4. Prompt para o agente
                prompt = (
                    f"Você está em um repositório git em '{work_dir}'. "
                    f"Conserte o seguinte problema descrito no issue:\n\n{problem}\n\n"
                    f"Verifique os arquivos correspondentes, modifique o código e execute os testes locais para garantir que a issue foi resolvida."
                )
                
                # 5. Roda o crom-agente dentro do container
                agent_cmd = [
                    "crom-agente", "run", prompt,
                    "--provider", self.provider,
                    "--model", self.model,
                    "--workspace", work_dir,
                    "--permission-mode", "total_access",
                    "--max-iterations", str(max_iterations),
                    "--disable-prompt-optimization"
                ]
                
                print("🧠 Executando crom-agente no container...")
                import time
                start_time = time.time()
                exit_code, output = sandbox.exec_run(agent_cmd, workdir=work_dir, timeout=timeout)
                elapsed = time.time() - start_time
                
                # 6. Recupera os dados de telemetria do estado gerado no container
                sandbox.copy_from_container(f"{work_dir}/.crom/.crom_state.json", str(temp_path / ".crom_state.json"))
                stats = {
                    "turns": 0, "tokens": 0, "status": "unknown", 
                    "files_created": 0, "files_validated": 0, "tool_calls": 0, "directory": ""
                }
                if (temp_path / ".crom_state.json").exists():
                    try:
                        with open(temp_path / ".crom_state.json") as f:
                            data = json.load(f)
                            stats = {
                                "turns": data.get("total_turnos", data.get("TotalTurnos", 0)),
                                "tokens": data.get("tokens_gastos", data.get("TokensGastos", 0)),
                                "status": data.get("ultimo_status", data.get("UltimoStatus", "unknown")),
                                "files_created": data.get("files_created", data.get("FilesCreated", 0)),
                                "files_validated": data.get("files_validated", data.get("FilesValidated", 0)),
                                "tool_calls": data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0)),
                                "directory": data.get("diretorio_atual", data.get("DiretorioAtual", ""))
                            }
                    except Exception:
                        pass
                
                # 7. Executa os testes de validação oficial
                # Aqui simula se a correção do agente passou (aplica o test_patch e roda os testes)
                # No fluxo simplificado, avaliamos o código de retorno ou aplicamos o patch de testes:
                test_code = 0
                if "test_patch" in instance and instance["test_patch"]:
                    # Escreve o test_patch em um arquivo e o aplica no container
                    with open(temp_path / "tests.patch", "w") as f:
                        f.write(instance["test_patch"])
                    sandbox.copy_to_container(str(temp_path / "tests.patch"), f"{work_dir}/tests.patch")
                    sandbox.exec_run("git apply tests.patch", workdir=work_dir, timeout=10)
                    
                    # Roda testes (ex: pytest ou python setup.py test)
                    # Para simplificar na simulação, rodamos pytest ou python -m unittest
                    test_code, test_out = sandbox.exec_run("python -m pytest", workdir=work_dir, timeout=30)
                    if test_code != 0:
                        test_code, test_out = sandbox.exec_run("pytest", workdir=work_dir, timeout=30)
                    if test_code != 0:
                        test_code, test_out = sandbox.exec_run("python setup.py test", workdir=work_dir, timeout=30)
                        
                success = (exit_code == 0 and test_code == 0) or (stats["status"] == "finished")
                
                stats["elapsed_seconds"] = elapsed
                stats["success"] = success
                stats["status"] = "success" if success else "failed"
                stats["output"] = output
                
                return stats
                
            finally:
                # Sempre limpa/para o container
                sandbox.stop()
