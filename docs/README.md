# crom-agente & crom-agente-sdk

Este projeto tem como objetivo desenvolver o **crom-agente** (um orquestrador central de agentes autônomos via CLI) e o **crom-agente-sdk** (um SDK portátil em Go para criação de múltiplos agentes), servindo como fundação para outros projetos derivados do agente e do SDK. Ambos são inspirados nos conceitos, estudos e ciclo ReAct do **Crom Agent** (desenvolvido originalmente no projeto `crom-agente4`).

O sistema é concebido para ser:
1. Um **orquestrador multi-agente** (`crom-agente`) — um binário único capaz de gerenciar N agentes trabalhando em N projetos/workspaces simultaneamente.
2. Um **SDK em Go estruturado** (`crom-agente-sdk`) para facilitar a criação, parametrização e orquestração de múltiplos agentes em qualquer aplicação Go.
3. Um **motor agnóstico** que permite construir outros serviços e interfaces derivados (WebSockets/gRPC/IPC/UI).

---

## 📂 Arquitetura do Ecossistema (Multi-Repositório)

O projeto evoluiu de um monorepo para um ecossistema **desacoplado**, onde cada componente vive em seu próprio repositório independente:

- **`crom-agente`**: O motor central. Daemon persistente em Go, orquestrador ReAct, servidor gRPC/HTTP e gerenciador de ferramentas e workspaces.
- **`crom-agente-sdk`**: SDK público que fornece bindings e interfaces (Go, Python, etc) para se comunicar com o daemon local de forma programática.
- **`crom-agente-cli`**: Interface de terminal interativa (REPL/TUI). Conecta-se ao daemon para chat e controle.
- **`crom-agente-app`**: O aplicativo Desktop gráfico (GUI) construído com Tauri + React + Vite, oferecendo a experiência de usuário final premium.
- **`cromia-api`**: O Gateway/API na nuvem (Go) para roteamento de LLMs (OpenRouter, local) e verificação de tokens/licenças.
- **`cromia-site`**: O painel e site institucional em Yii2 (PHP) para gestão de contas, venda de tokens e onboarding.

---

## 🎯 Objetivos do Projeto

- **Orquestração Multi-Agente Multi-Projeto**: O binário roda como processo único e gerencia múltiplos `AgenticLoop` concorrentes, cada um atuando em workspaces independentes com suas próprias configurações, permissões e estado.
- **Configuração em Camadas**: Separação clara entre **configuração global** (`.env` com chaves de API + `global.json` com defaults) e **configuração por workspace** (`config.json` por projeto). Tudo editável pelo próprio binário via `crom-agente config`.
- **Estruturação Padronizada de Agentes**: Cada agente possui sua própria pasta contendo memória persistente, artefatos gerados, logs históricos e funções específicas.
- **Multitasking & Concorrência**: Orquestração de subagentes concorrentes rodando em goroutines paralelas e se comunicando através de canais Go (`chan`).
- **Acesso ao Sistema com Segurança**: Acesso nativo ao sistema operacional com 3 modos de permissão por workspace (Acesso Total, Perguntar Sempre, Permissões Escopadas).
- **Camada de LLM Abstrata**: Suporte plugável para chamadas de APIs diretas (OpenAI, Anthropic, Gemini, Ollama) e proxies multi-modelo (OpenRouter) configuradas via `.env` global.
- **Agendador de Tarefas (Cronjobs)**: Execução programada de tarefas periódicas associadas a agentes e projetos específicos.

---

## 🚀 Quick Start

### 1. Compilar o binário

```bash
cd crom-agente
go build -tags headless -o bin/crom-agente ./cmd/crom-agente
```

### 2. Configurar chaves de API

```bash
# Configurar chave do OpenRouter (acesso a múltiplos modelos)
./bin/crom-agente config env set OPENROUTER_API_KEY sk-or-sua-chave-aqui

# Ou configurar diretamente um provedor
./bin/crom-agente config env set OPENAI_API_KEY sk-sua-chave-aqui
```

