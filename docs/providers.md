# Provedores de LLM do crom-agente

O `crom-agente` suporta múltiplos provedores de LLM (Large Language Models), permitindo ao usuário escolher entre APIs de nuvem, proxies multi-modelo e modelos locais. A configuração é feita via variáveis de ambiente no arquivo `~/.crom/.env` e/ou configuração por workspace em `.crom/config.json`.

---

## 📋 Visão Geral dos Provedores

| Provedor | Identificador CLI | Chave no `.env` | Tipo | Suporta Tool Calls |
|---|---|---|---|---|
| OpenAI | `openai` | `OPENAI_API_KEY` | API Cloud | ✅ Sim |
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | API Cloud | ✅ Sim |
| Google Gemini | `gemini` | `GEMINI_API_KEY` | API Cloud | ✅ Sim |
| Ollama | `ollama` | `OLLAMA_HOST` | Local | ⚠️ Depende do modelo |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | Proxy Cloud | ✅ Sim (depende do modelo) |
| Mock | `mock` | — | Testes | ✅ Sim (simulado) |

---

## 🔧 Configuração do `.env`

O arquivo de segredos fica em `~/.crom/.env` e pode ser editado via CLI:

```bash
# Configurar via CLI (recomendado)
crom-agente config env set OPENAI_API_KEY sk-sua-chave-aqui
crom-agente config env set OPENROUTER_API_KEY sk-or-sua-chave-aqui

# Listar chaves configuradas (mascaradas)
crom-agente config env list
```

### Exemplo completo de `.env`:
```env
# === APIs de Nuvem ===
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
GEMINI_API_KEY=AIza...
OPENROUTER_API_KEY=sk-or-...

# === Modelos Locais ===
OLLAMA_HOST=http://localhost:11434
```

---

## 1. OpenAI (`openai`)

### Configuração
```bash
crom-agente config env set OPENAI_API_KEY sk-sua-chave
crom-agente config workspace set provider openai
crom-agente config workspace set model gpt-4o
```

### Modelos Recomendados
| Modelo | Uso | Tool Calls |
|---|---|---|
| `gpt-4o` | Tarefas complexas, raciocínio | ✅ |
| `gpt-4o-mini` | Tarefas simples, custo baixo | ✅ |
| `o3-mini` | Raciocínio avançado | ✅ |

### Implementação
- Arquivo: `internal/llm/openai.go`
- Endpoint: `https://api.openai.com/v1/chat/completions`
- Formato: API compatível com OpenAI Chat Completions

---

## 2. Anthropic (`anthropic`)

### Configuração
```bash
crom-agente config env set ANTHROPIC_API_KEY sk-ant-sua-chave
crom-agente config workspace set provider anthropic
crom-agente config workspace set model claude-sonnet-4-20250514
```

### Modelos Recomendados
| Modelo | Uso | Tool Calls |
|---|---|---|
| `claude-sonnet-4-20250514` | Desenvolvimento de código | ✅ |
| `claude-opus-4-20250514` | Raciocínio profundo | ✅ |

### Implementação
- Arquivo: `internal/llm/anthropic.go`
- Endpoint: `https://api.anthropic.com/v1/messages`
- Formato: API Anthropic Messages (com conversão interna para formato unificado)

---

## 3. Google Gemini (`gemini`)

### Configuração
```bash
crom-agente config env set GEMINI_API_KEY AIza-sua-chave
crom-agente config workspace set provider gemini
crom-agente config workspace set model gemini-2.5-flash
```

### Modelos Recomendados
| Modelo | Uso | Tool Calls |
|---|---|---|
| `gemini-2.5-flash` | Rápido e barato | ✅ |
| `gemini-2.5-pro` | Raciocínio avançado | ✅ |

### Implementação
- Arquivo: `internal/llm/gemini.go`
- Endpoint: `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`
- Formato: API Google Generative AI (com conversão interna)

---

## 4. Ollama (`ollama`)

### Configuração
```bash
# Iniciar o Ollama (se não estiver rodando)
ollama serve

# Baixar um modelo
ollama pull llama3.1

# Configurar o crom-agente
crom-agente config env set OLLAMA_HOST http://localhost:11434
crom-agente config workspace set provider ollama
crom-agente config workspace set model llama3.1
```

