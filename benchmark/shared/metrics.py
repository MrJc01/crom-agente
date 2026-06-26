import json
import time
from pathlib import Path
from tabulate import tabulate

class MetricsTracker:
    def __init__(self, benchmark_name, model, provider, config_path=None):
        self.benchmark_name = benchmark_name
        self.model = model
        self.provider = provider
        self.results = []
        self.prices = {
            "input": 0.05,
            "output": 0.15
        }
        
        # Carrega preços de tokens se config_path for provido
        if config_path:
            try:
                with open(config_path) as f:
                    config = json.load(f)
                    prices_map = config.get("token_prices_per_million", {})
                    # Procura correspondência parcial no nome do modelo
                    model_lower = model.lower()
                    matched = False
                    for key, val in prices_map.items():
                        if key.lower() in model_lower or model_lower in key.lower():
                            self.prices = val
                            matched = True
                            break
                    if not matched:
                        # Fallback padrão baseado em HSL/8b ou 3b
                        if "8b" in model_lower or "7b" in model_lower or "flash" in model_lower:
                            self.prices = {"input": 0.075, "output": 0.3}
                        else:
                            self.prices = {"input": 2.0, "output": 8.0}
            except Exception:
                pass

    def add_result(self, task_id, task_name, run_state):
        """
        Adiciona o resultado de uma tarefa executada.
        """
        tokens = run_state.get("tokens", 0)
        
        # Como o crom-agente expõe apenas o total de tokens gastos na struct do estado,
        # estimamos uma divisão comum de 85% input / 15% output para precificação detalhada.
        input_est = int(tokens * 0.85)
        output_est = tokens - input_est
        
        cost_est = (input_est / 1_000_000.0 * self.prices.get("input", 0.0)) + \
                   (output_est / 1_000_000.0 * self.prices.get("output", 0.0))
                   
        res = {
            "task_id": task_id,
            "task_name": task_name,
            "success": run_state.get("success", False),
            "turns": run_state.get("turns", 0),
            "tokens": tokens,
            "cost_usd": cost_est,
            "elapsed_seconds": run_state.get("elapsed_seconds", 0.0),
            "status": run_state.get("status", "unknown"),
            "files_created": run_state.get("files_created", 0),
            "tool_calls": run_state.get("tool_calls", 0),
        }
        self.results.append(res)
        return res

    def generate_reports(self, output_dir):
        """
        Gera relatórios JSON e Markdown consolidando o desempenho do modelo.
        """
        out_path = Path(output_dir)
        out_path.mkdir(parents=True, exist_ok=True)
        
        date_str = time.strftime("%Y%m%d_%H%M%S")
        model_safe = self.model.replace("/", "_").replace(":", "_")
        
        json_file = out_path / f"benchmark_summary_{self.benchmark_name}_{model_safe}_{date_str}.json"
        md_file = out_path / f"benchmark_report_{self.benchmark_name}_{model_safe}_{date_str}.md"
        
        # 1. Salva o JSON bruto
        with open(json_file, "w") as f:
            json.dump(self.results, f, indent=2)
            
        # 2. Calcula agregados
        total_runs = len(self.results)
        successful_runs = sum(1 for r in self.results if r["success"])
        success_rate = (successful_runs / total_runs * 100.0) if total_runs > 0 else 0.0
        
        total_tokens = sum(r["tokens"] for r in self.results)
        total_cost = sum(r["cost_usd"] for r in self.results)
        total_time = sum(r["elapsed_seconds"] for r in self.results)
        total_turns = sum(r["turns"] for r in self.results)
        
        avg_time = (total_time / total_runs) if total_runs > 0 else 0.0
        avg_turns = (total_turns / total_runs) if total_runs > 0 else 0.0
        
        # 3. Formata tabela Markdown
        table_data = []
        for r in self.results:
            succ_emoji = "✅" if r["success"] else "❌"
            table_data.append([
                r["task_id"],
                r["task_name"],
                f"{r['elapsed_seconds']:.1f}s",
                r["turns"],
                r["tokens"],
                f"${r['cost_usd']:.6f}",
                succ_emoji
            ])
            
        headers = ["ID", "Nome da Tarefa", "Tempo", "Turnos", "Tokens", "Custo (USD)", "Resultado"]
        
        md_content = []
        md_content.append(f"# Relatório de Benchmark: {self.benchmark_name.upper()}")
        md_content.append(f"\n*   **Modelo:** `{self.model}`")
        md_content.append(f"*   **Provedor:** `{self.provider}`")
        md_content.append(f"*   **Data de Execução:** {time.strftime('%Y-%m-%d %H:%M:%S')}")
        md_content.append("\n## Métricas Consolidadas")
        md_content.append(f"*   **Sucesso Geral:** {successful_runs}/{total_runs} ({success_rate:.1f}%)")
        md_content.append(f"*   **Total de Tokens:** {total_tokens}")
        md_content.append(f"*   **Custo Total Estimado:** ${total_cost:.6f} USD")
        md_content.append(f"*   **Tempo Médio por Tarefa:** {avg_time:.2f}s (Total: {total_time:.1f}s)")
        md_content.append(f"*   **Média de Turnos ReAct:** {avg_turns:.1f}")
        md_content.append("\n## Resultados por Tarefa")
        md_content.append(tabulate(table_data, headers=headers, tablefmt="github"))
        
        with open(md_file, "w") as f:
            f.write("\n".join(md_content))
            
        print(f"✓ Relatório JSON salvo em: {json_file}")
        print(f"✓ Relatório Markdown salvo em: {md_file}")
        
        return {
            "success_rate": success_rate,
            "total_cost": total_cost,
            "total_tokens": total_tokens,
            "avg_time": avg_time,
            "avg_turns": avg_turns,
            "json_path": str(json_file),
            "md_path": str(md_file)
        }
