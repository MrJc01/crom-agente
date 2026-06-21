# 🏗️ Análise Arquitetural Profunda — crom-agente

## 📦 Estrutura de Pacotes

```
crom-agente/
├── cmd/
│   ├── crom-agente/       # Entry point do daemon
│   └── crom-agente-cli/   # Entry point da CLI
├── internal/
│   ├── blackbox/          # Módulo de caixa preta (testes isolados)
│   ├── cli/               # Lógica da CLI interativa
│   ├── cli-tui/           # TUI (Text User Interface)
│   ├── config/             # Sistema de configuração e PromptManager
│   │   └── assets/        # default_prompts.json
│   ├── cron/              # Tarefas agendadas
│   ├── daemon/            # Daemon HTTP + WebSocket + gRPC
│   ├── llm/               # Abstração de provedores de IA
│   ├── loop/              # Motor ReAct (coração do sistema)
│   ├── mcp/               # Protocol MCP (Model Context Protocol)
│   ├── orchestrator/      # Multi-Agente Manager
│   ├── permission/        # HITL (Human-in-the-Loop) Permissions
│   ├── security/          # Redação de dados sensíveis
│   ├── state/             # Persistência de estado (sessões e IterationLogs)
│   └── tools/             # 38 ferramentas nativas
├── pkg/
│   ├── config/            # Config pública para SDK
│   └── sdk/               # SDK programático para agentes
├── scripts/               # Scripts auxiliares
└── tests/                 # Testes de integração
```

---

## 🔄 Fluxograma 1: Loop ReAct Principal (Agente)

O `AgenticLoop.Execute()` segue o padrão **ReAct** (Reason + Act). Com as recentes refatorações, o loop incorpora logs granulares, gerenciamento de prompts via JSON e sumarização inteligente.

```mermaid
flowchart TD
    START["🚀 Execute(ctx, intent)"] --> INIT_STATUS["handler.OnStatusChange('thinking')"]
    INIT_STATUS --> LOAD_MSGS["Carregar mensagens do StateManager"]
    LOAD_MSGS --> FIRST_MSG{É a primeira mensagem?}
    
    FIRST_MSG -->|Sim| OPTIMIZE{"Prompt Optimization habilitada?"}
    OPTIMIZE -->|Sim| LLM_OPTIMIZE["🔮 Chamada LLM: OptimizePrompt()"]
    OPTIMIZE -->|Não| CREATE_MSG["Criar mensagem user com intent"]
    LLM_OPTIMIZE --> CREATE_MSG
    FIRST_MSG -->|Não| APPEND_MSG["Append intent às mensagens existentes"]
    
    CREATE_MSG --> INJECT_SYS["💉 PromptManager.GetAllEnabled()
    Injeta system prompts a partir do default_prompts.json
    (com merges do workspace local)"]
    APPEND_MSG --> LOOP_START
    
    INJECT_SYS --> SAVE_MSGS1["SaveMsgs → StateManager → disco"]
    SAVE_MSGS1 --> LOOP_START
    
    LOOP_START["🔁 for i = 0 → MaxIterations"] --> CTX_CHECK{"ctx cancelado?"}
    CTX_CHECK -->|Sim| EXIT_CANCEL["❌ Loop cancelado"]
    CTX_CHECK -->|Não| BUILD_OPTS
    
    BUILD_OPTS["buildRequestOptions()
    Gera ToolDefinitions (com Tool Pruning)"] --> SYNC_PLAN["SyncPlanToContext()"]
    
    SYNC_PLAN --> COMPACT["compactMessages()
    Se > 40 msgs, sumariza o meio da
    conversa via LLM (HISTORY SUMMARY)"]
    
    COMPACT --> LLM_CALL["📡 provider.SendMessages()"]
    
    LLM_CALL -->|Erro| EXIT_ERROR["❌ Erro na chamada LLM"]
    LLM_CALL -->|OK| RECORD_TOKENS["📊 RecordTokens()"]
    
    RECORD_TOKENS --> CHECK_RESP{"Tem tool_calls?"}
    
    CHECK_RESP -->|Não| TASK_DONE["✅ Tarefa concluída"]
    
    CHECK_RESP -->|Sim| EXEC_TOOLS["🔧 Para cada tool_call:
    tool.Execute(ctx, args)"]
    
    EXEC_TOOLS --> SAVE_ITER_LOG["💾 SaveIterationLog()
    Grava iterations/{00X}.json com
    tokens exatos, ferramentas e status"]
    
    SAVE_ITER_LOG --> CHECK_FAIL{"consecutiveFailures >= max?"}
    CHECK_FAIL -->|Sim| EXIT_FAIL["❌ Abort: falhas consecutivas"]
    CHECK_FAIL -->|Não| LOOP_START
    
    style LLM_OPTIMIZE fill:#ff6b6b,stroke:#333,color:#fff
    style LLM_CALL fill:#ff6b6b,stroke:#333,color:#fff
    style COMPACT fill:#ffa726,stroke:#333,color:#fff
    style SAVE_ITER_LOG fill:#4caf50,stroke:#333,color:#fff
```

