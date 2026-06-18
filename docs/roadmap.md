# Roadmap de Implementação para crom-agente & crom-agente-sdk

Este documento estabelece as fases incrementais para o desenvolvimento do crom-agente e crom-agente-sdk, focando inicialmente na CLI, evoluindo para o SDK e garantindo que **cada etapa seja acompanhada de suítes de testes automatizados** para evitar regressões.

---

## 📅 Fase 1: Fundação do Projeto & CLI Inicial ✅
**Objetivo**: Estabelecer a estrutura do projeto em Go, gerenciamento de estado e comandos básicos.
- [x] Inicializar o módulo Go (`go mod init github.com/crom/crom-agente`).
- [x] Definir a estrutura básica de diretórios (`cmd/`, `pkg/`, `internal/`).
- [x] Criar o gerenciador de estado (`state/manager.go`) que lê e salva o arquivo `.crom_state.json`.
- [x] Implementar a CLI inicial utilizando a biblioteca `cobra` para comandos básicos (`version`, `state`, `run`).
- [x] **[TESTS]** Escrever testes unitários para a serialização e concorrência do estado (`manager_test.go`) — **7/7 PASS com race detector**.
- [x] Compilar o binário de produção v0.1.0 e testar os 3 comandos.

## ⚙️ Fase 2: Sistema de Configuração em Camadas ✅
**Objetivo**: Implementar a separação entre configuração global (provedores/chaves) e configuração por workspace (limites, permissões, comportamento do agente).
- [x] Criar o `ConfigLoader` com suporte a leitura de `.env` (segredos) e `global.json` (defaults).
- [x] Criar o `WorkspaceConfig` com leitura de `config.json` por workspace.
- [x] Implementar a função `Resolve()` para merge hierárquico: CLI Flags > Workspace config.json > Global global.json > .env > Hardcoded defaults.
- [x] Implementar os comandos CLI de configuração:
  - [x] `crom-agente config global list/get/set` — gerenciar `global.json`.
  - [x] `crom-agente config env list/set` — gerenciar `.env` (com mascaramento de chaves).
  - [x] `crom-agente config workspace list/get/set` — gerenciar `config.json` do workspace.
  - [x] `crom-agente config resolved` — exibir a configuração efetiva resolvida.
- [x] Refatorar o `AgenticLoop` para usar `ResolvedConfig` ao invés de constantes hardcoded (`MaxIterations`, `MaxConsecutiveFailures`, `ToolTimeoutSeconds`, `MaxMessageHistory`).
- [x] **[TESTS]** Testar o merge de precedência (global + workspace + flags) com todos os cenários de override.
- [x] **[TESTS]** Testar leitura e escrita de `.env` e `config.json` (incluindo criação automática de defaults quando os arquivos não existem).

## 🔁 Fase 3: Ciclo de Execução Principal e LLMs ✅
**Objetivo**: Criar o loop ReAct completo, adaptadores de LLM e detecção de anomalias.
- [x] Criar o core do `AgenticLoop` com suporte a limites de iteração.
- [x] Criar laço de feedback e adaptador/interface `Provider` e o `MockProvider` para testes offline.
- [x] Implementar a interface `Tool` e o registro de ferramentas.
- [x] Implementar detecção de loops repetitivos e auto-correção.
- [x] Implementar fase de verificação automática.
- [x] Adicionar o algoritmo de compactação de histórico de mensagens.
- [x] **[TESTS]** Criar suíte completa do AgenticLoop — **14/14 PASS**.
- [x] Implementar adaptadores de LLM reais (OpenAI, Gemini, Anthropic, Ollama).

## 🌐 Fase 4: Orquestração Multi-Agente Multi-Projeto ✅
**Objetivo**: Transformar o binário em um orquestrador central capaz de gerenciar N agentes em N workspaces simultaneamente.
- [x] Implementar o `MultiAgentManager` com suporte a múltiplos workspaces concorrentes.
- [x] Implementar o `WorkspaceContext` (registro de workspaces e status persistente).
- [x] Implementar o `AgentInstance` (goroutines ativas com `AgenticLoop` gerenciadas).
- [x] Implementar os comandos CLI de workspace:
  - [x] `crom-agente workspace add <path> --name <name>` — registrar workspace.
  - [x] `crom-agente workspace list` — listar workspaces registrados.
  - [x] `crom-agente workspace remove <name>` — remover workspace.
  - [x] `crom-agente status --all` — status de todos os agentes em todos os workspaces.
- [x] Implementar execução simultânea e isolamento por diretório.
- [x] **[TESTS]** Testar concorrência: múltiplos agentes em goroutines com race detector.
- [x] **[TESTS]** Testar isolamento e controle de cancelamento de loops individuais.

