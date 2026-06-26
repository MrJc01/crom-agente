# Suíte de Benchmarking do `crom-agente`

Esta pasta contém o ecossistema de avaliação contínua do `crom-agente` para os principais benchmarks de engenharia de software de 2026.

## 📊 Benchmarks Suportados

1.  **SWE-bench Verified & Pro**: Resolução de bugs e issues reais do GitHub em repositórios complexos.
2.  **Terminal-Bench v2.1**: Autonomia em ambiente CLI (solução de builds quebrados, depuração e scripts).
3.  **LiveCodeBench**: Desafios de programação competitiva novos (LeetCode/CodeForces) à prova de contaminação.
4.  **EvalPlus & BigCodeBench**: Validação funcional de escrita de código (versões melhoradas de HumanEval/MBPP).
5.  **Índices Compostos (Kilo Bench & DeepSWE)**: Avaliação combinada de acurácia, turnos totais e custo-benefício.

---

## 🛠️ Requisitos de Instalação

A execução de contêineres requer o serviço do **Docker** ativo na máquina.

1.  Instale as dependências de Python necessárias:
    ```bash
    pip install -r benchmark/requirements.txt
    ```
2.  Garanta que o daemon do Docker esteja rodando:
    ```bash
    sudo systemctl start docker
    ```

---

## 🚀 Como Executar

### Opção A: Executando Integrado via CLI (Recomendado)
Compile o agente em Go e chame o subcomando de benchmark:
```bash
# Compilar o binário
make build  # ou go build -tags headless -o bin/crom-agente ./cmd/crom-agente

# Executar EvalPlus para os 5 primeiros desafios usando Gemini 3.5 Flash
./bin/crom-agente benchmark run --type evalplus --limit 5 --model gemini-3.5-flash
```

### Opção B: Executando via Script Python Direto
Você também pode rodar o script controlador diretamente:
```bash
python3 benchmark/main.py run --benchmark swe-bench --limit 3 --provider gemini --model gemini-3.5-flash
```

---

## 📈 Relatórios de Desempenho
Cada execução gera relatórios automáticos salvos na pasta `benchmark/reports/`:
*   `reports/benchmark_summary_<model>_<date>.json`: Histórico bruto da execução.
*   `reports/benchmark_report_<model>_<date>.md`: Relatório formatado em tabela Markdown com métricas de pass@1, tokens consumidos, custos estimados e tempo médio.
