# crom-agente & crom-agente-sdk

This project aims to develop **crom-agente** (a central daemon/CLI orchestrator of autonomous agents) and **crom-agente-sdk** (a portable Go SDK to create multiple agents), serving as the foundation for other derived agent and SDK projects. Both are inspired by the concepts, studies, and ReAct loop of **Crom Agent** (originally developed in the `crom-agente4` project).

The system is designed to be:
1. A **multi-agent orchestrator** (`crom-agente`) — a single binary capable of managing N independent agents working in N projects/workspaces simultaneously.
2. A **structured Go SDK** (`crom-agente-sdk`) to facilitate the creation, configuration, and orchestration of multiple agents in any Go application.
3. An **agnostic engine** that allows building other derived services and interfaces (WebSockets/gRPC/IPC/UI).

---

## 📂 Monorepo Structure

```
crom-agente5/
├── crom-agente/              # Go CLI Orchestrator + SDK
│   ├── cmd/
│   │   ├── crom-agente/      # Entrypoint for the main orchestrator/daemon binary
│   │   └── crom-agente-cli/  # Entrypoint for the interactive TUI (REPL)
│   ├── internal/             # Internal packages
│   │   ├── cli/              # Cobra commands (root, config, session, workspace, daemon)
│   │   ├── cli-tui/          # Inline TUI interface (ui, commands, markdown, styles)
│   │   ├── config/           # Layered configuration system
│   │   ├── cron/             # Periodic task scheduler
│   │   ├── daemon/           # Persistent daemon, IPC, tray icon, HTTP API
│   │   ├── llm/              # LLM adapters (OpenAI, Gemini, Anthropic, Ollama, OpenRouter)
│   │   ├── loop/             # ReAct loop (AgenticLoop) and subagents
│   │   ├── mcp/              # MCP (Model Context Protocol) client
│   │   ├── orchestrator/     # MultiAgentManager (multi-workspace)
│   │   ├── permission/       # HITL permission manager
│   │   ├── state/            # State and session manager
│   │   ├── tools/            # Native tools (read_file, write_file, terminal, spawn_subagent)
│   │   └── blackbox/         # E2E black-box tests
│   ├── pkg/                  # Public SDK API
│   │   ├── config/           # Exported configuration types
│   │   └── sdk/              # Public Go SDK (Agent, Manager)
│   ├── scripts/              # Build scripts
│   └── bin/                  # Compiled binaries
├── crom-agente-cli/          # Standalone interactive TUI (compiled binary)
├── docs/                     # Project documentation
└── test0/                    # Test workspace
```

---

## 🎯 Project Goals

- **Multi-Agent Multi-Project Orchestration**: The orchestrator runs as a single process and manages multiple concurrent `AgenticLoop`s running in parallel Go routines. Each workspace is independent, with its own settings, permissions, and state.
- **Layered Configuration**: Separation of concerns between **global configuration** (`.env` with API keys + `global.json` with defaults) and **workspace configuration** (`config.json` per project). All editable via the CLI using `crom-agente config`.
- **Standardized Agent Folder Structure**: Each agent has its own folder containing persistent memory, generated artifacts, historical logs, and custom tools.
- **Multitasking & Concurrency**: Orchestration of concurrent subagents running in parallel goroutines and communicating via Go channels (`chan`).
- **Secure System Access**: Native access to the OS with 3 permission modes per workspace (Total Access, Ask Every Time, Scoped Permissions).
- **Abstract LLM Layer**: Plug-and-play support for direct API calls (OpenAI, Anthropic, Gemini, Ollama) and multi-model proxies (OpenRouter) configured via the global `.env` file.
- **Task Scheduler (Cronjobs)**: Programmed execution of recurring tasks associated with specific agents and projects.

---

## 🚀 Quick Start

### 1. Compile the binary

```bash
cd crom-agente
go build -tags headless -o bin/crom-agente ./cmd/crom-agente
```

### 2. Configure API keys

```bash
# Configure OpenRouter key (access to multiple models)
./bin/crom-agente config env set OPENROUTER_API_KEY sk-or-your-key-here

# Or configure a direct provider key
./bin/crom-agente config env set OPENAI_API_KEY sk-your-key-here
```

### 3. Initialize a workspace

```bash
cd ~/my-project
/path/to/bin/crom-agente config workspace set provider openrouter
/path/to/bin/crom-agente config workspace set model google/gemini-2.5-flash
```

### 4. Run a task

```bash
/path/to/bin/crom-agente run "Analyze this code and suggest improvements"
```

### 5. Chat sessions

```bash
# Create a persistent session
/path/to/bin/crom-agente run --session my-session "Hello, analyze the project"

# Continue the same session
/path/to/bin/crom-agente run --session my-session "Now refactor module X"
```

---

## 🔌 Supported LLM Providers

| Provider | `.env` Key | Description |
|---|---|---|
| **OpenAI** | `OPENAI_API_KEY` | Direct OpenAI API (GPT-4o, etc.) |
| **Anthropic** | `ANTHROPIC_API_KEY` | Direct Anthropic API (Claude Sonnet, etc.) |
| **Gemini** | `GEMINI_API_KEY` | Direct Google API (Gemini Pro, Flash, etc.) |
| **Ollama** | `OLLAMA_HOST` | Local models via Ollama (default: `http://localhost:11434`) |
| **OpenRouter** | `OPENROUTER_API_KEY` | Multi-model proxy — access hundreds of models via a single endpoint |
| **Mock** | — | Offline mock provider for testing and development |

---

## 📂 Documentation Layout

For in-depth explanations and designs, check the following documents:

1. [**Comparative Study (comparative_study.md)**](../comparative_study.md): Comparison of Cursor, Local-first IDE, Claude Code, and the positioning of crom-agente.
2. [**Orchestrator Architecture (architecture.md)**](../architecture.md): Agent engine, multi-project execution, concurrency, and directory design.
3. [**Capabilities Catalog (capabilities.md)**](../capabilities.md): List of the 40 native capabilities and implementation status.
4. [**LLM Providers (providers.md)**](../providers.md): Configuration, models, and setup for all supported backends.
5. [**CLI Reference (cli_reference.md)**](../cli_reference.md): CLI commands reference and options.
6. [**SDK and Cron Layer (sdk_and_cron.md)**](../sdk_and_cron.md): How to embed the Go SDK and schedule recurring tasks.
7. [**REPL CLI TUI (repl_cli.md)**](../repl_cli.md): Interactive inline terminal TUI reference and slash (`/`) commands.
8. [**System Tray and Notifications (system_tray.md)**](../system_tray.md): Desktop system tray menus, status indicators, and desktop alerts.
9. [**Security and Best Practices (security.md)**](../security.md): Workspace sandboxing, secret redaction, and containerization guidelines.
