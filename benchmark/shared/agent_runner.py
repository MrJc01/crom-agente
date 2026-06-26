import os
import sys
import json
import subprocess
import time
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent.parent
BINARY_PATH = BASE_DIR / "bin" / "crom-agente"

def build_agent():
    """
    Compila o executável Go do crom-agente em modo headless.
    """
    print("🔨 Compilando crom-agente (tags=headless)...")
    bin_dir = BASE_DIR / "bin"
    bin_dir.mkdir(exist_ok=True)
    
    cmd = ["go", "build", "-tags", "headless", "-o", str(BINARY_PATH), "./cmd/crom-agente"]
    res = subprocess.run(cmd, cwd=str(BASE_DIR), capture_output=True, text=True)
    if res.returncode != 0:
        print(f"❌ Erro ao compilar crom-agente:\n{res.stderr}", file=sys.stderr)
        return False
    print(f"✓ Binário compilado com sucesso em: {BINARY_PATH}")
    return True

def get_agent_state(workspace_dir):
    """
    Carrega o arquivo de estado .crom_state.json ou arquivos de sessão do workspace
    para extrair estatísticas de execução (turnos, tokens, etc.).
    """
    workspace_path = Path(workspace_dir)
    state_file = workspace_path / ".crom" / ".crom_state.json"
    
    # 1. Tenta ler o estado global no workspace
    if state_file.exists():
        try:
            with open(state_file) as f:
                data = json.load(f)
                return {
                    "turns": data.get("total_turnos", data.get("TotalTurnos", 0)),
                    "tokens": data.get("tokens_gastos", data.get("TokensGastos", 0)),
                    "status": data.get("ultimo_status", data.get("UltimoStatus", "unknown")),
                    "files_created": data.get("files_created", data.get("FilesCreated", 0)),
                    "files_validated": data.get("files_validated", data.get("FilesValidated", 0)),
                    "tool_calls": data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0)),
                    "directory": data.get("diretorio_atual", data.get("DiretorioAtual", ""))
                }
        except Exception as e:
            print(f"Erro ao ler .crom_state.json: {e}")

    # 2. Tenta ler da última sessão
    sessions_dir = workspace_path / ".crom" / "sessions"
    if sessions_dir.exists():
        sessions = sorted(list(sessions_dir.glob("session-*")), key=os.path.getmtime)
        if sessions:
            for filename in [".crom_state.json", "session.json"]:
                session_json = sessions[-1] / filename
                if session_json.exists():
                    try:
                        with open(session_json) as f:
                            data = json.load(f)
                            return {
                                "turns": data.get("total_turnos", data.get("TotalTurnos", 0)),
                                "tokens": data.get("tokens_gastos", data.get("TokensGastos", 0)),
                                "status": data.get("ultimo_status", data.get("UltimoStatus", "unknown")),
                                "files_created": data.get("files_created", data.get("FilesCreated", 0)),
                                "files_validated": data.get("files_validated", data.get("FilesValidated", 0)),
                                "tool_calls": data.get("tool_calls_emitted", data.get("ToolCallsEmitted", 0)),
                                "directory": data.get("diretorio_atual", data.get("DiretorioAtual", ""))
                            }
                    except Exception as e:
                        pass
    
    return {
        "turns": 0,
        "tokens": 0,
        "status": "unknown",
        "files_created": 0,
        "files_validated": 0,
        "tool_calls": 0,
        "directory": ""
    }

def run_agent_task(task, workspace_dir, provider, model, max_iterations=30, timeout=180, env_override=None):
    """
    Executa o agente em uma tarefa específica dentro do workspace local.
    """
    workspace_path = Path(workspace_dir)
    workspace_path.mkdir(parents=True, exist_ok=True)
    
    # Limpa estados anteriores se existirem
    crom_dir = workspace_path / ".crom"
    if crom_dir.exists():
        try:
            import shutil
            shutil.rmtree(crom_dir)
        except Exception:
            pass
            
    cmd = [
        str(BINARY_PATH), "run", task,
        "--provider", provider,
        "--model", model,
        "--workspace", str(workspace_path),
        "--permission-mode", "total_access",
        "--max-iterations", str(max_iterations),
        "--disable-prompt-optimization"
    ]
    
    env = os.environ.copy()
    env["CROM_PERMISSION_MODE"] = "total_access"
    if env_override:
        env.update(env_override)
        
    start_time = time.time()
    try:
        res = subprocess.run(
            cmd,
            env=env,
            cwd=str(workspace_path),
            capture_output=True,
            text=True,
            errors="replace",
            timeout=timeout
        )
        elapsed = time.time() - start_time
        success = res.returncode == 0
        output = res.stdout + "\n" + res.stderr
    except subprocess.TimeoutExpired as te:
        elapsed = time.time() - start_time
        success = False
        output = f"TIMEOUT EXPIRED after {timeout}s\nSTDOUT:\n{te.stdout or ''}\nSTDERR:\n{te.stderr or ''}"
        
    state = get_agent_state(workspace_path)
    state["elapsed_seconds"] = elapsed
    state["success"] = success or (state["status"] in ["finished", "success"])
    state["output"] = output
    
    return state
