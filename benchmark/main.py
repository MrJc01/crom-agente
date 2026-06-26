import os
import sys
import json
import argparse
import time
from pathlib import Path

# Adiciona diretório raiz ao path para importação correta
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from benchmark.shared.agent_runner import build_agent
from benchmark.shared.metrics import MetricsTracker
from benchmark.adapters.swe_bench import SWEBenchAdapter
from benchmark.adapters.terminal_bench import TerminalBenchAdapter
from benchmark.adapters.livecode_bench import LiveCodeBenchAdapter
from benchmark.adapters.evalplus import EvalPlusAdapter
from benchmark.adapters.composite import CompositeMetrics

CONFIG_FILE = Path(__file__).resolve().parent / "config.json"
REPORTS_DIR = Path(__file__).resolve().parent / "reports"

def load_config():
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE) as f:
                return json.load(f)
        except Exception as e:
            print(f"Erro ao ler config.json: {e}")
    return {}

def main():
    parser = argparse.ArgumentParser(description="Orquestrador de Benchmarks do crom-agente")
    parser.add_argument("action", choices=["run", "compare"], help="Ação a ser executada")
    parser.add_argument("--benchmark", choices=["swe-bench", "terminal-bench", "livecode-bench", "evalplus", "bigcodebench"], 
                        default="evalplus", help="Qual benchmark executar")
    parser.add_argument("--provider", help="Override do provedor de LLM (ex: openrouter, gemini)")
    parser.add_argument("--model", help="Override do modelo de LLM (ex: meta-llama/llama-3.1-8b-instruct)")
    parser.add_argument("--limit", type=int, default=5, help="Limite máximo de instâncias para rodar")
    parser.add_argument("--max-iterations", type=int, help="Número máximo de turnos por tarefa")
    parser.add_argument("--timeout", type=int, help="Timeout em segundos para cada tarefa")
    parser.add_argument("--mock-agent", action="store_true", help="Usa execução mock para testar a suíte sem gastar tokens")
    parser.add_argument("--output-dir", default=str(REPORTS_DIR), help="Pasta para salvar os relatórios")
    parser.add_argument("--temp", type=float, help="Temperatura do modelo LLM")
    parser.add_argument("--top-p", type=float, help="Top-P do modelo LLM")
    parser.add_argument("--max-context-tokens", type=int, help="Máximo de tokens permitidos no histórico")
    
    
    args = parser.parse_args()
    
    config = load_config()
    provider = args.provider or config.get("default_provider", "openrouter")
    model = args.model or config.get("default_model", "meta-llama/llama-3.1-8b-instruct")
    max_iter = args.max_iterations or config.get("max_iterations", 30)
    timeout = args.timeout or config.get("timeout_seconds", 180)
    
    if args.temp is not None:
        os.environ["CROM_MODEL_TEMP"] = str(args.temp)
    if args.top_p is not None:
        os.environ["CROM_MODEL_TOP_P"] = str(args.top_p)
    if args.max_context_tokens is not None:
        os.environ["CROM_MAX_CONTEXT_TOKENS"] = str(args.max_context_tokens)
    
    if args.action == "compare":
        print("📊 Gerando Relatório Comparativo Composto...")
        CompositeMetrics.aggregate_runs(args.output_dir, Path(args.output_dir) / "composite_comparison_report.md")
        return
        
    print("==================================================")
    print("🚀 Iniciar Suíte de Benchmarking")
    print(f"   Benchmark:  {args.benchmark.upper()}")
    print(f"   Provedor:   {provider.upper()}")
    print(f"   Modelo:     {model}")
    print(f"   Mock Mode:  {args.mock_agent}")
    print("==================================================")
    
    # 1. Compilar executável Go se não estiver em mock
    if not args.mock_agent:
        if not build_agent():
            print("❌ Falha na compilação do agente. Abortando.")
            sys.exit(1)
            
    # 2. Inicializar adapter correspondente
    adapter = None
    if args.benchmark == "swe-bench":
        adapter = SWEBenchAdapter(provider, model, str(CONFIG_FILE))
    elif args.benchmark == "terminal-bench":
        adapter = TerminalBenchAdapter(provider, model, str(CONFIG_FILE))
    elif args.benchmark == "livecode-bench":
        adapter = LiveCodeBenchAdapter(provider, model, str(CONFIG_FILE))
    elif args.benchmark in ["evalplus", "bigcodebench"]:
        adapter = EvalPlusAdapter(provider, model, str(CONFIG_FILE))
        
    if not adapter:
        print(f"❌ Adaptador {args.benchmark} não pôde ser instanciado.")
        sys.exit(1)
        
    # 3. Carregar instâncias
    print("📥 Carregando instâncias...")
    if args.benchmark in ["evalplus", "bigcodebench"]:
        instances = adapter.load_instances(args.benchmark, limit=args.limit, mock=args.mock_agent)
    else:
        instances = adapter.load_instances(limit=args.limit, mock=args.mock_agent)
        
    print(f"✓ {len(instances)} instâncias carregadas.")
    
    # 4. Inicializar o rastreador de métricas
    tracker = MetricsTracker(args.benchmark, model, provider, str(CONFIG_FILE))
    
    # 5. Executar iteração sobre as instâncias
    for idx, inst in enumerate(instances):
        inst_id = inst.get("instance_id", inst.get("task_id", f"task-{idx+1}"))
        inst_name = inst.get("name", inst.get("repo", f"Task {idx+1}"))
        
        print(f"\n[{idx+1}/{len(instances)}] Executando {inst_id} ({inst_name})...")
        
        if args.mock_agent:
            # Simulação rápida sem chamar o binário Go ou gastar tokens
            import random
            time.sleep(1)
            success = random.choice([True, False])
            run_state = {
                "success": success,
                "turns": random.randint(3, 12),
                "tokens": random.randint(1000, 8500),
                "elapsed_seconds": round(random.uniform(2.0, 10.0), 2),
                "status": "finished" if success else "error"
            }
        else:
            # Executa de verdade chamando o adaptador
            run_state = adapter.run_instance(inst, max_iterations=max_iter, timeout=timeout)
            
        res = tracker.add_result(inst_id, inst_name, run_state)
        status_sym = "✅" if res["success"] else "❌"
        print(f"   Status: {status_sym} | Turnos: {res['turns']} | Custo Est: ${res['cost_usd']:.5f} | Tempo: {res['elapsed_seconds']:.1f}s")
        
    # 6. Salva relatórios
    print("\n==================================================")
    report_summary = tracker.generate_reports(args.output_dir)
    print("==================================================")
    
    # Exibe os resultados compostos no terminal
    deepswe = CompositeMetrics.calculate_deepswe(report_summary["success_rate"], report_summary["avg_turns"], max_iter)
    kilobench = CompositeMetrics.calculate_kilobench(
        sum(1 for r in tracker.results if r["success"]), 
        report_summary["total_cost"]
    )
    print(f"📈 Índice DeepSWE Calculado: {deepswe}")
    print(f"💲 Índice Kilo Bench (resoluções/$10 USD): {kilobench}")
    print("==================================================")

if __name__ == "__main__":
    main()
