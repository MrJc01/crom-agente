import os
import sys
import json
import time
import subprocess
import shutil
from pathlib import Path

# Paths
BASE_DIR = Path("/home/j/Documentos/GitHub/crom-agente")
SIMS_DIR = BASE_DIR / "simulations"
BINARY_PATH = BASE_DIR / "build" / "crom-agente"

# Prompts and workspace paths for the 10 simulations
SIMULATIONS = [
    {
        "id": 1,
        "name": "Greeting (Fast Path)",
        "prompt": "Olá",
        "dir": "sim01_greeting"
    },
    {
        "id": 2,
        "name": "Yes/No Reply (Fast Path)",
        "prompt": "Sim",
        "dir": "sim02_yes_no"
    },
    {
        "id": 3,
        "name": "Report Generation (Basic)",
        "prompt": "Crie um arquivo chamado 'relatorio_clima.md' contendo um relatório simples sobre o clima no Brasil em 3 parágrafos.",
        "dir": "sim03_report"
    },
    {
        "id": 4,
        "name": "Factorial Script (Basic)",
        "prompt": "Escreva um script em Python chamado 'fatorial.py' que calcula o fatorial de 5 e imprime o resultado.",
        "dir": "sim04_factorial"
    },
    {
        "id": 5,
        "name": "JSON Parser (Medium)",
        "prompt": "Crie um script em Python chamado 'formatador.py' que lê um arquivo JSON 'dados.json' e imprime formatado. Adicione também um arquivo 'dados.json' com dados fictícios de teste.",
        "dir": "sim05_json_parser"
    },
    {
        "id": 6,
        "name": "SQLite Product Table (Medium)",
        "prompt": "Crie um script Python 'db.py' que cria uma tabela de produtos num banco SQLite local 'vendas.db', insere 3 itens e faz uma query listando-os no console.",
        "dir": "sim06_sqlite"
    },
    {
        "id": 7,
        "name": "HTML Static Page (Medium)",
        "prompt": "Gere uma página HTML 'index.html' elegante e minimalista com CSS inline estilizando um portfólio profissional de desenvolvedor.",
        "dir": "sim07_portfolio"
    },
    {
        "id": 8,
        "name": "Go HTTP Server (Advanced)",
        "prompt": "Crie um servidor web simples em Go em 'server.go' com um endpoint '/' que retorna a mensagem 'Hello, Crom!' em formato JSON. Adicione um comando terminal no seu plano para compilar/verificar o arquivo.",
        "dir": "sim08_go_server"
    },
    {
        "id": 9,
        "name": "File Organizer (Advanced)",
        "prompt": "Crie um script Python 'organizer.py' que organiza arquivos de uma pasta movendo arquivos com extensão .txt para a pasta 'textos' e .json para a pasta 'dados'.",
        "dir": "sim09_file_organizer"
    },
    {
        "id": 10,
        "name": "Yii2 MVC PHP SQLite (Complex)",
        "prompt": "Crie a estrutura de arquivos para um template de projeto PHP Yii2 MVC usando SQLite. Crie os arquivos 'config/db.php' com a conexão PDO SQLite, um model ActiveRecord em 'models/Item.php', e um controller em 'controllers/ItemController.php'.",
        "dir": "sim10_yii2_sqlite"
    }
]

def load_env_vars():
    env_file = Path("/home/j/.crom/.env")
    env = os.environ.copy()
    if env_file.exists():
        with open(env_file) as f:
            for line in f:
                if "=" in line and not line.strip().startswith("#"):
                    k, v = line.strip().split("=", 1)
                    env[k] = v
    return env