## 🛠️ Fase 5: Ferramentas Nativas, Permissões e Segurança 🔧 (Parcial)
**Objetivo**: Integrar as ferramentas básicas de arquivos/terminal, os 3 modos de permissão e limites de sandbox.
- [x] Implementar ferramentas de arquivos (`read_file`, `write_file`) com validação rígida de caminho.
- [x] Implementar execução de terminal (`terminal_command`) via PTY (`creack/pty`) com timeouts e buffers controlados.
- [x] Implementar `spawn_subagent` como ferramenta nativa.
- [x] Criar o `PermissionManager` com suporte aos modos: `total_access`, `ask_every_time` e `scoped` (com Grants salvos em `permissions.json` por workspace).
- [ ] Implementar ferramentas restantes do catálogo de 40 capacidades: `rename_file`, `delete_file`, `tree`, `search_files`, git tools, HTTP client, etc. (**~35 ferramentas pendentes**).
- [x] **[TESTS]** Escrever testes de segurança do sandbox (tentativas de Path Traversal) e validação de comandos bloqueados.
- [x] **[TESTS]** Testar unitariamente os fluxos de permissão (simulando input de usuário e matching de grants salvos).

> **Nota**: Atualmente, apenas 5 ferramentas estão implementadas: `read_file`, `write_file`, `terminal_command`, `spawn_subagent` e `path` (validação). As demais 35 capacidades listadas em `capabilities.md` ainda precisam ser codificadas.

## 🔀 Fase 6: Subagentes e Hierarquia de Execução ✅
**Objetivo**: Suportar subagentes paralelos dentro de cada workspace e refatorar para SDK importável.
- [x] Organizar o código em pacotes públicos no SDK (`crom-agente-sdk`).
- [x] Implementar a ferramenta `spawn_subagent`, permitindo disparar novos loops em goroutines filhas.
- [x] Estruturar a comunicação de volta via canais Go (`chan SubagentEvent`).
- [x] Implementar rollback automático baseado em Git em caso de falhas consecutivas do subagente.
- [x] **[TESTS]** Escrever testes de concorrência disparando múltiplos subagentes em paralelo, validando sincronismo dos canais e isolamento dos estados em disco.

## ⏰ Fase 7: Cronjobs & MCP (Model Context Protocol) ✅
**Objetivo**: Integrar execuções agendadas e ferramentas de terceiros via servidores MCP.
- [x] Integrar a biblioteca `robfig/cron` para agendar tarefas periódicas.
- [x] Criar o cliente MCP nativo em Go, negociando o handshake JSON-RPC 2.0 por stdin/stdout.
- [x] **[TESTS]** Escrever testes integrados para o agendador de tarefas periódicas, simulando a passagem do tempo rápida.
- [x] **[TESTS]** Criar mock de servidor MCP para validar handshake, listagem e chamada remota de ferramentas.

## 🖥️ Fase 8: Daemon Persistente & System Tray 🔧 (Parcial)
**Objetivo**: Transformar o binário em um daemon que roda permanentemente com ícone na bandeja do sistema, IPC via Unix Socket e notificações desktop.
- [x] Implementar o `Daemon` struct com gerenciamento de ciclo de vida (start/stop/restart/autostart).
- [x] Implementar o `IPCServer` com Unix Domain Socket (`~/.crom/crom-agente.sock`).
- [x] Implementar o protocolo `IPCMessage`/`IPCResponse` para comunicação CLI ↔ Daemon.
- [x] Implementar detecção automática de daemon no CLI (se daemon roda → envia via IPC, senão → executa standalone).
- [x] Implementar modo `--headless` para servidores sem GUI.
- [x] Implementar PID file e shutdown gracioso via `os/signal`.
- [/] Integrar `getlantern/systray` para ícone na bandeja com menu de contexto (estrutura criada, mas com stub para builds headless).
- [ ] Integrar `gen2brain/beeep` para notificações desktop nativas (conclusão de tarefas, erros, solicitações HITL).
- [ ] Implementar ícone dinâmico (verde=idle, azul=trabalhando, vermelho=erro) — requer assets de ícone.
- [x] **[TESTS]** Testar IPC: cliente envia comando → daemon recebe → executa → retorna resposta.
- [x] **[TESTS]** Testar detecção automática de daemon (com e sem daemon rodando).
- [x] **[TESTS]** Testar shutdown gracioso: agentes em execução devem salvar estado antes de encerrar.

> **Nota**: O daemon IPC e headless funcionam. O System Tray possui implementação com stub (`tray_stub.go`) para ambientes sem GUI. Notificações desktop e ícones dinâmicos ainda precisam de validação em ambientes gráficos reais.

