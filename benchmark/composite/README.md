# Índices Compostos e Multiagentes (O Estado da Arte)

Estes índices medem o desempenho geral do agente ponderando sua acurácia (taxa de acerto) contra métricas operacionais de custo (preço de API) e esforço (tempo decorrido e turnos ReAct gastos).

## 📊 Métricas Ponderadas

### 1. DeepSWE (Foco em Profundidade de Engenharia)
*   **Conceito**: Mede a eficiência de resolução de problemas profundos de repositório.
*   **Fórmula Local**:
    $$DeepSWE = (TaxaDeSucesso \times 0.70) + \left(1.0 - \frac{TurnosMedios}{TurnosMaximos}\right) \times 30.0$$
*   Proporciona pontuação máxima (100) caso o agente tenha 100% de sucesso e use o mínimo de turnos ReAct. Liderado no mercado por **DeepSeek V4 Pro** e **GLM 5.2**.

### 2. Kilo Bench (Foco em Custo-Benefício)
*   **Conceito**: Não avalia apenas o índice absoluto de acerto, mas o custo financeiro gasto em tokens para atingir tal resolução.
*   **Fórmula Local**:
    $$KiloBench = \left(\frac{TarefasResolvidasComSucesso}{CustoTotalUsd}\right) \times 10.0$$
*   Mede quantas tarefas são resolvidas a cada $10.00 USD de gastos de API. Útil para comparar modelos menores de código aberto (Laguna M.1, Qwen Coder) contra proprietários de custo alto.

---

## 🛠️ Funcionamento do Adaptador Local
O agregador composto em [composite.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/composite.py):
1.  Lê os arquivos JSON brutos com a telemetria das execuções armazenados em `benchmark/reports/`.
2.  Extrai a quantidade de turnos e calcula o custo com base no preço por milhão de tokens de input/output mapeado no [config.json](file:///home/j/Documentos/GitHub/crom-agente/benchmark/config.json).
3.  Imprime e exporta um relatório agregado markdown consolidando todos os modelos testados lado a lado.
