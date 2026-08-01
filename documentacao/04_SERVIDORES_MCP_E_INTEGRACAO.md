# 🔌 04. Servidores MCP (Model Context Protocol) e Integração

Este documento explica como estender o **`crom-agente`** utilizando o **Model Context Protocol (MCP)** para conectar com bancos de dados (PostgreSQL, SQLite), serviços em nuvem (GitHub, Slack) e APIs personalizadas sem modificar arquivos de código local do projeto.

---

## 📑 Sumário
1. [O que é o Model Context Protocol (MCP)?](#1-o-que-é-o-model-context-protocol-mcp)
2. [Arquitetura MCP no `crom-agente`](#2-arquitetura-mcp-no-crom-agente)
3. [Configuração Global vs Configuração Inline via Código](#3-configuração-global-vs-configuração-inline-via-código)
4. [Exemplo 1: Conector PostgreSQL via MCP](#4-exemplo-1-conector-postgresql-via-mcp)
5. [Exemplo 2: Conector GitHub via MCP](#5-exemplo-2-conector-github-via-mcp)
6. [Conectores Remotos via SSE (Server-Sent Events)](#6-conectores-remotos-via-sse-server-sent-events)

---

## 1. O que é o Model Context Protocol (MCP)?

O **MCP** é um padrão aberto desenvolvido para permitir que modelos de inteligência artificial se conectem a fontes de dados externas e ferramentas com segurança e interoperabilidade.

Com o MCP, em vez de escrever adaptadores customizados para cada API:
- O servidor MCP expõe ferramentas (ex: `query_db`, `create_issue`).
- O `crom-agente` descobre essas ferramentas dinamicamente e as registra no ciclo ReAct.

---

## 2. Arquitetura MCP no `crom-agente`

```mermaid
flowchart LR
    subgraph Engine["crom-agente Core Engine"]
        Agent[ReAct Agent Loop]
        MCPClient[Cliente MCP Nativo JSON-RPC 2.0]
    end

    subgraph MCPServers["Servidores MCP Externos"]
        StdioPostgres["npx @modelcontextprotocol/server-postgres (Stdin/Stdout)"]
        StdioGitHub["npx @modelcontextprotocol/server-github (Stdin/Stdout)"]
        RemoteSSE["http://api.empresa.com/mcp/sse (HTTP / SSE)"]
    end

    Agent <--> MCPClient
    MCPClient <-->|IPC Stdin/Stdout| StdioPostgres
    MCPClient <-->|IPC Stdin/Stdout| StdioGitHub
    MCPClient <-->|HTTP / Server-Sent Events| RemoteSSE

    style Engine fill:#1e1e2e,stroke:#89b4fa,color:#cdd6f4
    style MCPServers fill:#181825,stroke:#a6e3a1,color:#cdd6f4
```

---

## 3. Configuração Global vs Configuração Inline via Código

### Opção A: Configuração Global no Usuário (`~/.crom/global.json`)
Registra servidores MCP que ficarão disponíveis para qualquer execução do `crom-agente` no seu sistema:

```json
{
  "mcp_servers": [
    {
      "name": "meu_postgres",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-postgres",
        "postgresql://usuario:senha@localhost:5432/meubanco"
      ]
    }
  ]
}
```

---

### Opção B: Configuração Inline no TypeScript SDK (Zero-File em Disco)
Injeta os servidores MCP diretamente no construtor da classe `CromAgentEngine`:

```typescript
import { CromAgentEngine } from "@crom/agente-sdk";

const agenteComPostgres = new CromAgentEngine({
  provider: "openrouter",
  model: "google/gemini-2.5-flash",
  toolsConfig: { mode: "only", list: ["mcp_meu_postgres_*"] },
  mcpServers: [
    {
      name: "meu_postgres",
      command: "npx",
      args: [
        "-y",
        "@modelcontextprotocol/server-postgres",
        "postgresql://admin:123456@localhost:5432/empresa_db"
      ]
    }
  ]
});
```

---

## 4. Exemplo 1: Conector PostgreSQL via MCP

Com o conector Postgres ativado, o agente ganha ferramentas como `postgres_query` e `postgres_list_tables`:

```typescript
// O agente executa a query no banco de dados e traz o resumo
const resposta = await agenteComPostgres.run(
  "Consulte a tabela de produtos e me diga qual produto tem o maior estoque atual"
);

console.log(resposta.data);
```

---

## 5. Exemplo 2: Conector GitHub via MCP

Conecte o `crom-agente` ao repositório da sua equipe no GitHub via token:

```typescript
const agenteGitHub = new CromAgentEngine({
  mcpServers: [
    {
      name: "github_mcp",
      command: "npx",
      args: ["-y", "@modelcontextprotocol/server-github"],
      env: {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_seu_token_aqui"
      }
    }
  ]
});

const res = await agenteGitHub.run(
  "Liste as últimas 5 issues abertas no repositório minha-empresa/meu-projeto"
);
```

---

## 6. Conectores Remotos via SSE (Server-Sent Events)

Para microsserviços rodando em contêineres ou servidores remotos, o `crom-agente` aceita conexões MCP via HTTP SSE:

```typescript
const agenteRemoto = new CromAgentEngine({
  mcpServers: [
    {
      name: "api_interna",
      url: "http://10.0.0.15:8080/mcp/sse"
    }
  ]
});
```