def get_session_stats(workspace_dir):
    crom_dir = workspace_dir / ".crom"
    state_file = crom_dir / ".crom_state.json"
    if state_file.exists():
        try:
            with open(state_file) as f:
                data = json.load(f)
                return {
                    "total_turns": data.get("total_turnos", 0),
                    "tokens_spent": data.get("tokens_gastos", 0),
                    "status": data.get("status_operacional", data.get("ultimo_status", "unknown")),
                    "cognitive_mode": data.get("modo_cognitive", data.get("modo_cognitivo", "unknown"))
                }
        except Exception as e:
            print(f"Error reading .crom_state.json: {e}")
            
    sessions_dir = crom_dir / "sessions"
    if sessions_dir.exists():
        sessions = sorted(list(sessions_dir.glob("session-*")), key=os.path.getmtime)
        if sessions:
            session_json = sessions[-1] / "session.json"
            if not session_json.exists():
                session_json = sessions[-1] / ".crom_state.json"
            if session_json.exists():
                try:
                    with open(session_json) as f:
                        data = json.load(f)
                        return {
                            "total_turns": data.get("total_turnos", 0),
                            "tokens_spent": data.get("tokens_gastos", 0),
                            "status": data.get("status_operacional", data.get("ultimo_status", "unknown")),
                            "cognitive_mode": data.get("modo_cognitive", data.get("modo_cognitivo", "unknown"))
                        }
                except Exception as e:
                    print(f"Error reading session file: {e}")
    return {}

import urllib.request
import urllib.error

def check_preflight(provider, env):
    print("\n==================================================")
    print("Realizando verificações pré-run (Preflight Checks)")
    print("==================================================")
    
    # 1. Check API Key
    key_name = None
    if provider == "openrouter":
        key_name = "OPENROUTER_API_KEY"
    elif provider == "openai":
        key_name = "OPENAI_API_KEY"
    elif provider == "gemini":
        key_name = "GEMINI_API_KEY"
    elif provider == "anthropic":
        key_name = "ANTHROPIC_API_KEY"

    if key_name:
        if key_name not in env or not env[key_name].strip():
            print(f"❌ ERRO: A variável de ambiente '{key_name}' não está definida ou está vazia.", file=sys.stderr)
            return False
        print(f"✓ Variável de ambiente '{key_name}' encontrada.")

    # 2. Check Network Connection/Reachability
    test_url = "https://openrouter.ai/api/v1/models" if provider == "openrouter" else "https://www.google.com"
    print(f"Testando conectividade de rede com {test_url}...")
    try:
        req = urllib.request.Request(
            test_url,
            headers={"User-Agent": "Cromia-Simulation-Preflight"}
        )
        with urllib.request.urlopen(req, timeout=5) as response:
            if response.status == 200:
                print("✓ Conectividade de rede estabelecida com sucesso.")
                return True
    except Exception as e:
        print(f"❌ ERRO: Falha ao conectar com o provedor/internet ({test_url}): {e}", file=sys.stderr)
        return False
    return True

def run_simulation(sim, env, provider, model, max_iterations, timeout=None):
    sim_dir = SIMS_DIR / sim["dir"]
    
    # Clean workspace folder if it exists
    if sim_dir.exists():
        shutil.rmtree(sim_dir)
    sim_dir.mkdir(parents=True, exist_ok=True)
    
    # Determina o timeout específico da simulação
    actual_timeout = timeout if timeout is not None else (180 if sim["id"] >= 8 else 120)
    
    print(f"\n==================================================")
    print(f"Iniciando Simulação {sim['id']}: {sim['name']}")
    print(f"Workspace: {sim_dir}")
    print(f"Prompt: {sim['prompt']}")
    print(f"Timeout: {actual_timeout}s")
    print(f"==================================================")
    
    cmd = [
        str(BINARY_PATH), "run", sim["prompt"],
        "--provider", provider,
        "--model", model,
        "--workspace", str(sim_dir),
        "--permission-mode", "total_access",
        "--max-iterations", str(max_iterations),
        "--disable-prompt-optimization"
    ]
    
    start_time = time.time()
    return_code = -1
    try:
        res = subprocess.run(cmd, env=env, cwd=str(sim_dir), capture_output=True, text=True, timeout=actual_timeout)
        elapsed = time.time() - start_time
        success = res.returncode == 0
        return_code = res.returncode
        output = res.stdout + "\n" + res.stderr
    except subprocess.TimeoutExpired as te:
        elapsed = time.time() - start_time
        success = False
        output = f"TIMEOUT EXPIRED: {te}\nSTDOUT: {te.stdout or ''}\nSTDERR: {te.stderr or ''}"
    
    print(f"Tempo decorrido: {elapsed:.2f}s")
    print(f"Código de retorno: {return_code if return_code != -1 else 'timeout'}")
    
    stats = get_session_stats(sim_dir)
    
    return {
        "id": sim["id"],
        "name": sim["name"],
        "prompt": sim["prompt"],
        "dir": sim["dir"],
        "elapsed_seconds": elapsed,
        "success": success,
        "total_turns": stats.get("total_turns", 0),
        "tokens_spent": stats.get("tokens_spent", 0),
        "status": stats.get("status", "unknown"),
        "cognitive_mode": stats.get("cognitive_mode", "unknown"),
        "output_snippet": output[-500:] if len(output) > 500 else output
    }

