# BigCodeBench (Uso de APIs Complexas e Bibliotecas)

Avalia a capacidade dos modelos de codificação de utilizar bibliotecas reais do ecossistema do Python (ex: pandas, requests, matplotlib, scikit-learn). Mede a habilidade de resolver tarefas reais com dependências de terceiros de forma coerente.

## 🔗 Fontes Originais
*   **Website Oficial**: [bigcode-bench.github.io](https://bigcode-bench.github.io/)
*   **Repositório GitHub**: [github.com/bigcode-project/bigcodebench](https://github.com/bigcode-project/bigcodebench)
*   **Hugging Face Datasets**:
    *   BigCodeBench: [bigcode-bench/BigCodeBench](https://huggingface.co/datasets/bigcode-bench/BigCodeBench)

## 🏆 Liderança do Mercado (2026)
*   **Claude 3.7 Sonnet**
*   **OpenAI o1** & **o3-mini**
*   **DeepSeek-R1**
*   **Quasar-Alpha**, **Grok-3**

## 🛠️ Funcionamento do Adaptador Local
O adaptador em [evalplus.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/evalplus.py) (que unifica a entrada de EvalPlus/BigCodeBench):
1.  Busca as especificações de desafios complexos que usam APIs do ecossistema Python.
2.  Dispara o agente informando o template da função.
3.  Avalia os assertions contidos na suíte do BigCodeBench contra o arquivo de solução gerado.