---

## 🔄 Fluxograma 2: Criação e Execução de Subagente (Dinâmico)

```mermaid
flowchart TD
    AGENT["🤖 Agente Principal"] --> DECIDES["LLM decide chamar
    spawn_subagent(agentName, task)"]
    
    DECIDES --> HITL["🔐 HITL: Pedir aprovação"]
    
    HITL -->|Aprovado| CREATE["RegisterSpawnSubagentTool()"]
    
    CREATE --> GEN_ID["Gera subagentID"]
    
    GEN_ID --> PARSE_JSON["📄 Lê .crom/agents/{agentName}/agent.json
    Carrega tools, modelo e prompt customizado"]
    
    PARSE_JSON --> NEW_LOOP["Cria novo AgenticLoop com
    ferramentas filtradas pelo JSON"]
    
    NEW_LOOP --> SUB_EXEC["subAL.Execute(ctx, task)"]
    
    SUB_EXEC -->|Sucesso| SUB_OK["✅ Sucesso"]
    SUB_EXEC -->|Falhou| ROLLBACK["🔄 rollbackGit()"]
    
    style PARSE_JSON fill:#2196f3,stroke:#333,color:#fff
    style SUB_EXEC fill:#ff6b6b,stroke:#333,color:#fff
```

---

## 🔄 Fluxograma 3: Comunicação Frontend ↔ Daemon ↔ Agente

```mermaid
mindmap
  root(("Comunicação e Componentes"))
    Frontend("🖥️ crom-agente-app")
      UI("ChatPanel.tsx")
      SYNC("useAgentSync.ts")
    Daemon("🔧 Daemon Go")
      WS("WebSocket Handler")
      ROUTER("IPCRouter")
      HANDLER("daemonAPIEventHandler")
      API("REST API")
    Motor("⚡ Motor ReAct")
      ORCH("MultiAgentManager")
      LOOP("AgenticLoop")
      TOOLS("38 ferramentas nativas")
      LLM("Provedores IA")
      STATE("StateManager")
      PROMPTS("PromptManager")
    Externos("🌐 Módulos Externos")
      MCP("MCP Servers")
      SCRIPTS("Scripts de ferramentas")
      NODE("SDK TS (Node)")
    Disco("💾 Persistência")
      JSON("session.json")
      ITER("iterations/*.json")
```

---

## 📋 Mapeamento de Prompts e Centralização (Refatorado)

Historicamente os prompts eram "hardcoded" em Go. Na arquitetura atual, eles foram **centralizados em JSON** gerenciados pelo `PromptManager`:

- **Assets Base**: Os prompts padrão residem em `internal/config/assets/default_prompts.json` (embutido no binário via `//go:embed`).
- **Overrides de Workspace**: O `PromptManager` lê automaticamente `.crom/prompts.json` na raiz do workspace para sobrescrever prompts, permitindo que a CLI ou o SDK modifiquem o comportamento base em tempo de execução.

**Estrutura do JSON (`default_prompts.json`)**:
```json
{
  "version": "1.0",
  "prompts": {
    "agentic_identity": { "id": "SYSTEM_AGENTIC_IDENTITY", "enabled": true, "content": "..." },
    "planning_requirement": { "id": "SYSTEM_PLANNING_REQUIREMENT", "enabled": true, "content": "..." },
    "tool_usage": { "id": "SYSTEM_TOOL_USAGE_REQUIREMENT", "enabled": true, "content": "..." }
  },
  "overrides": {}
}
```

---

## 🚀 Otimizações de Performance e Tokens Implementadas

| Componente | Otimização Aplicada | Impacto |
|---|---|---|
| **System Prompts** | Prompts redundantes consolidados via `PromptManager`. | Economia fixa de overhead por requisição. |
| **Histórico Longo** | `compactMessages()` faz resumo via LLM (`SYSTEM HISTORY SUMMARY`) em vez de preservar todas as iterações ou cortá-las secamente. | Queda drástica no crescimento linear de tokens em loops longos (> 15 iterações). |
| **Tool Pruning** | Ferramentas de domínio super-específico (como `MCP`) são omitidas na iteração se o intento do usuário não referenciá-las. | Economia de ~500 a 1000 tokens em *Tool Definitions* por request. |
| **Logs de Observabilidade** | A struct `IterationLog` salva o consumo *real* de tokens (Prompt, Completion, Total) devolvido pelo provedor LLM em `.crom/sessions/.../iterations/{00x}.json`. | Permite debugging financeiro granular. |
| **Subagentes Dinâmicos** | Instanciação de subagente não clona mais todas as ferramentas. Lê `agent.json` que restringe o escopo de tools (ex: um subagente "tester" só precisa de `run_tests`). | Maior segurança, menos alucinação e altíssima economia de tokens no loop interno do subagente. |
