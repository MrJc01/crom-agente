# LiveCodeBench & LiveBench (Foco Anti-Contaminação)

Mede o raciocínio lógico e predição de código usando desafios recém-criados de programação competitiva (LeetCode, CodeForces, AtCoder). Ao usar dados em tempo real pós-treinamento dos modelos, previne que IAs simplesmente memorizem respostas pré-existentes.

## 🔗 Fontes Originais
*   **Website Oficial LiveBench**: [livebench.ai](https://livebench.ai/)
*   **Website Oficial LiveCodeBench**: [livecodebench.github.io](https://livecodebench.github.io/)
*   **Repositório GitHub**: [github.com/LiveCodeBench/LiveCodeBench](https://github.com/LiveCodeBench/LiveCodeBench)
*   **Hugging Face Datasets**:
    *   LiveCodeBench (code generation): [livecodebench/code_generation](https://huggingface.co/datasets/livecodebench/code_generation)

## 🏆 Liderança do Mercado (2026)
*   **Gemini 3 Pro Preview** & **Gemini 3 Flash Preview**
*   **DeepSeek V3.2 Speciale**
*   Modelos de Raciocínio Puro (como OpenAI **o1** e **o3-mini**)

## 🛠️ Funcionamento do Adaptador Local
O adaptador em [livecode_bench.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/livecode_bench.py):
1.  Obtém os problemas de codificação estruturados contendo inputs e outputs esperados.
2.  Descreve a especificação do desafio e instrui o agente a criar o arquivo de solução.
3.  Executa testes locais de caixa preta com as entradas e confere a saída em tempo de execução via subprocesso do Python.
