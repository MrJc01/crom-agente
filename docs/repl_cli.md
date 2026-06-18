# Interface de Terminal Interativa (REPL/TUI): crom-agente-cli

O **`crom-agente-cli`** é a interface de terminal interativa oficial para o `crom-agente`, concebida para fornecer uma experiência de desenvolvimento e co-programming fluida e ágil diretamente pelo console, inspirada em ferramentas como Claude Code e Google Gemini CLI.

Ao contrário de interfaces baseadas em tela cheia (como `vim` ou `nano`), o `crom-agente-cli` adota a filosofia **Inline REPL**, imprimindo logs de execução, outputs de ferramentas e respostas da IA diretamente no buffer padrão do console. Isso preserva o histórico de rolagem do terminal (scrollback) e garante compatibilidade máxima com emuladores de terminal modernos.

---

## 🏗️ 1. Arquitetura da TUI Inline

A TUI é projetada em Go puro, dividida em quatro pilares estruturais sob `internal/cli-tui`:

```
                       ┌─────────────────────────┐
                       │   Entrada de Stdin      │
                       └────────────┬────────────┘
                                    │ Shared Reader (*bufio.Reader)
                                    ▼
                       ┌─────────────────────────┐
                       │   TUIModel / REPL Loop  │
                       └──────┬────────────▲─────┘
                              │            │
             Executa (Async)  │            │ Eventos (Stream)
                              ▼            │
  ┌─────────────────────────────┐        ┌─┴───────────────────────────┐
  │   AgenticLoop (ReAct)       ├───────►│  tuiEventHandler (Spinner)  │
  └─────────────────────────────┘        └─────────────────────────────┘
```

1. **REPL Loop Principal (`ui.go`)**:
   * O loop lê o `os.Stdin` usando o **`Reader` compartilhado**. Ele detecta se a entrada é um comando de barra (`/`) ou uma instrução textual. Se for uma instrução, ele inicia o loop ReAct do agente de forma assíncrona em uma goroutine background, mantendo o console livre para interações secundárias.
2. **Spinner Inline Assíncrono (`InlineSpinner`)**:
   * Enquanto a goroutine do loop ReAct estiver ativa, um spinner (`⠋ Pensando...`) é animado continuamente na linha ativa do terminal.
   * O spinner é temporariamente parado e limpo antes de qualquer texto (logs de ferramentas, diffs, ou respostas da IA) ser impresso no `stdout` para evitar que os dados se misturem e quebrem o visual. Assim que o texto é impresso, o spinner reinicia na linha de baixo.
3. **Leitor de Stdin Compartilhado (`bufio.Reader`)**:
   * Para evitar race conditions e concorrência sobre o descritor de arquivo `os.Stdin` (como ocorria quando o REPL principal e o prompt do HITL tentavam ler o terminal ao mesmo tempo), a TUI compartilha a mesma instância de `*bufio.Reader`. As entradas são consumidas ordenadamente de forma thread-safe.
4. **Renderizador de Markdown (`markdown.go`)**:
   * Utiliza a biblioteca `glamour` configurada para word-wrap dinâmico (com base no tamanho da janela do terminal detectado via `golang.org/x/term`), gerando realce de sintaxe nativo para blocos de código e tabelas.

---

## 💬 2. Referência de Comandos de Barra (Slash Commands)

Durante uma sessão de chat com o agente, você pode digitar comandos especiais iniciados com `/`:

| Comando | Sintaxe | Descrição |
|:---|:---|:---|
| `/help` | `/help` | Exibe a lista de ajuda e descrição dos comandos disponíveis. |
| `/add` | `/add <caminho_arquivo>` | Lê o arquivo local indicado e o anexa como contexto em formato Markdown para o seu próximo envio ao agente. |
| `/session` | `/session <nome_sessao>` | Salva o estado atual e chaveia dinamicamente para outra sessão do workspace, carregando o seu histórico. |
| `/diff` | `/diff` | Roda `git diff` no workspace e formata as modificações no console com cores nativas (verde para inclusões, vermelho para remoções, ciano para hunks). |
| `/cost` | `/cost` ou `/usage` | Exibe em tempo real os tokens de LLM consumidos e a quantidade de turnos decorridos na sessão ativa. |
| `/btw` | `/btw <pergunta>` | Executa uma **pergunta lateral rápida** (Side Question). Ele clona a sessão, roda o loop ReAct e, após retornar a resposta, descarta o turno, preservando o histórico principal. |
| `/compact` | `/compact` | Compacta as mensagens do histórico da sessão (mantendo a intenção original e os últimos turnos) para economizar limites de tokens da API. |
| `/color` | `/color <cor>` | Altera a cor do prompt (`crom-agente >`). Cores suportadas: `red`, `green`, `blue`, `yellow`, `purple`, `cyan`, `orange`, `pink`. |
| `/clear` | `/clear` | Limpa toda a tela do terminal ativo utilizando o escape code `\033[H\033[2J`. |
| `/exit` | `/exit` ou `/quit` | Interrompe o CLI de forma graciosa. |

---

## ⚠️ 3. Fluxo de Confirmação Interativo (HITL - Human In The Loop)

Quando o agente decide executar uma ferramenta crítica (como alterar um arquivo ou rodar um comando no terminal), o loop ReAct bloqueia sua execução e solicita autorização do usuário. 

Na interface do `crom-agente-cli`, esse fluxo é integrado de forma limpa e direta:
1. O spinner é pausado automaticamente.
2. A TUI exibe o aviso em amarelo no stdout:
   ```
   ⚠️  [HITL] crom-agente solicita permissão para a ação [write_file] no alvo: "readme.md"
   👉 Pressione [a] para aprovar uma vez, [s] para sempre permitir, [r] para rejeitar: 
   ```
3. O usuário digita uma das opções e aperta Enter:
   * **`a` (Approve)**: Permite a execução da ferramenta apenas para esta iteração específica.
   * **`s` (Always / Save)**: Permite e adiciona a ação/alvo na lista de grants autorizados do projeto (`.crom/permissions.json`). O agente não voltará a perguntar para alvos que batam com esse padrão.
   * **`r` (Reject)**: Rejeita a ação. O agente recebe a informação da rejeição e tenta recalcular sua rota lógica em modo chat.
4. O leitor compartilhado consome a entrada, repassa a decisão para o `PermissionManager` e o spinner reinicia instantaneamente.

---

## 🛠️ 4. Como Compilar e Rodar

### Compilação
O CLI utiliza dependências de TUI que, por padrão, esperam suporte gráfico. Para gerar um binário leve de produção sem dependência de drivers X11/GTK, compile utilizando a tag `-tags headless`:

```bash
cd crom-agente
go build -tags headless -o bin/crom-agente-cli ./cmd/crom-agente-cli
```

### Inicialização
Após a compilação, você pode executar o binário passando flags para customizar a sessão:

```bash
./bin/crom-agente-cli --workspace ./meu-projeto --session chat-revisao --provider openrouter --model google/gemini-2.5-flash
```

*Se executado de dentro de uma pasta de workspace que já contenha a configuração `.crom/config.json`, as flags de modelo e provedor serão resolvidas automaticamente a partir do arquivo.*
