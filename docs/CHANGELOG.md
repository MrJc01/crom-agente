# Registro de Mudanças (Changelog) — crom-agente

Todo o progresso notável deste projeto será documentado neste arquivo.

---

## [0.2.0] — 2026-06-16

### 🚀 Novas Funcionalidades
* **Arquitetura de Daemon Persistente**: Introduzido servidor em segundo plano rodando via Socket Unix (IPC) e portas locais.
* **Ganchos de Integração (gRPC & WebSockets)**:
  * Servidor gRPC gerado a partir de `daemon.proto` ouvindo em porta TCP.
  * Servidor WebSocket sob `/ws` com suporte a multiplos inscritos.
  * Roteador de eventos `AgentEventsRouter` multiplexando progresso em tempo real.
* **Segurança Reforçada**:
  * Implementação de sanitizador de segredos (`redactor.go`) ocultando chaves de API sob padrão `***REDACTED***`.
  * Autenticação baseada em token de sessão local e temporário gerado no startup (`~/.crom/session_token`) para gRPC e WebSockets.
* **Compilação Cross-Platform**:
  * Automação de builds via `scripts/build.sh` para Linux, macOS e Windows.
  * Verificação estática pós-build garantindo binários sob o limite de 25MB.
  * Script de validação usando Docker (`scripts/test_docker.sh`) garantindo compatibilidade no Ubuntu, Alpine e Fedora.

### ⚙️ Melhorias
* Adicionado tratamento robusto de caminhos cruzados, convertendo automaticamente contra-barras (`\`) do Windows para barras normais (`/`).

---

## [0.1.0] — Versão Inicial

### 🚀 Novas Funcionalidades
* Mecanismo ReAct Loop (`AgenticLoop`) básico para tomada de decisão estruturada baseada em ferramentas.
* Ferramentas básicas de manipulação de disco (`read_file`, `write_file`, `tree`, `grep`, `delete_file`).
* Suporte a múltiplos modelos através de adaptadores locais (Ollama) e remotos (OpenAI, Gemini).
* Armazenamento básico de sessões de execução sob arquivos de estado locais `.json`.
