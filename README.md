# 🤖 Crom Agent (`crom-agente`)

O **`crom-agente`** é o motor central e orquestrador de agentes autônomos locais do Ecossistema Crom. Desenvolvido em **Go**, ele foi projetado para funcionar no modelo *local-first*, servindo como um daemon de segundo plano ou executando de forma independente (*standalone*). Ele gerencia múltiplos agentes autônomos em diferentes espaços de trabalho (*workspaces*), oferecendo controle de segurança rígido por meio de uma sandbox ajustável e um sistema de permissão Human-in-the-Loop (HITL).

Este repositório contém a CLI oficial do motor, o daemon IPC (gRPC/HTTP/Unix Sockets), o motor ReAct, o gerenciador de MCP (Model Context Protocol) e as ferramentas de infraestrutura para os aplicativos clientes do ecossistema.

---

## 📂 1. Arquitetura do Ecossistema (Multi-Repositório)

O ecossistema Crom é modular e desacoplado, sendo distribuído nos seguintes repositórios:

*   **[`crom-agente`](file:///home/j/Documentos/GitHub/crom-agente)** (Este Repositório): O core engine em Go. Contém o daemon persistente, o loop cognitivo ReAct, APIs locais gRPC/HTTP, suporte a MCP e ferramentas nativas de sandbox.
*   **[`crom-agente-sdk`](file:///home/j/Documentos/GitHub/crom-agente-sdk)**: SDK em Go que expõe bindings estruturados para incorporar a engine em qualquer aplicativo Go ou em outras linguagens programaticamente.
*   **[`crom-agente-cli`](file:///home/j/Documentos/GitHub/crom-agente-cli)**: Interface de linha de comando interativa (REPL/TUI) para controle rápido e chat em tempo real com o daemon local.
*   **[`crom-agente-app`](file:///home/j/Documentos/GitHub/crom-agente-app)**: Interface gráfica de usuário (GUI) oficial de desktop construída em Tauri + React + Vite, com painéis de chat, terminal, editor integrado e preview web.
*   **[`cromia-api`](file:///home/j/Documentos/GitHub/cromia-api)**: API/Gateway em nuvem em Go para roteamento unificado de LLMs (como OpenRouter), controle de tokens, autenticação e licenciamento.
*   **[`cromia-site`](file:///home/j/Documentos/GitHub/cromia-site)**: Site institucional e painel de controle administrativo em PHP (Yii2) para gestão de planos, faturamento de tokens e onboarding.

---

## ✨ 2. Principais Funcionalidades

### 🌐 Orquestração Multi-Agente & Multi-Projeto
Um único processo daemon do `crom-agente` gerencia concorrentemente múltiplos loops cognitivos (`AgenticLoop`) rodando em goroutines paralelas. Cada workspace é isolado, tendo suas próprias chaves de API, arquivos de configuração locais (`config.json`), histórico de conversação e arquivos de foco.

### ⚙️ Sistema de Configuração em Camadas
O agente resolve as configurações utilizando uma precedência rígida:
1.  **Flags de CLI** (Ex: `--max-iterations 30` - Prioridade máxima)
2.  **Configuração do Workspace** (`.crom/config.json` no diretório do projeto)
3.  **Configuração Global** (`~/.crom/global.json` para padrões do usuário)
4.  **Variáveis de Ambiente** (`~/.crom/.env` para chaves e segredos de API)
5.  **Padrões Hardcoded** (Precedência mínima)

### 🔁 Ciclo ReAct Cognitivo e Auto-Correção
O agente roda o ciclo cognitivo **ReAct (Reasoning and Acting)**. O loop nativo previne loops repetitivos infinitos analisando o hash das ações recentes, possui mecanismos de auto-correção caso a LLM vaze blocos de código fora do formato esperado, e executa uma **fase de auto-verificação** acionando lints e testes automáticos (como `go vet` ou `npm test`) antes de considerar a tarefa finalizada.

### 🔒 Segurança de Sandbox e Modos de Permissão (HITL)
Cada workspace define o nível de confiança dado ao agente:
*   **Acesso Total (`total_access`)**: Executa qualquer comando ou escrita de arquivo autonomamente.
*   **Perguntar Sempre (`ask_every_time`)**: Toda ação requer aprovação manual do usuário.
*   **Permissões Escopadas (`scoped`)**: Padrão do sistema. O usuário aprova um escopo (ex: comandos `git *` ou escritas em `/src/*`) e o orquestrador salva esse grant em `.crom/permissions.json` para autorizar chamadas futuras equivalentes automaticamente.

### 🔌 Cliente MCP (Model Context Protocol) Nativo
Suporta integração com servidores MCP externos via JSON-RPC 2.0 por Stdin/Stdout, permitindo estender dinamicamente as ferramentas do agente com qualquer servidor MCP disponível na comunidade (como Postgres, Github, etc.).

### 🎙️ Gravação de Mídia e Transcrição Offline (Vosk)
O daemon fornece endpoints de baixo nível para gravação de áudio do sistema com transcrição offline integrada em português via biblioteca Vosk, além de compartilhar janelas específicas do sistema operacional em fluxos de desktop Tauri.

---

## 🏗️ 3. Estrutura de Diretórios (`crom-agente`)

```
crom-agente/
├── bin/                    # Binários executáveis compilados
├── cmd/                    # Pontos de entrada da aplicação
│   ├── crom-agente/        # Ponto de entrada do orquestrador/daemon
│   └── crom-agente-cli/    # Ponto de entrada do cliente CLI clássico
├── docs/                   # Documentação detalhada do projeto
├── internal/               # Código interno do motor (não importável por terceiros)
│   ├── cli/                # Implementação dos comandos Cobra CLI
│   ├── config/             # Gerenciamento de chaves e mesclagem de configurações
│   ├── daemon/             # Servidor gRPC, HTTP, WebSockets e System Tray
│   ├── loop/               # Ciclo cognitivo ReAct, adaptadores de erros e validação
│   ├── llm/                # Clientes e provedores (OpenAI, Anthropic, Gemini, Ollama)
│   ├── orchestrator/       # Gerenciador de múltiplos agentes e concorrência
│   ├── permission/         # Gerenciamento de grants de segurança e HITL
│   ├── tools/              # Ferramentas nativas do sistema (arquivos, terminal PTY, etc.)
│   └── ...
├── pkg/                    # Pacotes públicos e SDK reutilizáveis
│   └── sdk/                # Implementação das interfaces públicas do crom-agente-sdk
├── scripts/                # Scripts auxiliares (builds, transcrição python Vosk, etc.)
├── tests/                  # Suíte de testes de integração e caixa preta
├── Makefile                # Automação de builds, lint e testes
├── go.mod                  # Módulo Go do projeto
└── daemon.proto            # Definição do protocolo gRPC
```

---

## 🚀 4. Quick Start

### Pré-requisitos
*   **Go 1.21+** instalado.
*   Bibliotecas nativas de desenvolvimento para a bandeja do sistema (somente se compilar com suporte a GUI):
    *   No Ubuntu/Debian: `sudo apt-get install libgtk-3-dev libayemu-dev`

### Compilação do Daemon

Você pode compilar o binário em dois modos distintos:

1.  **Modo Headless (Recomendado para servidores, contêineres e testes de terminal)**:
    Ignora as dependências de GUI GTK, compilando um binário leve:
    ```bash
    go build -tags headless -o ./bin/crom-agente ./cmd/crom-agente
    ```

2.  **Modo Completo (Com suporte à bandeja do sistema / System Tray)**:
    Requer bibliotecas nativas de UI instaladas no sistema:
    ```bash
    go build -o ./bin/crom-agente ./cmd/crom-agente
    ```

---

### Configuração de Chaves e Provedores

Você pode gerenciar as credenciais globais diretamente pela CLI, sem precisar mexer nos arquivos manualmente. As chaves de API são armazenadas mascaradas por segurança:

```bash
# Configurar chave do OpenRouter (para acesso multimodelo)
./bin/crom-agente config env set OPENROUTER_API_KEY sk-or-sua-chave-aqui

# Configurar chave direta da Anthropic, OpenAI ou Gemini
./bin/crom-agente config env set ANTHROPIC_API_KEY sk-ant-sua-chave
./bin/crom-agente config env set GEMINI_API_KEY AIzaSuaChave
./bin/crom-agente config env set OPENAI_API_KEY sk-sua-chave-openai

# Configurar host do Ollama local
./bin/crom-agente config env set OLLAMA_HOST http://localhost:11434
```

### Inicializando um Workspace e Executando Tarefas

```bash
# 1. Entre no seu projeto de desenvolvimento
cd ~/meu-projeto

# 2. Inicialize as configurações locais do workspace (.crom/config.json)
/caminho/para/bin/crom-agente config workspace set provider openrouter
/caminho/para/bin/crom-agente config workspace set model google/gemini-2.5-flash
/caminho/para/bin/crom-agente config workspace set permission_mode scoped

# 3. Dispare a primeira tarefa autônoma
/caminho/para/bin/crom-agente run "Analise a estrutura deste projeto e crie um arquivo TODO.md"

# 4. Use sessões de chat persistentes para manter o contexto
/caminho/para/bin/crom-agente run --session chat-desenvolvimento "Crie testes unitários para a função X"
/caminho/para/bin/crom-agente run --session chat-desenvolvimento "Corrija o erro de importação reportado"
```

---

## 🖥️ 5. Funcionamento do Daemon Persistente

O daemon gerencia toda a orquestração concorrente e a comunicação IPC. Ele pode ser ativado e controlado pelos seguintes comandos de terminal:

```bash
# Iniciar o daemon de fundo em segundo plano (headless)
CROM_DISABLE_AUTH=true ./bin/crom-agente daemon start --headless &

# Verificar o estado do daemon ativo e dos agentes que estão executando nele
./bin/crom-agente daemon status

# Interromper o processo graciosamente
./bin/crom-agente daemon stop
```

Quando você roda `crom-agente run` ou `crom-agente config` no terminal, o binário **detecta automaticamente** se o daemon persistente está ativo no sistema:
*   Se o daemon **está ativo**: O comando envia a requisição de forma ultra-leve via Unix Domain Socket (`~/.crom/crom-agente.sock`) para processamento centralizado e exibe o output do agente em streaming.
*   Se o daemon **não está ativo**: O comando executa em modo *standalone* (isolado no próprio processo), rodando o loop ReAct diretamente inline.

---

## 🧪 6. Testes e Qualidade

O projeto preza pelo desenvolvimento seguro baseado em testes para evitar regressões (TDD):

### Executando Testes Unitários
Para disparar a suíte inteira de testes com detecção de concorrência insegura (*data races*):
```bash
go test -v -race -cover ./...
```

### Guia para Executar Inteiramente Local
Se você estiver integrando a CLI ao frontend Tauri (`crom-agente-app`), consulte o guia completo de testes detalhados e configuração de dependências de mídia (Vosk e pickers de monitor) no arquivo [`COMO_TESTAR.md`](file:///home/j/Documentos/GitHub/crom-agente/COMO_TESTAR.md).

---

## 📂 7. Documentação Completa e Referências

O ecossistema está ricamente documentado na pasta [`docs/`](file:///home/j/Documentos/GitHub/crom-agente/docs). Consulte os guias abaixo para aprofundar-se nos tópicos desejados:

*   [**Estudo Comparativo (comparative_study.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/comparative_study.md): Análise aprofundada dos paradigmas do Cursor, Claude Agent, Local-first IDE e como a arquitetura do Crom se posiciona.
*   [**Arquitetura do Orquestrador (architecture.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/architecture.md): Detalhes da engine de execução, workspaces, mapeamento concorrente em goroutines, hierarquia e comunicação de eventos IPC.
*   [**Catálogo de 40 Capacidades (capabilities.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/capabilities.md): Lista com o status de implementação de cada uma das 40 capacidades do agente (Manipulação de arquivos, PTY, Git, Rede, etc.).
*   [**Referência Completa da CLI (cli_reference.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/cli_reference.md): Lista exaustiva de todos os comandos do binário (`run`, `config`, `session`, `workspace`, `daemon`), com flags e parâmetros.
*   [**Provedores de LLM Suportados (providers.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/providers.md): Configuração de APIs da OpenAI, Anthropic, Gemini, Ollama e roteadores de LLM como OpenRouter.
*   [**SDK e Cronjobs (sdk_and_cron.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/sdk_and_cron.md): Funcionamento do agendamento periódico de tarefas e especificações públicas para uso da engine como SDK importável em Go.
*   [**Bandeja de Sistema (system_tray.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/system_tray.md): Integração nativa com systray em modo gráfico e stubs em modo headless.
*   [**Estudo de Segurança (security.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/security.md): Mecanismos de contenção de path traversal, jail do workspace, mitigação de privilege escalation e sanitização de segredos.
*   [**Roadmap de Execução (roadmap.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/roadmap.md): Planejamento detalhado em 11 fases do projeto, com o progresso atualizado.
*   [**Interface de Terminal REPL (repl_cli.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/repl_cli.md): Detalha a TUI interativa inline do `crom-agente-cli` e os comandos de barra (`/add`, `/diff`, `/cost`).
*   [**Backlog de Funcionalidades (backlog.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/backlog.md): Registro das demandas planejadas para evoluir as interações de agentes.
*   [**Troubleshooting (troubleshooting.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/troubleshooting.md): Respostas para problemas comuns de build, conexões de sockets ou variáveis ausentes.
*   [**Changelog (CHANGELOG.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/CHANGELOG.md): Histórico de atualizações e novas implementações.
*   [**Guia de Contribuição (CONTRIBUTING.md)**](file:///home/j/Documentos/GitHub/crom-agente/docs/CONTRIBUTING.md): Diretrizes para desenvolvedores estenderem as capacidades e submeterem modificações.