### 3. Inicializar um workspace

```bash
cd ~/meu-projeto
/caminho/para/bin/crom-agente config workspace set provider openrouter
/caminho/para/bin/crom-agente config workspace set model google/gemini-2.5-flash
```

### 4. Executar uma tarefa

```bash
/caminho/para/bin/crom-agente run "Analise o código e sugira melhorias"
```

### 5. Sessões de chat

```bash
# Criar uma sessão persistente
/caminho/para/bin/crom-agente run --session minha-sessao "Olá, analise o projeto"

# Continuar a mesma sessão
/caminho/para/bin/crom-agente run --session minha-sessao "Agora refatore o módulo X"
```

---

## 🔌 Provedores de LLM Suportados

| Provedor | Chave `.env` | Descrição |
|---|---|---|
| **OpenAI** | `OPENAI_API_KEY` | API direta da OpenAI (GPT-4o, etc.) |
| **Anthropic** | `ANTHROPIC_API_KEY` | API direta da Anthropic (Claude Sonnet, etc.) |
| **Gemini** | `GEMINI_API_KEY` | API direta do Google (Gemini Pro, Flash, etc.) |
| **Ollama** | `OLLAMA_HOST` | Modelos locais via Ollama (default: `http://localhost:11434`) |
| **OpenRouter** | `OPENROUTER_API_KEY` | Proxy multi-modelo — acesso a centenas de modelos (gratuitos e pagos) via um único endpoint |
| **Mock** | — | Provedor de testes offline (para desenvolvimento) |

---

## 📂 Organização da Documentação

Para entender a fundo o design planejado para o orquestrador, consulte os seguintes documentos:

1. [**Estudo Comparativo (comparative_study.md)**](docs/comparative_study.md): Analisa a fundo os paradigmas do Cursor, Local-first IDE, Claude Agent e o posicionamento do crom-agente e crom-agente-sdk.
2. [**Arquitetura do Orquestrador (architecture.md)**](docs/architecture.md): Detalha a engine do agente, o **sistema de configuração em camadas** (global vs workspace), a **orquestração multi-agente multi-projeto**, o modelo de concorrência com subagentes, o registro de ferramentas e a estrutura de pastas isoladas.
3. [**Catálogo de Capacidades (capabilities.md)**](docs/capabilities.md): Mapeia as 40 capacidades do agente em Go, o funcionamento interno em baixo nível, os requisitos de segurança e o **status de implementação** de cada uma.
4. [**Provedores de LLM (providers.md)**](docs/providers.md): Documentação detalhada de todos os provedores suportados (OpenAI, Anthropic, Gemini, Ollama, OpenRouter, Mock), configuração, modelos e troubleshooting.
5. [**Referência de Comandos CLI (cli_reference.md)**](docs/cli_reference.md): Referência completa de todos os comandos do binário, flags globais e exemplos de uso.
6. [**Camada de SDK e Cron (sdk_and_cron.md)**](docs/sdk_and_cron.md): Detalha como as estruturas Go serão consumidas como SDK e como o agendamento de tarefas periódicas funcionará.
7. [**Roadmap de Execução (roadmap.md)**](docs/roadmap.md): O passo a passo em 11 fases para tirar o projeto do papel, desde a CLI até as integrações de interface.
8. [**Interface de Terminal REPL (repl_cli.md)**](docs/repl_cli.md): Detalha a TUI interativa inline do `crom-agente-cli`, os comandos de barra (`/add`, `/btw`, `/diff`, `/cost`, etc.) e o fluxo de permissão HITL.
9. [**Backlog de Funcionalidades (backlog.md)**](docs/backlog.md): Registra as melhorias sugeridas pelo usuário como personalização de prompt, seleção de texto para contexto e formatação de resultados com grafos.

