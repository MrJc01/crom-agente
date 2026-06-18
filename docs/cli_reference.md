# Referência Completa de Comandos CLI do crom-agente

Documentação detalhada de todos os comandos disponíveis no binário `crom-agente`, incluindo flags globais, subcomandos e exemplos de uso.

---

## 🔧 Flags Globais (Persistentes)

Estas flags estão disponíveis em **todos os comandos**:

| Flag | Tipo | Default | Descrição |
|---|---|---|---|
| `--storage` | string | `.crom` | Diretório de armazenamento do estado do agente |
| `--workspace` | string | `.` | Caminho para o workspace do projeto |
| `--session` | string | — | ID ou nome da sessão de chat no workspace |
| `--provider` | string | — | Override: Provedor de LLM (`openai`, `gemini`, `anthropic`, `ollama`, `openrouter`) |
| `--model` | string | — | Override: Modelo de LLM (ex: `gpt-4o`, `google/gemini-2.5-flash`) |
| `--max-iterations` | int | 0 | Override: Máximo de iterações do loop ReAct |
| `--max-failures` | int | 0 | Override: Máximo de falhas consecutivas de ferramentas |
| `--timeout` | int | 0 | Override: Timeout para execução de ferramentas (segundos) |
| `--max-history` | int | 0 | Override: Limite de mensagens mantidas no histórico |
| `--permission-mode` | string | — | Override: Modo de permissão (`total_access`, `ask_every_time`, `scoped`) |

---

## 📋 Comandos

### `crom-agente version`

Exibe a versão atual do binário.

```bash
$ crom-agente version
crom-agente v0.1.0
```

---

### `crom-agente state`

Exibe o estado atual do agente persistido no disco.

```bash
$ crom-agente state
═══════════════════════════════════════
  crom-agente :: Estado Atual
═══════════════════════════════════════
  Status:     sucesso
  Tarefa:     Escreva 'Sucesso Gemini' no arquivo info.txt
  Tokens:     1234
  Turnos:     3
  Diretório:  /home/j/projeto
  Timestamp:  2026-06-16 07:30:00
═══════════════════════════════════════

# Ver estado de uma sessão específica
$ crom-agente state --session chat-otimizacao
```

---

### `crom-agente run [tarefa]`

Executa uma tarefa utilizando o agente. Este é o **comando principal**.

```bash
# Execução básica
$ crom-agente run "Analise o código e sugira melhorias"

# Com sessão persistente
$ crom-agente run --session revisao "Analise o código do módulo auth"

# Com override de provedor e modelo
$ crom-agente run --provider openrouter --model google/gemini-2.5-flash "Escreva testes unitários"

# Com override de permissão
$ crom-agente run --permission-mode total_access "Execute todos os testes do projeto"

# Com workspace específico
$ crom-agente run --workspace ~/meu-projeto "Refatore o módulo de login"
```

**Comportamento**:
1. Se um daemon está rodando, o comando envia a tarefa via IPC Unix Socket.
2. Se não há daemon, executa o loop ReAct diretamente (modo standalone).
3. A sessão especificada via `--session` persiste o histórico de mensagens.

---

### `crom-agente config`

Gerencia as configurações do sistema. Possui 4 subgrupos:

#### `crom-agente config global`

Gerencia o arquivo `~/.crom/global.json` (defaults globais).

```bash
# Listar todas as configurações globais
$ crom-agente config global list

# Ler um valor
$ crom-agente config global get default_model
# → gpt-4o

# Setar um valor
$ crom-agente config global set max_iterations_default 25
$ crom-agente config global set default_provider openrouter
```

#### `crom-agente config env`

Gerencia o arquivo `~/.crom/.env` (segredos e chaves de API).

```bash
# Listar chaves configuradas (mascaradas por segurança)
$ crom-agente config env list
# OPENAI_API_KEY=sk-***...
# OPENROUTER_API_KEY=sk-or-***...

# Setar uma chave de API
$ crom-agente config env set OPENROUTER_API_KEY sk-or-nova-chave
$ crom-agente config env set OPENAI_API_KEY sk-nova-chave
$ crom-agente config env set OLLAMA_HOST http://localhost:11434
```

#### `crom-agente config workspace`

Gerencia o arquivo `.crom/config.json` do workspace atual.

```bash
# Listar configurações do workspace atual
$ crom-agente config workspace list

# Listar configurações de outro workspace
$ crom-agente config workspace list --workspace ~/api-backend

# Ler um valor
$ crom-agente config workspace get provider

# Setar valores
$ crom-agente config workspace set provider openrouter
$ crom-agente config workspace set model google/gemini-2.5-flash
$ crom-agente config workspace set max_iterations 30
$ crom-agente config workspace set permission_mode total_access
```

**Campos configuráveis do workspace:**

