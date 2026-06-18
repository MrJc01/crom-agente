# Estudo Comparativo e Posição do crom-agente & crom-agente-sdk

Este documento reúne a análise comparativa entre os principais paradigmas de assistentes de IA e define a posição estratégica do **crom-agente** (CLI) e do **crom-agente-sdk** (SDK).

---

## 🔍 1. Análise Comparativa dos Paradigmas de Agentes

| Critério | Cursor (IDE-first) | Local-first IDE (Soberano/Local) | Claude Agent / Cline (Autônomo) | crom-agente & crom-agente-sdk |
|---|---|---|---|---|
| **Arquitetura** | Fork do VS Code (Electron/Monaco) | Fork offline (Electron sem telemetry) | Extensão de IDE / CLI com laço ReAct | **CLI crom-agente / SDK Go portátil** |
| **Comunicação** | Nuvem com indexação semântica local | 100% Offline (Local Models) | Híbrida (Nuvem ou local via APIs) | **Configurável (HTTP puro / SDKs)** |
| **Autonomia** | Assistida (Chat/Copilot autocomplete) | Assistida e restrita | Elevada (Roda scripts, edita e lê) | **Máxima (Loop ReAct auto-verificável)** |
| **Concorrência** | Inexistente (Execução linear) | Inexistente (Execução linear) | Linear (Um agente por vez) | **Paralela (Goroutines + Canais)** |
| **Portabilidade** | Preso à IDE específica | Preso à IDE específica | Preso à IDE / Ambiente Node | **Binário único e independente** |

---

## 💡 2. Lições Aprendidas de Cada Ferramenta

### A. Cursor (IDE-first)
* **O que extraímos**: O conceito de **Fast Apply** e geração de deltas. Reescrever arquivos gigantes a cada alteração gasta tokens e tempo. Nosso orquestrador em Go usará blocos direcionados de Search/Replace.
* **O que evitamos**: O acoplamento estrito a uma única IDE. Nosso motor é um SDK portátil que pode rodar em qualquer lugar, de um pipeline CI/CD até um terminal SSH.

### B. Local-first IDE (Offline/Soberano)
* **O que extraímos**: Privacidade absoluta e suporte local. O orquestrador Go suportará nativamente conexões com instâncias locais do Ollama e Llama.cpp sem vazar códigos.
* **O que evitamos**: O custo de manutenção de um fork pesado do Electron/Chromium. Nós focamos no core do orquestrador lógico, facilitando a integração de qualquer interface frontend.

### C. Claude Agent / Cline (Loop Cognitivo)
* **O que extraímos**: O modelo de loop ReAct (Reasoning and Acting) com acesso ao terminal via PTY e aprovações HITL (Human-in-the-loop).
* **O que evitamos**: A lentidão de inicialização e a dependência de dependências nativas em Node.js (que quebram ao atualizar versões). Em Go, compilamos tudo estaticamente em um binário nativo.

---

## 🎯 3. Posicionamento Estratégico do crom-agente & crom-agente-sdk

Nosso projeto propõe um **orquestrador híbrido e desacoplado**:

```
 ┌──────────────────────────────────────────────────────────┐
 │                  Frontend / Interface                    │
 │         (CLI Terminal, Extensão VS Code, Web UI)         │
 └────────────────────────────┬─────────────────────────────┘
                              │ gRPC / WebSockets / IPC
 ┌────────────────────────────▼─────────────────────────────┐
 │                 Orquestrador Go (SDK)                    │
 │ ┌──────────────────────────────────────────────────────┐ │
 │ │            Cycle & State Manager (JSON)              │ │
 │ └──────────────────────────┬───────────────────────────┘ │
 │                            ▼                             │
 │ ┌──────────────────────────────────────────────────────┐ │
 │ │      AgenticLoop / Router de Subagentes (Go)        │ │
 │ └──────────────────────────┬───────────────────────────┘ │
 │                            ▼                             │
 │ ┌──────────────────────────────────────────────────────┐ │
 │ │    Tool Registry (PTY, Filesystem, Git, Custom)      │ │
 │ └──────────────────────────────────────────────────────┘ │
 └──────────────────────────────────────────────────────────┘
```

1. **Camada Core em Go Puro**: Garante velocidade máxima e portabilidade.
2. **Abstração de LLMs**: Permite configurar e alternar entre APIs cloud de ponta (Gemini Pro, Claude Sonnet) e modelos locais rodando no próprio computador do desenvolvedor.
3. **Árvore de Subagentes**: Uma goroutine principal de orquestração pode despachar sub-tarefas concorrentes para subagentes, lendo os canais de progresso sem travar a thread de execução do usuário.