def main():
    import argparse
    parser = argparse.ArgumentParser(description="Executa as 10 simulações de templates de projeto.")
    parser.add_argument("--model", type=str, default="meta-llama/llama-3.1-8b-instruct", help="Modelo de LLM a usar (separe por vírgula para rodar múltiplos)")
    parser.add_argument("--provider", type=str, default="openrouter", help="Provedor de LLM")
    parser.add_argument("--max-iterations", type=int, default=0, help="Limite máximo de iterações (0 = ilimitado)")
    parser.add_argument("--timeout", type=int, default=None, help="Tempo limite customizado (em segundos) para cada simulação")
    args = parser.parse_args()

    SIMS_DIR.mkdir(parents=True, exist_ok=True)
    env = load_env_vars()
    env["CROM_PERMISSION_MODE"] = "total_access"
    
    # Executar verificações pré-run
    if not check_preflight(args.provider, env):
        print("\n❌ ERRO: Verificações pré-run falharam. Abortando execução.", file=sys.stderr)
        sys.exit(1)
    
    models = [m.strip() for m in args.model.split(",") if m.strip()]
    all_model_results = {}
    
    for idx, model in enumerate(models):
        print(f"\n==================================================")
        print(f"INICIANDO SIMULAÇÕES PARA O MODELO: {model} ({idx+1}/{len(models)})")
        print(f"==================================================")
        
        results = []
        for sim in SIMULATIONS:
            res = run_simulation(sim, env, args.provider, model, args.max_iterations, args.timeout)
            results.append(res)
            time.sleep(2)
            
        model_safe = model.replace("/", "_").replace(":", "_")
        summary_file = SIMS_DIR / f"simulations_summary_{model_safe}.json"
        with open(summary_file, "w") as f:
            json.dump(results, f, indent=2)
            
        report_file = SIMS_DIR / f"simulations_report_{model_safe}.md"
        md = []
        md.append(f"# Relatório de Simulações do crom-agente via {args.provider.capitalize()}")
        md.append(f"\nData de execução: {time.strftime('%Y-%m-%d %H:%M:%S')}")
        md.append(f"Modelo utilizado: `{model}` via {args.provider}")
        md.append("\n## Tabela de Resultados das Simulações")
        md.append("\n| ID | Nome da Simulação | Status Final | Modo Cognitivo | Turnos | Tokens Gasto | Tempo (s) | Sucesso |")
        md.append("|---|---|---|---|---|---|---|---|")
        
        total_tokens = 0
        total_time = 0.0
        successful_runs = 0
        
        for r in results:
            succ_emoji = "✅" if r["success"] else "❌"
            md.append(f"| {r['id']} | {r['name']} | `{r['status']}` | `{r['cognitive_mode']}` | {r['total_turns']} | {r['tokens_spent']} | {r['elapsed_seconds']:.2f}s | {succ_emoji} |")
            total_tokens += r["tokens_spent"]
            total_time += r["elapsed_seconds"]
            if r["success"]:
                successful_runs += 1
                
        price_per_1m = 0.055 if "8b" in model.lower() or "9b" in model.lower() else 0.075
        md.append(f"\n### Métricas Consolidadas")
        md.append(f"- **Simulações Executadas**: {len(results)}")
        md.append(f"- **Taxa de Sucesso**: {successful_runs}/{len(results)} ({successful_runs/len(results)*100:.1f}%)")
        md.append(f"- **Total de Tokens Consumidos**: {total_tokens}")
        md.append(f"- **Custo Estimado**: ${(total_tokens / 1000000) * price_per_1m:.6f} USD (baseado em ${price_per_1m} por 1M tokens)")
        md.append(f"- **Tempo Total de Execução**: {total_time:.2f} segundos")
        md.append(f"- **Média de Tempo por Simulação**: {total_time/len(results):.2f} segundos")
        
        with open(report_file, "w") as f:
            f.write("\n".join(md))
            
        print(f"\n✓ Relatório para o modelo {model} gravado em: {report_file}")
        all_model_results[model] = results

    # Gerar relatório comparativo automático se houver múltiplos modelos
    if len(models) > 1:
        comp_report_file = SIMS_DIR / "simulations_comparative_report.md"
        comp_md = []
        comp_md.append("# Relatório Comparativo de Modelos (Simulações)")
        comp_md.append(f"\nData de execução: {time.strftime('%Y-%m-%d %H:%M:%S')}")
        comp_md.append(f"Provedor: `{args.provider}`\n")
        
        # Tabela resumo comparativa geral
        comp_md.append("## Resumo Geral dos Modelos\n")
        comp_md.append("| Modelo | Taxa de Sucesso | Tokens Totais | Tempo Total (s) | Média por Simulação (s) |")
        comp_md.append("|---|---|---|---|---|")
        for model in models:
            m_res = all_model_results[model]
            succ = sum(1 for r in m_res if r["success"])
            tok = sum(r["tokens_spent"] for r in m_res)
            t_total = sum(r["elapsed_seconds"] for r in m_res)
            comp_md.append(f"| `{model}` | {succ}/{len(m_res)} ({succ/len(m_res)*100:.1f}%) | {tok} | {t_total:.2f}s | {t_total/len(m_res):.2f}s |")
            
        # Detalhe por simulação
        comp_md.append("\n## Detalhamento Lado-a-Lado por Simulação\n")
        
        # Cabeçalho dinâmico para os modelos
        header_cols = ["ID", "Nome da Simulação"]
        sub_header = ["---|---"]
        for m in models:
            m_short = m.split("/")[-1]
            header_cols.extend([f"[{m_short}] Sucesso", f"[{m_short}] Turnos", f"[{m_short}] Tempo"])
            sub_header.extend(["---|---|---"])
        
        comp_md.append("| " + " | ".join(header_cols) + " |")
        comp_md.append("| " + " | ".join(sub_header) + " |")
        
        for idx in range(len(SIMULATIONS)):
            sim = SIMULATIONS[idx]
            row = [str(sim["id"]), sim["name"]]
            for model in models:
                res_sim = all_model_results[model][idx]
                succ_emoji = "✅" if res_sim["success"] else "❌"
                row.extend([succ_emoji, str(res_sim["total_turns"]), f"{res_sim['elapsed_seconds']:.1f}s"])
            comp_md.append("| " + " | ".join(row) + " |")
            
        with open(comp_report_file, "w") as f:
            f.write("\n".join(comp_md))
        print(f"\n==================================================")
        print(f"✓ Relatório Comparativo de Modelos gravado em: {comp_report_file}")
        print(f"==================================================")

if __name__ == "__main__":
    main()
