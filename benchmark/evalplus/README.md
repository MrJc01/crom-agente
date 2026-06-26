# EvalPlus (O Teste Unitário Elevado ao Máximo)

EvalPlus estende o clássico benchmark **HumanEval** adicionando mais de 80x mais casos de teste gerados automaticamente via fuzzing para cada problema. Isso garante que o código escrito não seja apenas um "falso positivo" que passa em testes triviais mas quebra em casos extremos.

## 🔗 Fontes Originais
*   **Website Oficial**: [evalplus.github.io](https://evalplus.github.io/)
*   **Repositório GitHub**: [github.com/evalplus/evalplus](https://github.com/evalplus/evalplus)
*   **Hugging Face Datasets**:
    *   HumanEval+: [evalplus/humanevalplus](https://huggingface.co/datasets/evalplus/humanevalplus)
    *   MBPP+: [evalplus/mbppplus](https://huggingface.co/datasets/evalplus/mbppplus)

## 🏆 Liderança do Mercado (2026)
*   **OpenAI o1**
*   **Qwen2.5-Coder-32B**
*   **DeepSeek-V3**
*(Historicamente domina as LLMs de geração direta de código).*

## 🛠️ Funcionamento do Adaptador Local
O adaptador em [evalplus.py](file:///home/j/Documentos/GitHub/crom-agente/benchmark/adapters/evalplus.py):
1.  Pega o protótipo da assinatura da função e seu respectivo docstring.
2.  Instrui o agente a preencher a lógica da função.
3.  Valida se a função está completa contra os casos de testes estendidos importando e rodando a função localmente.