### Modelos Recomendados
| Modelo | Uso | Tool Calls |
|---|---|---|
| `llama3.1` | Uso geral local | ⚠️ Limitado |
| `qwen2.5-coder` | Código, local | ⚠️ Limitado |
| `deepseek-r1` | Raciocínio | ❌ Problemas com parsing |

### Implementação
- Arquivo: `internal/llm/ollama.go`
- Endpoint: `{OLLAMA_HOST}/api/chat`
- Formato: API Ollama Chat (com conversão interna)
- Default: `http://localhost:11434` (se `OLLAMA_HOST` não for definido)

> [!WARNING]
> Muitos modelos locais no Ollama **não suportam tool calls nativamente**, o que pode causar erros HTTP 400 ao tentar usar ferramentas do agente. Nesses casos, o sistema de auto-recuperação (self-healing) tenta limpar o histórico e completar a tarefa em modo texto puro.

---

## 5. OpenRouter (`openrouter`)

### Configuração
```bash
crom-agente config env set OPENROUTER_API_KEY sk-or-sua-chave
crom-agente config workspace set provider openrouter
crom-agente config workspace set model google/gemini-2.5-flash
```

### Como Funciona
O OpenRouter é um **proxy multi-modelo** que dá acesso a centenas de modelos de diferentes provedores via um único endpoint e uma única chave de API. É especialmente útil para:
- Testar modelos diferentes rapidamente sem precisar de múltiplas contas
- Acessar modelos gratuitos (`:free` suffix)
- Comparar resultados entre provedores

### Modelos Gratuitos Populares
| Modelo | Provedor Original | Tool Calls |
|---|---|---|
| `google/gemini-2.5-flash` | Google | ✅ |
| `google/gemma-4-31b-it:free` | Google | ⚠️ |
| `meta-llama/llama-3.3-70b-instruct:free` | Meta | ⚠️ |
| `meta-llama/llama-3.2-3b-instruct:free` | Meta | ❌ |

### Implementação
- Arquivo: `internal/llm/factory.go` (linhas 34-41)
- **Reutiliza o adaptador OpenAI** com URL diferente
- Endpoint: `https://openrouter.ai/api/v1/chat/completions`
- Formato: Compatível com OpenAI Chat Completions

> [!NOTE]
> **Modelos gratuitos** no OpenRouter (sufixo `:free`) frequentemente **não suportam tool calls**, o que causa erro HTTP 400. O crom-agente ativa a auto-recuperação (limpeza de histórico) nesses casos, mas o agente pode não conseguir usar ferramentas de forma confiável.

---

## 6. Mock (`mock`)

### Uso
```bash
crom-agente config workspace set provider mock
```

### Objetivo
Provedor de testes que simula respostas de LLM sem fazer chamadas de rede reais. Usado para:
- Testes unitários e de integração
- Desenvolvimento offline
- Validação do fluxo do AgenticLoop sem custos de API

### Implementação
- Arquivo: `internal/llm/mock_provider.go`
- Retorna respostas pré-configuradas (texto e/ou tool calls)
- Suporta cenários de erro programáveis

---

## 🔄 Hierarquia de Resolução do Provedor

Quando o agente precisa determinar qual provedor usar, a hierarquia de precedência é:

```
CLI Flag (--provider openrouter)    ← Máxima prioridade
        ▼
Workspace config.json               ← Configuração do projeto
        ▼
Global global.json                  ← Defaults do usuário
        ▼
.env (CROM_DEFAULT_PROVIDER)        ← Variável de ambiente
        ▼
Hardcoded Default ("openai")        ← Mínima prioridade
```

---

## 🐛 Troubleshooting por Provedor

### Erro: "OPENROUTER_API_KEY nao esta configurada no .env"
```bash
crom-agente config env set OPENROUTER_API_KEY sk-or-sua-chave
```

### Erro: HTTP 400 com modelos free no OpenRouter
Modelos gratuitos geralmente não suportam tool calls. Use um modelo pago ou o Gemini Flash:
```bash
crom-agente config workspace set model google/gemini-2.5-flash
```

### Erro: "connection refused" com Ollama
Verifique se o Ollama está rodando:
```bash
ollama serve
# Em outro terminal:
curl http://localhost:11434/api/tags
```

### Erro: Auto-recuperação executada (self-healing)
Significa que o provedor retornou um erro e o agente limpou o histórico para tentar novamente. Geralmente causado por:
- Modelos que não suportam tool calls
- Histórico de mensagens muito longo (excede limite de tokens)
- Formato de resposta incompatível
