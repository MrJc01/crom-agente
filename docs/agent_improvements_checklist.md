# Checklist de Melhorias do Crom-Agente (50+ Itens)

Este documento reúne **54 melhorias e otimizações** planejadas para o `crom-agente` com o objetivo de aumentar a taxa de resolução (Pass@1) em benchmarks complexos (como SWE-bench, Terminal-Bench e EvalPlus) e reduzir o custo operacional (tokens por tarefa).

---

## 🧠 1. Cognitive Loop & ReAct Logic (1-15)

- [ ] **1.** Implementar fallback de modo text-only no motor do agente para extrair chamadas de ferramenta estruturadas via regex/JSON simplificado quando o LLM retornar payloads inválidos.
- [ ] **2.** Aumentar a janela histórica de detecção em `DetectRepetitiveLoop` de 2 para 4 turnos do assistant para detectar padrões de oscilação entre duas abordagens erradas.
- [ ] **3.** Ajustar dinamicamente a temperatura ou parâmetros de amostragem do LLM (se suportado pelo provider) quando `DetectRepetitiveLoop` for acionado para forçar caminhos de raciocínio alternativos.
- [ ] **4.** Introduzir limite de turnos dinâmico ajustado pela complexidade estimada da tarefa (ex: 30 turnos para SWE-bench, 10 para EvalPlus).
- [ ] **5.** Detectar e mitigar respostas vazias consecutivas do LLM enviando uma correção no canal de sistema sem incrementar o limite físico de iterações.
- [ ] **6.** Interromper a execução imediatamente se o LLM declarar que terminou a tarefa em texto mas o loop principal não executou a etapa de encerramento (`finalizer`).
- [ ] **7.** Permitir bypass do agente `finalizer` caso a verificação automatizada de testes do sistema de destino retorne sucesso absoluto (exit code 0), economizando tokens da última iteração.
- [ ] **8.** Implementar backoff exponencial com jitter em caso de rate-limitings temporários (HTTP 429) do provedor de LLM.
- [ ] **9.** Rastrear arquivos modificados e seus hashes MD5/SHA256 turno a turno para detectar loops de alteração redundante (gravação do mesmo conteúdo seguidamente).
- [ ] **10.** Emitir avisos preventivos para o usuário/logs de telemetria quando o agente consumir mais de 80% do limite máximo de iterações configurado.
- [ ] **11.** Suportar paralelização de execução de ferramentas quando as ações não possuírem interdependência direta de escrita sobre os mesmos arquivos.
- [ ] **12.** Adicionar mecanismo de autorrecuperação se o plano estruturado (`state.GetPlan()`) for corrompido, recriando-o com base no histórico recente.
- [ ] **13.** Otimizar as transições de ModoCognitivo (`Planning`, `Executing`, `Debugging`, `Verifying`, `Interacting`) associando diretrizes comportamentais específicas do sistema a cada uma.
- [ ] **14.** Implementar validação lógica para detectar se o assistente está tentando re-ler repetidamente o mesmo arquivo sem que tenha havido qualquer alteração no workspace.
- [ ] **15.** Suportar um comando `/abort` em tempo real para permitir cancelamento imediato e seguro do loop ReAct pelo usuário.

---

## 🔍 2. Error Reflection & Auto-Linting (16-30)

- [ ] **16.** Expandir validação de Python em `file_validator.go` executando parsing AST via subprocess (`python -c "import ast; ast.parse(...)"`) para extrair linha e causa exata de erros de sintaxe (como `SyntaxError`, `IndentationError`).
- [ ] **17.** Adicionar validação de arquivos JSON usando `json.Unmarshal` estruturado para detectar erros de formatação (vírgulas extras, chaves órfãs) com feedback detalhado de offset.
- [ ] **18.** Implementar linter básico para arquivos JavaScript/TypeScript rodando `node -c` se o Node.js estiver presente no path do host.
- [ ] **19.** Adicionar validação Go nativa que execute `go fmt` no arquivo gravado e alerte o agente se houverem discrepâncias graves de formatação.
- [ ] **20.** Implementar validador de sintaxe YAML/TOML para arquivos de configuração para assegurar que modificações de setup não quebrem o ambiente.
- [ ] **21.** Se a validação de sintaxe falhar, injetar a mensagem de erro formatada incluindo o snippet de código ao redor da linha problemática no histórico do agente.
- [ ] **22.** Diferenciar saídas de stdout e stderr de ferramentas que falharem no terminal, fornecendo mensagens estilizadas que destaquem as stack traces.
- [ ] **23.** Scanear saídas de frameworks de teste padrão (como `pytest`, `unittest`, `go test`) para extrair trechos de assert falhos e formatá-los para o LLM.
- [ ] **24.** Validar dependências/imports nos scripts modificados do workspace e alertar o agente caso ele importe módulos inexistentes na stack técnica local.
- [ ] **25.** Implementar checagem estática para shell scripts criados ou modificados pelo agente (usando `shellcheck` se disponível no host).
- [ ] **26.** Rejeitar chamadas a ferramentas cujos argumentos obrigatórios possuam tipos incorretos no JSON enviado, devolvendo uma resposta de erro estruturada antes de rodar o comando.
- [ ] **27.** Garantir que a ferramenta `diff_replace` verifique a existência do arquivo alvo antes de tentar aplicar modificações, reportando erro técnico imediato se inexistente.
- [ ] **28.** Auto-formatar arquivos Go modificados rodando `go fmt` ou `goimports` antes de submeter ao validador sintático final.
- [ ] **29.** Criar uma área de rollback/backup (`.crom/backups/`) que preserve a última versão estável de um arquivo antes que o agente o sobrescreva.
- [ ] **30.** Estabelecer limite de 3 tentativas consecutivas de correção de lint para um único arquivo antes de forçar o rollback automático para a versão do backup.

