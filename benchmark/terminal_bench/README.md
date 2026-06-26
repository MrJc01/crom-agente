# Terminal-Bench (Foco em Autonomia e CLI)

Criado para testar puramente a capacidade de agentes de usarem o terminal do sistema operacional (bash) de forma autônoma. O agente recebe uma tarefa vaga (ex: "consertar o build quebrado") e deve inspecionar logs de erro, depurar, rodar scripts e corrigir o código e dependências.

## 🔗 Fontes Originais
*   **Leaderboard / Artificial Analysis**: [artificialanalysis.ai](https://artificialanalysis.ai/)
*   **Morph-Labs / MorphLLM**: [github.com/Morph-Labs/Terminal-Bench](https://github.com/Morph-Labs/Terminal-Bench)
*   **Hugging Face Datasets**:
    *   Terminal-Bench: [artificialanalysis/terminal-bench](https://huggingface.co/datasets/artificialanalysis/terminal-bench)

## 🏆 Liderança do Mercado (2026)
*   **Codex CLI** (+ GPT-5.5 liderando com ~83.4%)
*   **Claude Code** (+ Claude Fable 5)
*   **Terminus 2**
*   **Gemini CLI** (Gemini 3.1 Pro)

## 🛠️ Funcionamento do Adaptador Local
O adaptador em [terminal_bench.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/terminal_bench.py):
1.  Sobe um container Docker isolado (ex: Ubuntu, Node ou Go) e roda uma rotina que gera um ambiente quebrado deliberadamente.
2.  Descreve os sintomas para o agente de forma CLI vaga.
3.  Permite o agente atuar de forma livre com a ferramenta `terminal_command`.
4.  Roda o comando de compilação ou teste no final para validar se o ambiente foi completamente restaurado e está funcional.
