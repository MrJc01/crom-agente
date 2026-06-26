# Pesquisa 0: Análise de Estabilidade do Loop Agentic e Infraestrutura de Benchmark

## Resumo Executivo
Neste documento analisamos os trabalhos recentes focados no aumento da robustez da infraestrutura de benchmarking e na melhoria da estabilidade do loop de execução do agente (Agentic Loop) no projeto Crom Agente. As modificações abordaram problemas críticos de vazamento de contexto (loops infinitos), restrições de infraestrutura (falhas de permissão e quotas) e otimização de custos durante execuções prolongadas com o modelo Llama 3.1 8B.

## 1. Problemas Identificados e Trabalhos Realizados

### 1.1 Infraestrutura e Hardening do Benchmark
Durante a execução em larga escala de testes do benchmark, foram identificados gargalos que impediam o sucesso da avaliação:
- **Falhas de Permissão no Docker**: Os artefatos gerados dentro do container de sandbox (no `/workspace`) pertenciam ao usuário `root`, resultando em erros de "Operation not permitted" quando o host tentava limpar ou acessar os arquivos entre as sessões.
  - *Solução*: Implementamos uma rotina de limpeza atômica executada diretamente no contexto do Docker (utilizando `find /workspace -mindepth 1 -delete`), garantindo que o estado fosse zerado com os privilégios apropriados antes de destruir o ambiente.
- **Restrições de Espaço em Disco**: Tarefas mais complexas que exigiam clonagem de repositórios inteiros falhavam por atingir as cotas padrão.
  - *Solução*: Incluímos suporte à variável e _flag_ `CROM_DISABLE_DISK_QUOTA`, permitindo escalar os recursos do contêiner sob demanda do desenvolvedor ou benchmark.
- **Resolução de Caminhos Inconsistente nas Ferramentas**: As ferramentas acionadas pelo agente geravam e patcheavam arquivos usando caminhos relativos de forma desordenada.
  - *Solução*: Padronizamos a navegação e manipulação de arquivos para estarem sempre ancorados ao `root` do workspace.

### 1.2 Estabilidade do Loop e Comportamento do Agente
O agente caía frequentemente em "loops infinitos de frustração", repetindo cegamente a chamada das mesmas ferramentas ou não progredindo em seu raciocínio.
- **Intervenções Explícitas no Loop (`loop_detector.go`)**: Ao invés de apenas sinalizar um erro passivo, o detector de loops agora atua ativamente, injetando uma "mensagem de intervenção de sistema" no contexto (memória) para forçar o modelo a quebrar o ciclo e tentar uma abordagem nova.
- **Circuit Breaker Baseado em Custos (`execute.go`)**: Introduzimos um limite dinâmico pautado no custo financeiro. O orquestrador monitora o `CustoTotalUSD` (mantido no `StateManager`) e, ao identificar risco de extrapolar o orçamento da tarefa, injeta um "ALERTA DE CUSTO". Isso orienta o agente a concluir o que tem rapidamente e abortar execuções predatórias de maneira autônoma.

## 2. O Que Descobrimos e Aprendemos

A execução massiva de todos os benchmarks, utilizando 20 execuções paralelas por cenário com o modelo open-source **Llama 3.1 8B** (via OpenRouter), trouxe ensinamentos importantíssimos:

### 2.1 Limitações e Comportamento de Modelos 8B
- **Degradação de Foco (Loss of Intent)**: Modelos menores como o de 8 bilhões de parâmetros não conseguem manter persistência lógica ao longo de trajetórias muito longas. A partir do **5º ao 6º turno**, o agente se esquece do objetivo maior da tarefa e passa a focar excessivamente no micro-erro imediato devolvido pelo terminal.
- **Efeito Rebote da Intervenção**: As injeções do detector de loop funcionam muito bem para interromper a ação repetida de uma ferramenta. No entanto, o agente muitas vezes **pivota para novos erros de raciocínio** em vez de formular uma solução correta. Ele entende o "Pare de fazer isso", mas, devido ao esquecimento parcial, não consegue derivar o "Então devo fazer *aquilo*".

### 2.2 Sincronia de Arquitetura de Testes
- Sandboxes fortemente isolados como Docker exigem alinhamento restrito do ciclo de vida da GID/UID, caso contrário, o teste relata falha por falha na infraestrutura de avaliação, ofuscando o real desempenho e os acertos de código gerados pelo modelo.

## 3. Recomendações e Próximos Passos
1. **Memória Resumida (Summarization Checkpoints)**: Para suportar modelos da classe 8B/7B de maneira autônoma, precisamos injetar blocos de "lembrete de objetivo" (summarizer) a cada 3 ou 4 turnos no `execute.go`, ancorando o raciocínio e impedindo a "deriva cognitiva" que vimos na avaliação.
2. **Métricas Híbridas de Quebra (Loop Breaks)**: O circuit breaker não deve avaliar só custo financeiro, mas também a relação de _ações realizadas_ versus _modificações de código úteis_, matando execuções infrutíferas ainda mais cedo.

---
*Documento gerado com base nos logs, relatórios comparativos e testes executados.*