| Campo | Tipo | Descrição |
|---|---|---|
| `provider` | string | Provedor de LLM a utilizar |
| `model` | string | Modelo de LLM |
| `max_iterations` | int | Limite de iterações do loop ReAct |
| `max_consecutive_failures` | int | Limite de falhas consecutivas |
| `max_tokens_per_task` | int | Limite de tokens por tarefa |
| `tool_timeout_seconds` | int | Timeout de ferramentas (segundos) |
| `max_message_history` | int | Limite de mensagens no histórico |
| `permission_mode` | string | Modo de permissão |
| `workspace_jail` | bool | Restringir agente ao workspace |
| `auto_verify` | bool | Verificação automática pós-edição |

#### `crom-agente config resolved`

Exibe a configuração efetiva resolvida (merge de global + workspace + flags).

```bash
$ crom-agente config resolved
═══════════════════════════════════════
  Configuração Resolvida
═══════════════════════════════════════
  Provider:     openrouter
  Model:        google/gemini-2.5-flash
  MaxIterations: 15
  ...
```

---

### `crom-agente session`

Gerencia as sessões de chat isoladas do workspace.

#### `crom-agente session list`

```bash
$ crom-agente session list
═══════════════════════════════════════
  Sessões de Chat Registradas
═══════════════════════════════════════
  - chat-otimizacao
  - revisao-auth
  - debug-api
═══════════════════════════════════════
```

#### `crom-agente session create [nome]`

```bash
$ crom-agente session create nova-feature
✓ Sessão 'nova-feature' criada com sucesso.
```

#### `crom-agente session delete [nome]`

```bash
$ crom-agente session delete sessao-antiga
✓ Sessão 'sessao-antiga' excluída com sucesso.
```

---

### `crom-agente workspace`

Gerencia os workspaces registrados no orquestrador.

#### `crom-agente workspace add [path]`

```bash
$ crom-agente workspace add ~/projeto-web --name web
✓ Workspace 'web' registrado: /home/j/projeto-web
```

#### `crom-agente workspace list`

```bash
$ crom-agente workspace list
═══════════════════════════════════════
  Workspaces Registrados
═══════════════════════════════════════
  - web      → /home/j/projeto-web
  - api      → /home/j/api-backend
═══════════════════════════════════════
```

#### `crom-agente workspace remove [name]`

```bash
$ crom-agente workspace remove web
✓ Workspace 'web' removido.
```

---

### `crom-agente status`

Exibe o status de todos os agentes ativos.

```bash
$ crom-agente status --all
═══════════════════════════════════════
  Status dos Agentes
═══════════════════════════════════════
  web   → running  (Refatorando auth)
  api   → idle
  lib   → finished
═══════════════════════════════════════
```

---

### `crom-agente daemon`

Gerencia o daemon persistente (servidor de fundo).

#### `crom-agente daemon start`

```bash
# Com system tray (requer GUI)
$ crom-agente daemon start

# Sem GUI (servidores, containers)
$ crom-agente daemon start --headless

# Com API HTTP na porta 9090
$ crom-agente daemon start --headless --port 9090
```

#### `crom-agente daemon stop`

```bash
$ crom-agente daemon stop
🔴 Daemon encerrado graciosamente
```

#### `crom-agente daemon status`

```bash
$ crom-agente daemon status
🟢 Daemon ativo (PID: 12345, uptime: 2h34m)
   Agentes ativos: 3
```

#### `crom-agente daemon restart`

```bash
$ crom-agente daemon restart
```

#### `crom-agente daemon autostart`

```bash
# Habilitar auto-start com o sistema
$ crom-agente daemon autostart --enable

# Desabilitar auto-start
$ crom-agente daemon autostart --disable
```

---

## 🎯 Exemplos de Uso Comuns

### Fluxo básico de desenvolvimento
```bash
# 1. Entrar no diretório do projeto
cd ~/meu-projeto

# 2. Configurar provedor e modelo
crom-agente config workspace set provider openrouter
crom-agente config workspace set model google/gemini-2.5-flash

# 3. Executar tarefas com sessão persistente
crom-agente run --session dev "Analise a estrutura do projeto"
crom-agente run --session dev "Agora crie testes para o módulo X"
crom-agente run --session dev "Corrija os erros encontrados"
```

### Testar com diferentes modelos
```bash
# Teste rápido com modelo gratuito
crom-agente run --provider openrouter --model google/gemini-2.5-flash "Descreva o projeto"

# Comparar com GPT-4o
crom-agente run --provider openai --model gpt-4o "Descreva o projeto"

# Usar modelo local
crom-agente run --provider ollama --model llama3.1 "Descreva o projeto"
```

### Multi-workspace
```bash
# Registrar dois projetos
crom-agente workspace add ~/frontend --name front
crom-agente workspace add ~/backend --name back

# Executar tarefas em paralelo (via daemon)
crom-agente daemon start --headless
crom-agente run --workspace front "Atualize os componentes React"
crom-agente run --workspace back "Otimize as queries SQL"
```
