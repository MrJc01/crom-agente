# SWE-bench (O Padrão Ouro de Engenharia de Software)

SWE-bench avalia a capacidade de agentes autônomos de resolver problemas reais de engenharia de software extraídos de repositórios reais do GitHub (ex: Django, SymPy, Scikit-learn, Sphinx).

## 🔗 Fontes Originais
*   **Website Oficial**: [swe-bench.com](https://www.swe-bench.com/)
*   **SWE-rebench**: [swe-rebench.com](https://swe-rebench.com)
*   **Repositório GitHub**: [github.com/princeton-nlp/SWE-bench](https://github.com/princeton-nlp/SWE-bench)
*   **Hugging Face Datasets**:
    *   SWE-bench Lite: [swe-bench/SWE-bench_Lite](https://huggingface.co/datasets/swe-bench/SWE-bench_Lite)
    *   SWE-bench Verified: [swe-bench/SWE-bench_Verified](https://huggingface.co/datasets/swe-bench/SWE-bench_Verified)
    *   SWE-bench Pro (bases mais densas contra contaminação): [swe-bench/SWE-bench_Pro](https://huggingface.co/datasets/swe-bench/SWE-bench_Pro)

## 🏆 Liderança do Mercado (2026)
*   **SWE-bench Pro & Verified**:
    *   **Claude Code** (com Claude Fable 5 & Opus 4.8)
    *   **OpenAI Codex** (com GPT-5.5)
    *   **Cursor** (Composer 2.5)
    *   **GLM 5.2**, **Qwen 3.7 Max**, **Gemini 3.5 Flash**
*   **SWE-rebench**:
    *   GPT-5.5, Opus 4.7, Kimi K2.6, Kimi K2.7 Code

## 🛠️ Funcionamento do Adaptador Local
O adaptador em [swe_bench.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/swe_bench.py):
1.  Faz o download dos metadados da issue/tarefa do Hugging Face.
2.  Inicializa um container Docker isolado para clonar o repositório na versão do commit base correto.
3.  Transfere e roda o binário do `crom-agente` dentro do workspace do container.
4.  Aplica o teste de validação oficial (`test_patch`) e executa o framework de testes (ex: `pytest` ou `django test`) para verificar se o bug foi resolvido.