---

## 📉 3. Context Compaction & Token Conservation (31-40)

- [ ] **31.** Refatorar `CompactMessages` em `compactor.go` para proteger as instruções de sistema originais, regras locais, stack técnica e o prompt de ferramentas de serem resumidos.
- [ ] **32.** Resumir saídas de terminal extensas em mensagens de ferramenta mantendo apenas as primeiras 20 e as últimas 30 linhas do log, indicando o volume de conteúdo suprimido no meio.
- [ ] **33.** Assegurar que a primeira intenção do usuário (`messages[0]` ou a otimizada pelo prompt manager) nunca seja incluída no pool de sumarização do compactor.
- [ ] **34.** Substituir a contagem simples de mensagens como gatilho de compactação por uma análise baseada em tokens (compactar quando a conversação exceder 75% da janela de contexto).
- [ ] **35.** Filtrar e agrupar mensagens consecutivas de erro repetido de ferramenta no histórico compacto para economizar espaço de prompt.
- [ ] **36.** Permitir que o compactor dropa saídas de ferramentas que foram totalmente superadas por execuções mais recentes com sucesso (ex: ler o mesmo arquivo várias vezes).
- [ ] **37.** Remover o conteúdo textual de mensagens `read_file` muito antigas e substituí-lo por uma marcação simbólica (`[FILE READ: path/to/file - size: X bytes]`) no histórico compactado.
- [ ] **38.** Compactar blocos de diff aplicados com sucesso em turnos anteriores, mantendo apenas a referência de que o arquivo foi modificado.
- [ ] **39.** Eliminar logs de depuração gerados pelas próprias chamadas de API do LLM que tenham sido persistidos de forma imprópria na lista de mensagens.
- [ ] **40.** Aplicar um limite de tokens de resposta (`MaxCompletionTokens`) otimizado dinamicamente por turno para evitar prolixidade em explicações textuais longas.

---

## 🔒 4. Tool Execution Safety & Semantics (41-54)

- [ ] **41.** Impedir modificações diretas ou gravações fora da pasta raiz do workspace ativa (isolamento absoluto), bloqueando caminhos relativos maliciosos (como `../../`).
- [ ] **42.** Criar lista negra de diretórios protegidos (como `.git/`, `.crom/`, `/etc/`, `/var/`) que não podem ser alterados pelo agente em hipótese alguma.
- [ ] **43.** Detectar comandos shell interativos que bloqueiam o console (ex: `python`, `top`, servidores web em foreground) e abortá-los após curto timeout, ensinando o agente a usar flags de background ou subagentes adequados.
- [ ] **44.** Limitar o tamanho do retorno de `read_file` em arquivos massivos (>250KB), retornando apenas uma amostragem do cabeçalho e rodapé com instruções de paginação.
- [ ] **45.** Definir limites estritos de tempo de execução (`timeout`) individuais para comandos de terminal executados via `terminal_command` (default: 30s).
- [ ] **46.** Validar o estado do repositório Git antes de permitir que o agente execute `git commit`, prevenindo erros de "nothing to commit".
- [ ] **47.** Mover a lógica heurística de extração de blocos markdown de código (modo text-only) para dentro do motor nativo Go de parsing, unificando a leitura de tool calls.
- [ ] **48.** Garantir que a ferramenta `diff_replace` confirme se o bloco `TargetContent` é único no arquivo antes de realizar a substituição para evitar alterações múltiplas indesejadas.
- [ ] **49.** Aprimorar as mensagens de erro do `diff_replace` exibindo uma análise de similaridade se a correspondência falhar por poucos caracteres.
- [ ] **50.** Bloquear execução de comandos destrutivos explícitos (como `rm -rf /`, `mkfs`, `dd`) na ferramenta de terminal.
- [ ] **51.** Limpar de forma assíncrona arquivos temporários, contêineres Docker órfãos e endpoints simulados após a conclusão de uma sessão de execução.
- [ ] **52.** Armazenar em cache o conteúdo lido de arquivos do workspace na iteração atual para evitar múltiplas chamadas físicas de leitura redundantes no mesmo turno.
- [ ] **53.** Monitorar e manter o controle acumulado de custo financeiro da sessão ativa, abortando o loop se o teto de gastos estipulado (ex: $2.00) for ultrapassado.
- [ ] **54.** Permitir que o agente defina opcionalmente seu "nível de confiança" (confidence score) nas mensagens internas para sinalizar gargalos e incertezas em tempo de execução.
