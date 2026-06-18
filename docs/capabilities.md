# Catálogo de 40 Capacidades do crom-agente & crom-agente-sdk

Este documento mapeia as 40 capacidades essenciais do orquestrador de agentes autônomos, detalhando como cada uma é implementada na nossa arquitetura Go (CLI crom-agente e SDK crom-agente-sdk) e o nível de segurança / aprovação do usuário (HITL - Human-in-the-Loop) exigido.

> [!NOTE]
> **Status de Implementação**: A coluna "Status" indica se a capacidade já possui implementação funcional no código-fonte. ✅ = Implementada, ❌ = Planejada (pendente de desenvolvimento).

> [!IMPORTANT]
> **Atualmente, 20 de 40 capacidades estão implementadas**: `read_file`, `write_file`, `diff_replace`, `rename_file`, `delete_file`, `tree`, `grep`, `port_monitor`, regras locais, stack detection, error log parser, planejamento ReAct, auto-validação (`go vet`/`go fmt`), `terminal_command`, `spawn_subagent`, prevenção de loops, roteamento HITL, sandboxing, timeouts e redação de segredos. As demais 20 estão planejadas e documentadas, mas ainda não existem como código funcional.

---

## 🛠️ Tabela de Capacidades do Agente em Go

| # | Status | Categoria | Capacidade / Ação | Implementação Go (Por Trás dos Panos) | Nível de Aprovação (HITL) |
|---|---|---|---|---|---|
| **1** | ✅ | Manipulação de Arquivos | Ler arquivos completos | `os.ReadFile` em `internal/tools/read_file.go`, limitando o buffer a 2MB. | Livre (Sem aprovação) |
| **2** | ✅ | Manipulação de Arquivos | Escrever novos arquivos do zero | `os.OpenFile` com flags `O_CREATE\|O_WRONLY\|O_TRUNC` em `internal/tools/write_file.go`, criando pastas pais com `os.MkdirAll`. | **Requer Aprovação (Alta)** |
| **3** | ✅ | Manipulação de Arquivos | Injetar deltas (Diffs/Replace) | Algoritmo Go de busca e substituição de blocos (Search/Replace) para evitar regenerar arquivos inteiros. | **Requer Aprovação (Alta)** |
| **4** | ✅ | Manipulação de Arquivos | Renomear e mover arquivos | `os.Rename` seguido de varredura das dependências de imports no projeto. | **Requer Aprovação (Média)** |
| **5** | ✅ | Manipulação de Arquivos | Excluir arquivos e pastas | `os.RemoveAll` com bloqueio explícito a pastas do sistema e repositório (`.git`, `.github`). | **Requer Aprovação (Crítica)** |
| **6** | ✅ | Análise de Contexto | Mapear árvore de diretórios (Tree) | `path/filepath.WalkDir` com profundidade limitada e filtragem de arquivos ignorados (`node_modules`, `build`). | Livre (Sem aprovação) |
| **7** | ✅ | Análise de Contexto | Busca semântica e estrutural (Grep) | Busca nativa recursiva usando o pacote `regexp` com fallback a execuções otimizadas do `ripgrep`. | Livre (Sem aprovação) |
| **8** | ❌ | Análise de Contexto | Sincronização de buffer ativo | Canal de comunicação bidirecional recebendo alterações em tempo real de arquivos abertos. | Livre (Sem aprovação) |
| **9** | ✅ | Análise de Contexto | Carregar regras locais | Leitura automática de `.cromrules` e `.voidrules` na raiz e injeção como mensagens de sistema no loop. | Livre (Sem aprovação) |
| **10** | ✅ | Análise de Contexto | Identificação de Stack técnica | Varredura estática de assinaturas de arquivos (`go.mod`, `package.json`, etc.) orientando o prompt. | Livre (Sem aprovação) |
| **11** | ✅ | Controle de Terminal (PTY) | Spawnar terminais em background | Criação de pseudo-terminais (PTYs) reais usando `github.com/creack/pty` em `internal/tools/terminal_command.go`. | **Requer Aprovação (Alta)** |
| **12** | ✅ | Controle de Terminal | Executar comandos shell nativos | `exec.CommandContext` rodando sob o PTY em `internal/tools/terminal_command.go` com variáveis de ambiente limpas. | **Requer Aprovação (Alta)** |
| **13** | ❌ | Controle de Terminal | Ler stdout/stderr em tempo real | Monitoramento de `io.Reader` do PTY com redirecionamento de buffers em tempo real para a CLI/SDK. | Livre (Sem aprovação) |
| **14** | ❌ | Controle de Terminal | Enviar sinais de interrupção (SIGINT) | Envio de sinal `syscall.SIGINT` (Ctrl+C) ao processo PTY em caso de travamentos. | Livre (Sem aprovação) |
| **15** | ✅ | Controle de Terminal | Monitorar portas locais | Verificação de sockets ativos via `net.DialTimeout` na ferramenta nativa `port_monitor`. | Livre (Sem aprovação) |
| **16** | ✅ | Laço Lógico (Agentic Loop) | Análise e extração de erros | Analisador regex que extrai erros de compilação da saída do terminal e exibe sumário estruturado. | Livre (Sem aprovação) |
| **17** | ❌ | Laço Lógico | Execução de rotinas de teste | Execução e acompanhamento automatizado de `go test`, `npm test`, `cargo test` no loop. | **Requer Aprovação (Média)** |
| **18** | ✅ | Laço Lógico | Prevenção de Loops Zumbis | Validação de contexto usando `context.WithTimeout` e contadores de tentativas em `internal/loop/agentic_loop.go`. | Automático (Sem ação do user) |
| **19** | ✅ | Laço Lógico | Auto-validação lógica | Execução de `go vet`/`go fmt` ao final da tarefa, reinserindo erros na iteração ReAct se detectados. | Livre (Sem aprovação) |
| **20** | ✅ | Laço Lógico | Planejamento em múltiplas etapas | Requisito de injeção estruturada do plano de sub-tarefas na primeira resposta de iteração do agente. | Livre (Sem aprovação) |
| **21** | ✅ | Segurança e Interação | Roteamento de aprovação (HITL) | Parada da goroutine bloqueando no canal de aprovação em `internal/permission/manager.go`. | Automático |
| **22** | ✅ | Segurança e Interação | Execução em Sandboxing | Normalização rígida via `filepath.Clean` e verificação de fronteiras em `internal/tools/path.go`. | Automático |
| **23** | ✅ | Segurança e Interação | Mascaramento de dados sensíveis | Ofuscação de chaves de API OpenAI/Anthropic/Gemini e credenciais em logs/históricos salvos no StateManager. | Automático |
| **24** | ✅ | Segurança e Interação | Tratamento de Timeouts | `context.WithTimeout` que cancela o contexto da ferramenta se a execução ultrapassar o limite. | Automático |
| **25** | ❌ | Segurança e Interação | Renderização de DiffZones | Uso de biblioteca Go de diff (ex: `github.com/sergi/go-diff/diffmatchpatch`) para extrair alterações parciais. | Automático |
| **26** | ❌ | Git e Versionamento | Leitura de histórico (Git Log/Status) | Chamadas nativas ao binário `git` mapeando outputs estruturados. | Livre (Sem aprovação) |
| **27** | ❌ | Git e Versionamento | Execução de Add/Commit | Empacotamento de modificações no stage e criação automática do commit via `os/exec`. | **Requer Aprovação (Média)** |
| **28** | ❌ | Git e Versionamento | Geração de mensagens estruturadas | Prompt de IA que analisa o `git diff` e sugere commits no formato Conventional Commits. | Livre (Sem aprovação) |
| **29** | ❌ | Git e Versionamento | Resolução de conflitos (Merge) | Leitura lógica das tags `<<<<<<< HEAD` e re-envio do bloco em conflito para o LLM propor a mesclagem correta. | Livre (Sem aprovação) |
| **30** | ❌ | Git e Versionamento | Troca e criação de Branches | Comandos `git checkout -b` para isolar refatorações, com bloqueio de push forçado (`--force`). | **Requer Aprovação (Média)** |
| **31** | ❌ | Rede e Web | Disparo de requisições HTTP | Cliente `http.Client` em Go com timeouts curtos para verificar endpoints e testar APIs locais. | Livre (Sem aprovação) |
| **32** | ❌ | Rede e Web | Raspagem de documentação | Parser de páginas HTML convertendo tabelas e textos limpos para markdown (RAG em tempo real). | Livre (Sem aprovação) |
| **33** | ❌ | Rede e Web | Teste de conectividade de banco | Tenta abrir conexões rápidas via drivers SQL do Go (`database/sql`) para validar conexões locais. | Livre (Sem aprovação) |
| **34** | ❌ | Rede e Web | Interceptação de tráfego (Proxy) | Proxy TCP local simples em Go para inspecionar payloads enviados e recebidos em testes de integração. | **Requer Aprovação (Alta)** |
| **35** | ❌ | Código e Engenharia | Tradução de Stack técnica | Análise e reescrita de código entre linguagens utilizando análise de árvores sintáticas (AST). | **Requer Aprovação (Média)** |
| **36** | ❌ | Código e Engenharia | Geração de documentação nativa | Geração automatizada de comentários GoDoc ou JSDoc nos cabeçalhos de funções criadas/editadas. | Livre (Sem aprovação) |
| **37** | ❌ | Código e Engenharia | Desofuscação e explicação de código| Varredura AST e análise semântica para gerar explicações detalhadas em formato markdown. | Livre (Sem aprovação) |
| **38** | ❌ | Código e Engenharia | Mocking de dependências | Geração automática de estruturas mockadas e dados falsos em Go ou JSON para testes unitários offline. | Livre (Sem aprovação) |
| **39** | ❌ | Código e Engenharia | Redução de complexidade de código | Análise de nós AST para identificar aninhamentos e sugerir refatoração de funções gigantes. | **Requer Aprovação (Média)** |
| **40** | ❌ | Código e Engenharia | Varredura de vazamentos de memória | Injeção de códigos de diagnóstico de profiling (`pprof` em Go) para análise em tempo de execução. | **Requer Aprovação (Média)** |

---

## 📊 Resumo de Implementação

| Categoria | Implementadas | Total | Progresso |
|---|---|---|---|
| Manipulação de Arquivos | 5 | 5 | 100% |
| Análise de Contexto | 4 | 5 | 80% |
| Controle de Terminal (PTY) | 3 | 5 | 60% |
| Laço Lógico (Agentic Loop) | 3 | 5 | 60% |
| Segurança e Interação | 4 | 5 | 80% |
| Git e Versionamento | 0 | 5 | 0% |
| Rede e Web | 0 | 4 | 0% |
| Código e Engenharia | 0 | 6 | 0% |
| **Total** | **20** | **40** | **50%** |
