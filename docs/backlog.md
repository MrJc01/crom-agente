# Backlog de Funcionalidades - Crom Agent

Este documento registra as ideias, melhorias e requisitos propostos pelo usuário para implementação futura no ecossistema **Crom Agent**.

---

## 🚀 Funcionalidades Prioritárias e Futuras

### 1. Formatação de Resultados em Markdown & Visualização de Grafos
* **Status**: 🔴 Pendente
* **Descrição**: Formatar a saída de planos e execuções no chat usando Markdown enriquecido.
* **Recursos**:
  * Renderizar grafos de dependência das tarefas (com Mermaid ou bibliotecas similares de grafos).
  * Exibição clara de blocos de código e scripts de terminal fáceis de copiar.
  * Botão de cópia rápida para comandos de terminal.
  * Integração de scripts executados com referência direta (`@`) no chat.
  * Botão para executar scripts diretamente no terminal a partir do chat.
  * Botão para copiar blocos de script do resultado.

### 2. Seleção de Texto Interativa no Chat (Floating "@" Context Tooltip)
* **Status**: 🔴 Pendente
* **Descrição**: Permitir a seleção livre de qualquer trecho de texto nas mensagens do chat.
* **Recursos**:
  * Ao selecionar um texto, exibir um tooltip flutuante próximo à seleção com a opção **"Mandar como @ context"**.
  * Clicar no botão insere o texto selecionado diretamente como anexo de contexto específico no campo de entrada do chat.
  * Permitir que qualquer trecho da conversa atual seja referenciado diretamente sem precisar reescrever.
  * Permitir selecionar textos do chat livremente (atualmente há dificuldade de seleção).

### 3. Customização da Personalidade do Modelo (System Prompt Creator/Editor)
* **Status**: 🔴 Pendente (interface futura, prioridade baixa)
* **Descrição**: Interface visual para gerenciar o Prompt de Sistema do agente.
* **Recursos**:
  * Botão na lista do menu "+" que abre um dropdown para selecionar personalidades existentes.
  * Opção de criar ou editar personalidades através de um popup com formulário.
  * Armazenar as personalidades no arquivo de configuração do workspace ou de forma global para reuso do agente.

### 4. Refatoração Modular do Código (Backend + Frontend)
* **Status**: 🟡 Planejado — Plano aprovado, execução pendente
* **Descrição**: Dividir arquivos grandes em subpastas, subpacotes e componentes menores.
* **Backend (`crom-agente`)**:
  * `handlers_agent.go` (1200 linhas) → dividir em `handlers_files.go`, `handlers_schedule.go`, `handlers_media.go`, `handlers_system.go`
  * `agentic_loop.go` (686 linhas) → extrair `message_utils.go`, `request_builder.go`, `context_injector.go`
  * `internal/tools/` (36 arquivos flat) → reorganizar em subpacotes: `fs/`, `git/`, `sys/`, `web/`, `ai/`, `sec/`
  * `internal/cli/` → dividir `root.go` e `config.go` em arquivos por comando
* **Frontend (`crom-agente-app`)**:
  * `ChatPanel.tsx` (2201 linhas) → subcomponentes: `ChatInput`, `ChatMessages`, `MessageBubble`, `PinnedItems`, `ModelSelector`, `RecordingControls`, `QuickActions`
  * `AppContext.tsx` (892 linhas) → sub-contexts: `SettingsContext`, `SessionContext`, `WorkspaceContext`, `DaemonContext`
  * `FilePanel.tsx` (820 linhas) → `FileTree`, `FilePreview`, `FileActions`
  * `BottomDashboard.tsx` (653 linhas) → `TerminalTab`, `LogsTab`, `TasksTab`

### 5. Isolamento Completo de Sessões por Pasta
* **Status**: 🟢 Parcialmente implementado (backend ok, frontend pendente)
* **Descrição**: Cada sessão deve ter sua própria pasta com histórico, artefatos, planos e scripts.
* **Estrutura**:
  ```
  .crom/sessions/<session-id>/
  ├── session.json    ✅ (já implementado)
  ├── plan.md         ✅ (já implementado)
  ├── scripts/        🔴 (pendente - criação automática)
  └── artifacts/      🔴 (pendente - criação automática)
  ```
* **Pendente no Frontend**:
  * Listar sessões pela pasta `.crom/sessions/` via API em vez de apenas localStorage.
  * Carregar `session.json` do disco ao restaurar sessão.
  * Sincronizar mensagens bidireccionalmente com o arquivo no disco.
  * Exibir artefatos da sessão (plan.md, scripts/) no painel lateral.

---

## 🛠️ Modificações Realizadas

* ✅ **Aprovação Automática (HITL)**: Criado o botão "+" no chat input que abre o dropdown rápido permitindo alternar a aprovação automática de comandos e ferramentas locais (HITL), enviando o estado `auto_approve` via WebSocket para o daemon em tempo real.
* ✅ **Sincronização Automática do Explorador de Arquivos**: Adicionado um trigger automático de recarga da árvore de arquivos sempre que o agente executa e retorna o resultado de uma ferramenta (`tool_result` com sucesso) ou finaliza uma tarefa (`finished`), além de adicionar um botão de atualização manual (`RefreshCw`) no topo do painel do Explorer.
* ✅ **Persistência de Sessões**: Correção do bug onde o recarregamento da página (reload) perdia o histórico da conversa. As sessões de chat e a sessão ativa são sincronizadas e carregadas do `localStorage` no startup.
* ✅ **Session Storage (Backend)**: `NewSessionStateManager` cria pastas de sessão em `.crom/sessions/<id>/session.json`.
* ✅ **Session Isolation Prompt**: Loop ReAct injeta `[SYSTEM SESSION ISOLATION]` instruindo o agente a salvar artefatos na pasta da sessão.
* ✅ **Plan.md per Session**: `WritePlanToFile` escreve o plano na pasta da sessão ativa.
