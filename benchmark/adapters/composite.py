import json
from pathlib import Path
from tabulate import tabulate

class CompositeMetrics:
    @staticmethod
    def calculate_deepswe(success_rate, avg_turns, max_iterations=30):
        """
        Calcula o índice DeepSWE (0 a 100).
        Foca na taxa de sucesso penalizando excesso de turnos (baixa eficiência).
        """
        if success_rate == 0:
            return 0.0
        # Penalidade por turnos: se gastar o máximo de iterações, a eficiência cai
        efficiency = 1.0 - (avg_turns / max_iterations)
        # DeepSWE = 70% acurácia + 30% eficiência de turnos
        score = (success_rate * 0.70) + (efficiency * 30.0)
        return round(score, 2)

    @staticmethod
    def calculate_kilobench(successful_runs, total_cost_usd):
        """
        Calcula o índice Kilo Bench.
        Mede a quantidade de tarefas resolvidas a cada $10.00 USD gastos em tokens.
        """
        if total_cost_usd == 0:
            return 0.0
        # Resolvidos por $10 USD
        score = (successful_runs / total_cost_usd) * 10.0
        return round(score, 2)

    @classmethod
    def aggregate_runs(cls, reports_dir, output_file=None):
        """
        Lê todos os relatórios JSON gerados na pasta de relatórios
        e monta uma tabela comparativa geral calculando os índices compostos.
        """
        reports_path = Path(reports_dir)
        if not reports_path.exists():
            return []
            
        summaries = []
        # Localiza arquivos JSON de resumos
        for file in reports_path.glob("*.json"):
            try:
                with open(file) as f:
                    results = json.load(f)
                    
                if not results:
                    continue
                    
                # Extrai dados do nome do arquivo ou do próprio resultado
                # Espera formato: benchmark_summary_<benchmark>_<model>_<date>.json
                parts = file.stem.split("_")
                benchmark = parts[2] if len(parts) > 2 else "unknown"
                model = parts[3] if len(parts) > 3 else "unknown"
                
                total_runs = len(results)
                successful_runs = sum(1 for r in results if r["success"])
                success_rate = (successful_runs / total_runs * 100.0) if total_runs > 0 else 0.0
                
                total_cost = sum(r["cost_usd"] for r in results)
                total_turns = sum(r["turns"] for r in results)
                avg_turns = (total_turns / total_runs) if total_runs > 0 else 0.0
                avg_time = (sum(r["elapsed_seconds"] for r in results) / total_runs) if total_runs > 0 else 0.0
                
                deepswe = cls.calculate_deepswe(success_rate, avg_turns)
                kilobench = cls.calculate_kilobench(successful_runs, total_cost)
                
                summaries.append({
                    "benchmark": benchmark,
                    "model": model,
                    "success_rate": f"{success_rate:.1f}%",
                    "avg_turns": f"{avg_turns:.1f}",
                    "avg_time": f"{avg_time:.1f}s",
                    "total_cost": f"${total_cost:.4f}",
                    "deepswe": deepswe,
                    "kilobench": kilobench
                })
            except Exception as e:
                print(f"Erro ao analisar arquivo {file}: {e}")
                
        # Imprime/salva tabela Markdown
        if summaries:
            headers = ["Benchmark", "Modelo", "Sucesso", "Turnos Médios", "Tempo Médio", "Custo Total", "DeepSWE Index", "Kilo Bench"]
            rows = [[s["benchmark"], s["model"], s["success_rate"], s["avg_turns"], s["avg_time"], s["total_cost"], s["deepswe"], s["kilobench"]] for s in summaries]
            
            report_md = ["# Relatório Comparativo Geral (Índices Compostos)"]
            report_md.append("\n" + tabulate(rows, headers=headers, tablefmt="github"))
            
            if output_file:
                with open(output_file, "w") as f:
                    f.write("\n".join(report_md))
                print(f"✓ Relatório comparativo composto salvo em: {output_file}")
                
            return summaries
        return []
