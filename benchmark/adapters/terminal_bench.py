import os
import json
import tempfile
import time
from pathlib import Path
from ..shared.container_sandbox import DockerSandbox

class TerminalBenchAdapter:
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

    def load_instances(self, limit=3, mock=False):
        """
        Define cenários reais do Terminal-Bench.
        Cria definições de tarefas que simulam ambientes quebrados.
        """
        scenarios = [
            {
                "task_id": "tb-go-build-error",
                "name": "Consertar importação quebrada no Go",
                "image": "golang:1.22-bookworm",
                "broken_setup": [
                    "mkdir -p /workspace/go-app",
                    "echo 'package main\\n\\nimport (\\n\\t\"fmt\"\\n\\t\"github.com/nonexistent/package\"\\n)\\n\\nfunc main() {\\n\\tfmt.Println(\"Hello\")\\n}' > /workspace/go-app/main.go",
                    "echo 'module go-app\\n\\ngo 1.22' > /workspace/go-app/go.mod"
                ],
                "prompt": "O build deste projeto Go está quebrando devido a uma dependência inexistente importada. Remova a dependência inválida ou corrija o código de forma que ele compile com 'go build'.",
                "validation_cmd": "go build",
                "workdir": "/workspace/go-app"
            },
            {
                "task_id": "tb-node-dep-conflict",
                "name": "Resolver conflito de dependência Node",
                "image": "node:20-slim",
                "broken_setup": [
                    "mkdir -p /workspace/node-app",
                    "echo '{\"name\": \"node-app\", \"version\": \"1.0.0\", \"scripts\": { \"build\": \"node build.js\" }, \"dependencies\": { \"lodash\": \"invalid-version\" }}' > /workspace/node-app/package.json",
                    "echo 'console.log(\"Building...\"); const _ = require(\"lodash\"); console.log(\"Success!\");' > /workspace/node-app/build.js"
                ],
                "prompt": "O package.json possui uma versão inválida para a biblioteca lodash. Altere para uma versão válida (ex: ^4.17.21) ou instale-a corretamente, de forma que o script 'npm run build' execute sem falhar.",
                "validation_cmd": "npm install && npm run build",
                "workdir": "/workspace/node-app"
            },
            {
                "task_id": "tb-python-env-fix",
                "name": "Corrigir erro de ambiente virtual Python",
                "image": "python:3.10-slim",
                "broken_setup": [
                    "mkdir -p /workspace/py-app",
                    "echo 'import numpy\\nprint(\"Loaded Numpy Successfully!\")' > /workspace/py-app/app.py"
                ],
                "prompt": "O arquivo 'app.py' tenta importar o pacote 'numpy' que não está instalado no ambiente. Instale a biblioteca numpy no container e garanta que 'python app.py' funcione.",
                "validation_cmd": "pip install numpy && python app.py",
                "workdir": "/workspace/py-app"
            }
        ]
        return scenarios[:limit]

    def run_instance(self, instance, max_iterations=30, timeout=180):
        """
        Executa um cenário do Terminal-Bench.
        """
        task_id = instance["task_id"]
        name = instance["name"]
        image = instance["image"]
        setup_cmds = instance["broken_setup"]
        prompt_text = instance["prompt"]
        validation_cmd = instance["validation_cmd"]
        workdir = instance["workdir"]

        print(f"\n🚀 [Terminal-Bench] Iniciando cenário: {name} ({task_id})")
        
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            
            # Inicializa sandbox
            sandbox = DockerSandbox(
                image=image,
                container_name=f"terminal-bench-{task_id}",
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
                # 1. Roda comandos de quebra no setup
                for setup_cmd in setup_cmds:
                    sandbox.exec_run(["bash", "-c", setup_cmd], timeout=15)
                
                # 2. Transfere o binário do crom-agente
                bin_path = Path(__file__).resolve().parent.parent.parent / "bin" / "crom-agente"
                if not bin_path.exists():
                    from ..shared.agent_runner import build_agent
                    build_agent()
                sandbox.copy_to_container(bin_path, "/usr/local/bin/crom-agente")
                sandbox.exec_run("chmod +x /usr/local/bin/crom-agente")
                
                # 3. Executa o crom-agente
                agent_cmd = [
                    "crom-agente", "run", prompt_text,
                    "--provider", self.provider,
                    "--model", self.model,
                    "--workspace", workdir,
                    "--permission-mode", "total_access",
                    "--max-iterations", str(max_iterations),
                    "--disable-prompt-optimization"
                ]
                
                start_time = time.time()
                exit_code, output = sandbox.exec_run(agent_cmd, workdir=workdir, timeout=timeout)
                elapsed = time.time() - start_time
                
                # 4. Lê telemetria local
                sandbox.copy_from_container(f"{workdir}/.crom/.crom_state.json", str(temp_path / ".crom_state.json"))
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
                
                # 5. Executa comando de validação para verificar se resolveu a quebra
                val_exit_code, val_output = sandbox.exec_run(["bash", "-c", validation_cmd], workdir=workdir, timeout=30)
                success = (val_exit_code == 0)
                
                stats["elapsed_seconds"] = elapsed
                stats["success"] = success
                stats["status"] = "success" if success else "failed"
                stats["output"] = output
                
                return stats
                
            finally:
                sandbox.stop()
