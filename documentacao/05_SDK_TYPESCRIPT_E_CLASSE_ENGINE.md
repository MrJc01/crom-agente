# 📦 05. SDK TypeScript e Classe `CromAgentEngine`

Este documento é a referência completa da biblioteca `@crom/agente-sdk` em TypeScript/JavaScript, cobrindo a classe **`CromAgentEngine`**, parâmetros de configuração zero-file em disco, validação de schema estruturado com Zod/JSON e consumo de métricas de telemetria.

---

## 📑 Sumário
1. [Instalação e Importação](#1-instalação-e-importação)
2. [Interface `CromAgentOptions`](#2-interface-cromagentoptions)
3. [Interface `RunOptions` e Validação de Schema](#3-interface-runoptions-e-validação-de-schema)
4. [Objeto de Resposta `AgentExecutionResponse`](#4-objeto-de-resposta-agentexecutionresponse)
5. [Exemplo Completo com Validação Zod](#5-exemplo-completo-com-validação-zod)

---

## 1. Instalação e Importação

Instale o pacote SDK oficial do ecossistema CromIA no seu projeto Node.js / TypeScript:

```bash
npm install @crom/agente-sdk
```

Importe a classe no seu arquivo TypeScript:

```typescript
import { CromAgentEngine, AgentExecutionResponse, CromAgentOptions } from "@crom/agente-sdk";
```

---

## 2. Interface `CromAgentOptions`

Ao instanciar `new CromAgentEngine(options)`, você parametriza a execução sem necessidade de arquivos `.json` no disco:

| Propriedade | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `provider` | `string` | `"openrouter"` | Provedor de LLM (`"openai"`, `"gemini"`, `"anthropic"`, `"ollama"`, `"openrouter"`). |
| `model` | `string` | `"google/gemini-2.5-flash"` | Identificador do modelo de linguagem. |
| `maxIterations` | `number` | `5` | Máximo de iterações do ciclo cognitivo ReAct por tarefa. |
| `ephemeral` | `boolean` | `true` | Se `true`, cria e apaga workspace efêmero em `/tmp` automaticamente. |
| `workspacePath` | `string` | `os.tmpdir()` | Caminho para o workspace físico (se `ephemeral: false`). |
| `toolsConfig` | `Object` | `{ mode: "none" }` | Restrição de ferramentas (`mode: "none" \| "only" \| "except" \| "plus"`). |
| `mcpServers` | `Array` | `[]` | Lista de servidores MCP inline (Stdin/Stdout ou SSE). |
| `daemonHost` | `string` | `"127.0.0.1"` | IP/Host do Daemon do crom-agente local. |
| `daemonPort` | `number` | `9090` | Porta HTTP/REST do Daemon local. |
| `systemRules` | `string[]` | `[]` | Regras globais injetadas no prompt do sistema. |

---

## 3. Interface `RunOptions` e Validação de Schema

O método `.run<T>(prompt, runOptions)` executa a tarefa e oferece suporte a retornos tipados:

```typescript
export interface RunOptions<T = any> {
  /** Se true, instrui a LLM a retornar JSON limpo */
  jsonResponse?: boolean;
  /** Função de validação/parseamento (ex: schema.parse do Zod) */
  schemaValidator?: (data: any) => T;
}
```

---

## 4. Objeto de Resposta `AgentExecutionResponse`

Cada execução retorna um objeto consolidado contendo os dados e telemetria completa de execução:

```typescript
export interface TelemetryMetrics {
  durationMs: number;       // Tempo total de execução em milissegundos
  promptTokens?: number;    // Tokens do prompt enviado
  completionTokens?: number;// Tokens gerados pela resposta
  totalTokens?: number;     // Total de tokens consumidos
  estimatedCostUSD?: number;// Custo estimado em dólares
}

export interface AgentExecutionResponse<T> {
  data: T;                  // Resultado parseado/validado
  telemetry: TelemetryMetrics;
  modelUsed: string;        // Modelo exato que executou a requisição
}
```

---

## 5. Exemplo Completo com Validação Zod

```typescript
import { CromAgentEngine } from "@crom/agente-sdk";
import { z } from "zod";

// 1. Define o Schema Zod do seu contrato de dados
const RelatorioClienteSchema = z.object({
  clienteId: z.number(),
  nivelRisco: z.enum(["BAIXO", "MEDIO", "ALTO"]),
  recomendacao: z.string(),
  tags: z.array(z.string())
});

type RelatorioCliente = z.infer<typeof RelatorioClienteSchema>;

// 2. Instancia o engine
const engine = new CromAgentEngine({
  provider: "openrouter",
  model: "google/gemini-2.5-flash",
  toolsConfig: { mode: "none" } // Pensamento puro
});

// 3. Executa com validação de schema garantida
export async function avaliarCliente(dadosCliente: object) {
  const prompt = `
Analise o perfil do cliente a seguir:
${JSON.stringify(dadosCliente)}

Retorne um JSON com os campos: clienteId, nivelRisco (BAIXO|MEDIO|ALTO), recomendacao, tags.
`;

  const response = await engine.run<RelatorioCliente>(prompt, {
    jsonResponse: true,
    schemaValidator: (rawJson) => RelatorioClienteSchema.parse(rawJson)
  });

  console.log(`Cliente #${response.data.clienteId} avaliado como ${response.data.nivelRisco}`);
  console.log(`Tempo: ${response.telemetry.durationMs}ms | Modelo: ${response.modelUsed}`);
  
  return response.data;
}
```