## 🔌 Fase 9: Ganchos de Integração (WebSockets / gRPC / IPC) 🔧 (Parcial)
**Objetivo**: Preparar a API do SDK para ser consumida por outros projetos e frontends visuais.
- [x] Implementar API HTTP no modo headless (`--port 9090`) para controle remoto (`daemon/api.go`).
- [ ] Criar um servidor gRPC que expõe eventos do agente e diffs (**não implementado — sem arquivos `.proto`**).
- [ ] Criar servidor WebSocket para streaming de eventos em tempo real.
- [ ] **[TESTS]** Criar testes de integração ponta a ponta (E2E), conectando clientes gRPC de mock e disparando tarefas do agente.

> **Nota**: Apenas a API HTTP REST existe em `daemon/api.go`. gRPC e WebSocket ainda não foram implementados.

## 📦 Fase 10: Compilação de Produção & Validação Pós-Build 🔧 (Parcial)
**Objetivo**: Compilar o executável de produção e rodar testes de integração E2E diretamente com o binário compilado em múltiplos cenários do S.O.
- [x] Criar o script de compilação oficial (`scripts/build.sh`) otimizando o tamanho do binário (`go build -ldflags="-s -w"`).
- [/] Implementar testes de caixa preta pós-build que executam o binário `./bin/crom-agente` contra projetos de mock e validam códigos de saída (existe `blackbox/e2e_test.go` com cenários básicos).
- [ ] **[TESTS]** Criar cenários de testes automatizados multi-plataforma (Linux/macOS/Windows) validando comandos em produção (**pendente — apenas Linux testado**).

> **Nota**: O script `build.sh` existe e funciona. Há 1 arquivo de testes E2E básico. Testes cross-platform (macOS/Windows) não foram criados.

## 💬 Fase 11: Suporte a Múltiplas Sessões de Chat (Workspace Multi-Session) 🚀 ✅
**Objetivo**: Permitir a criação de sessões isoladas de chat no mesmo workspace de projeto, preservando o histórico de conversas e isolando estados de turnos, logs e tokens.
- [x] Implementar a persistência do array de mensagens (`Messages`) no `AgentState`.
- [x] Adicionar suporte a múltiplos arquivos de estado (`sessions/<session_name>.json`) via `NewSessionStateManager`.
- [x] Adicionar a flag `--session` nos comandos `run` e `status` da CLI.
- [x] Adicionar comando `crom-agente session` para `list`, `create` e `delete` de sessões.
- [x] Adicionar suporte a sessões no Daemon, IPC/API WebSocket e SDK Público Go.
- [x] **[TESTS]** Validar concorrência de sessões simultâneas e testes unitários.

---

## 🧪 Diretrizes de Testes e Qualidade (TDD/Regressão)

Para garantir que novas modificações não quebrem recursos anteriores (regressão), aplicamos as seguintes políticas de testes:

1. **A Cobertura Vem Primeiro (Test-First/TDD)**:
   - Todo novo arquivo de lógica (`foo.go`) deve ser obrigatoriamente criado em conjunto com seu par de testes (`foo_test.go`).
2. **Cobertura de 100% das 40 Capacidades**:
   - Cada uma das 40 capacidades descritas em [**capabilities.md**](capabilities.md) deve ter pelo menos um teste automatizado dedicado (seja em `internal/tools/write_file_test.go`, `internal/security/path_jail_test.go`, etc.).
   - O orquestrador manterá um validador/mapeamento de testes que relaciona e valida a execução unitária de cada item (1 a 40).
3. **Testes de Integração Pós-Build (Cenários Múltiplos)**:
   - Os testes de produção não rodarão via `go test` sobre o código-fonte, mas sim sobre o binário compilado (`./bin/crom-agente`). O teste irá invocar o binário no terminal disparando tarefas complexas em ambientes descartáveis (ex: diretórios temporários na pasta `/tmp/crom_test_*`) para checar se o comportamento real se comporta perfeitamente no sistema operacional em produção.
4. **Execução de Regressão Automatizada**:
   - Criar um script simplificado no projeto para rodar a suíte completa de testes locais:
     ```bash
     go test -v -race ./...
     ```
   - O flag `-race` deve ser sempre executado para identificar "data races" (concorrência insegura) nas goroutines do `AgenticLoop` e subagentes.
5. **Mocks de LLM**:
   - Evitar chamadas de rede reais durante a execução padrão dos testes unitários para reduzir custos de API e garantir que os testes rodem offline com velocidade. Usar injeção de dependência para passar o cliente mock.