---

## 🆚 4. crom-agente vs Claude Code e Outros Concorrentes Modernos

Recentemente, ferramentas baseadas em terminal puro como o **Claude Code** (da Anthropic) e extensões de IDE altamente ativas como o **Cline / Roo Code** e o **Aider** ganharam tração. Abaixo apresentamos uma análise detalhada comparando o `crom-agente` com esses sistemas:

| Característica | Claude Code | Cline / Roo Code | Aider | crom-agente / crom-agente-sdk |
|---|---|---|---|---|
| **Linguagem / Build** | JS / Node.js (requer runtime) | TypeScript (extensão VS Code) | Python (requer ambiente/pip) | **Go puro / Compilado estaticamente** (binário único, <25MB) |
| **Isolamento / Sandbox** | Baseado em permissões do SO do usuário | Permissões do VS Code | Permissões do Python | **Jail / Sandbox nativo no workspace** + HITL avançado |
| **Concorrência** | Monolítica / Linear | Linear (um agente por aba) | Linear | **Múltiplos agentes paralelos** com `MultiAgentManager` em Goroutines |
| **Integração de APIs** | Saída direta no stdout / REPL | Integrado na UI do VS Code | Saída no terminal | **API HTTP, WebSockets e gRPC nativas** + Tray de controle |
| **Suporte MCP** | Limitado / Estático | Nativo (configuração JSON) | Suporte parcial | **Cliente MCP nativo integrado** em Go |
| **Provedores de LLM** | Exclusivo Anthropic (Claude) | Multi-Provider via OpenRouter, etc. | Multi-Provider via LiteLLM | **Multi-Provider nativo** (OpenAI, Gemini, Anthropic, Ollama, OpenRouter, Mock) |

---

## 🔬 5. Análise Detalhada dos Concorrentes

### A. Claude Code (Anthropic)
* **Pontos Fortes**: Excelente tratamento de Prompt Caching (reduz custos ao manter a história do chat viva no cache do Claude) e sintonia fina de comandos Git. O controle de terminal sabe discernir quando um processo de build terminou de um servidor HTTP contínuo.
* **Limitações**: Restrito aos modelos da Anthropic (especificamente `claude-3-7-sonnet`). Roda em JavaScript/Node.js, exigindo gerenciadores de pacote locais para instalação e atualização.
* **Diferencial crom-agente**: O `crom-agente` é multi-provedor (permite usar o Google Gemini 2.5 Pro ou modelos locais de Ollama para tarefas mais baratas) e roda compilado diretamente em Go puro, consumindo muito menos memória e CPU.

### B. Cline / Roo Code (VS Code Extension)
* **Pontos Fortes**: Pioneiro no uso extensivo do **Model Context Protocol (MCP)**, permitindo que servidores externos forneçam ferramentas para o agente de forma dinâmica (como busca de código, banco de dados e APIs). Permite carregar instruções personalizadas por projeto (`.clinerules`).
* **Limitações**: Extremamente acoplado ao ecossistema do VS Code. Para rodar em CI/CD ou servidores remotos sem interface gráfica, o processo torna-se complexo.
* **Diferencial crom-agente**: Oferece o mesmo suporte para MCP e regras locais (`.cromrules`) mas em formato de daemon portátil e REPL nativo, controlável via gRPC/WebSockets, funcionando em qualquer IDE ou linha de comando pura.

### C. Aider (Python CLI)
* **Pontos Fortes**: Criação automática de commits baseados no diff de cada edição realizada. Uso inteligente de um "Repo Map" (gerado via AST de tree-sitter) que envia ao LLM um mapa resumido de assinaturas de funções e classes do repositório inteiro, sem estourar a janela de contexto.
* **Limitações**: Curva de aprendizado íngreme, requer ambiente Python configurado localmente e não possui modelo daemon multi-cliente/multi-projeto integrado.
* **Diferencial crom-agente**: Integra o analisador AST e tradutor de stacks diretamente no binário estático em Go, empacotando monitoramento de vazamentos de memória (`pprof`) e tradução de tipos nativamente.

---

## 🎯 6. Conclusão e Visão de Futuro

O `crom-agente` se posiciona na interseção da portabilidade de terminal do Claude Code com o poder multi-provedor/MCP do Roo Code e a solidez de engenharia de software do Aider. Ao fornecer uma arquitetura em **Go puro**, estruturada como **SDK** e acionada por um **Daemon em segundo plano**, removemos a barreira de dependência de IDEs e runtimes de script, permitindo que a IA atue como um engenheiro autônomo headless em qualquer infraestrutura moderna.


