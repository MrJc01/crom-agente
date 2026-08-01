# 💡 07. Receitas e Exemplos Práticos de Integração

Este documento fornece **receitas de código completas e prontas para uso em produção** integrando o **`crom-agente`** em aplicações desenvolvidas em Node.js (Express / Fastify), TypeScript, Python (FastAPI) e Go (Fiber / Gin).

---

## 📑 Sumário
1. [Receita 1: API REST em Node.js (Express + TypeScript)](#1-receita-1-api-rest-em-nodejs-express--typescript)
2. [Receita 2: Servidor em TypeScript de Alta Performance (Fastify)](#2-receita-2-servidor-em-typescript-de-alta-performance-fastify)
3. [Receita 3: API Python com FastAPI](#3-receita-3-api-python-com-fastapi)
4. [Receita 4: Microsserviço de Alta Concorrência em Go (Fiber)](#4-receita-4-microsserviço-de-alta-concorrência-em-go-fiber)

---

## 1. Receita 1: API REST em Node.js (Express + TypeScript)

Nesta receita, criamos um endpoint `/api/v1/classificar-feedback` que recebe textos de usuários e usa o `crom-agente` em modo efêmero e pensamento puro (`toolsConfig: { mode: "none" }`) para retornar o sentimento e categoria em JSON:

```typescript
import express, { Request, Response } from "express";
import { CromAgentEngine } from "@crom/agente-sdk";
import { z } from "zod";

const app = express();
app.use(express.json());

// Instância compartilhada do crom-agente (Zero-File em disco)
const agenteAnalisador = new CromAgentEngine({
  provider: "openrouter",
  model: "google/gemini-2.5-flash",
  ephemeral: true,
  toolsConfig: { mode: "none" } // Pensamento puro
});

// Schema Zod de validação
const ResultadoFeedbackSchema = z.object({
  sentimento: z.enum(["POSITIVO", "NEUTRO", "NEGATIVO"]),
  categoria: z.string(),
  urgencia: z.number().min(1).max(5),
  resumo: z.string()
});

type ResultadoFeedback = z.infer<typeof ResultadoFeedbackSchema>;

app.post("/api/v1/classificar-feedback", async (req: Request, res: Response) => {
  const { textoFeedback } = req.body;

  if (!textoFeedback) {
    return res.status(400).json({ error: "Campo 'textoFeedback' é obrigatório" });
  }

  const prompt = `
Analise o feedback do cliente a seguir:
"${textoFeedback}"

Retorne um JSON estrito no formato:
{"sentimento": "POSITIVO"|"NEUTRO"|"NEGATIVO", "categoria": string, "urgencia": number, "resumo": string}
`;

  try {
    const execution = await agenteAnalisador.run<ResultadoFeedback>(prompt, {
      jsonResponse: true,
      schemaValidator: (raw) => ResultadoFeedbackSchema.parse(raw)
    });

    return res.json({
      sucesso: true,
      resultado: execution.data,
      telemetria: {
        tempoMs: execution.telemetry.durationMs,
        modelo: execution.modelUsed
      }
    });
  } catch (err: any) {
    return res.status(500).json({ error: "Falha ao processar feedback", detalhes: err.message });
  }
});

app.listen(3000, () => {
  console.log("🚀 Servidor Express rodando na porta 3000");
});
```

---

## 2. Receita 2: Servidor em TypeScript de Alta Performance (Fastify)

```typescript
import Fastify from "fastify";
import { CromAgentEngine } from "@crom/agente-sdk";

const fastify = Fastify({ logger: true });

const agenteFastify = new CromAgentEngine({
  provider: "openrouter",
  model: "google/gemini-2.5-flash",
  ephemeral: true,
  maxIterations: 3,
  toolsConfig: { mode: "none" }
});

fastify.post("/extrair-entidades", async (request, reply) => {
  const { documentoTexto } = request.body as { documentoTexto: string };

  const prompt = `
Extraia todas as entidades mencionadas (Pessoas, Empresas, Valores, Datas) do texto abaixo:
${documentoTexto}

Retorne um JSON: {"pessoas": string[], "empresas": string[], "valores": string[], "datas": string[]}
`;

  const res = await agenteFastify.run(prompt, { jsonResponse: true });
  return { data: res.data, ms: res.telemetry.durationMs };
});

fastify.listen({ port: 3001 }, () => {
  console.log("⚡ Fastify rodando na porta 3001");
});
```

---

## 3. Receita 3: API Python com FastAPI

Em Python, integramos chamando o Daemon do `crom-agente` via requisições HTTP REST assíncronas (`httpx`):

```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import httpx
import json

app = FastAPI(title="Crom Agent Python API")

CROM_DAEMON_URL = "http://127.0.0.1:9090/api/agent/run"

class InputData(BaseModel):
    texto: str

class OutputData(BaseModel):
    classificacao: str
    confianca: float

@app.post("/classificar", response_model=OutputData)
async def classificar_texto(payload: InputData):
    prompt = f"""
Classifique a mensagem: "{payload.texto}"
Retorne um JSON estrito: {{"classificacao": "string", "confianca": float}}
"""
    body = {
        "workspace": "/tmp/crom-python-ephemeral",
        "provider": "openrouter",
        "model": "google/gemini-2.5-flash",
        "task": prompt,
        "max_iterations": 3,
        "allowed_tools": [] # Pensamento puro
    }

    async with httpx.AsyncClient() as client:
        res = await client.post(CROM_DAEMON_URL, json=body, timeout=30.0)
        if res.status_code != 200:
            raise HTTPException(status_code=500, detail="Erro no crom-agente daemon")
        
        data = res.json()
        raw_summary = data.get("summary", "")
        clean_json = raw_summary.replace("```json", "").replace("```", "").strip()
        parsed = json.loads(clean_json)

        return OutputData(**parsed)
```

---

## 4. Receita 4: Microsserviço de Alta Concorrência em Go (Fiber)

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/crom/crom-agente/pkg/sdk"
	"github.com/gofiber/fiber/v2"
)

type Requisicao struct {
	Texto string `json:"texto"`
}

func main() {
	app := fiber.New()
	manager := sdk.NewManager()

	app.Post("/processar", func(c *fiber.Ctx) error {
		var req Requisicao
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Payload inválido"})
		}

		// Cria um agente efêmero em Go
		agent, err := manager.CreateAgent(sdk.AgentConfig{
			AgentID:  "worker-fiber",
			Provider: "openrouter",
			Model:    "google/gemini-2.5-flash",
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		prompt := fmt.Sprintf("Analise e resuma este texto em 1 frase: %s", req.Texto)
		res, err := agent.ExecuteTask(context.Background(), prompt)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"status":  res.Status,
			"resumo":  res.Summary,
		})
	})

	log.Fatal(app.Listen(":3002"))
}
```
