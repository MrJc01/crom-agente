# Guia de Contribuição — crom-agente

Obrigado pelo seu interesse em contribuir para o **crom-agente**! Este documento orienta você sobre o processo de desenvolvimento, padrões de codificação e como enviar suas contribuições de forma eficiente.

---

## 🛠️ Requisitos de Desenvolvimento

Antes de começar, certifique-se de ter instalado:
1. **Go 1.21+**: O projeto utiliza recursos de workspace do Go.
2. **Docker**: Para testes de compatibilidade em containers Linux.
3. **Protobuf Compiler (`protoc`)**: Opcional, para regenerar gRPC (requer `protoc-gen-go` e `protoc-gen-go-grpc`).

---

## 📂 Organização do Repositório

O repositório é organizado em módulos Go sob uma estrutura monorepo:
* **`crom-agente/`**: Contém o módulo principal com o agente ReAct, ferramentas e daemon.
  * `cmd/`: Ponto de entrada do executável.
  * `internal/`: Código privado e de arquitetura.
    * `cli/`: Comandos e parser de flags CLI.
    * `daemon/`: Servidor de segundo plano, APIs HTTP, WebSockets e gRPC.
    * `loop/`: Mecanismo ReAct principal.
    * `tools/`: Ferramentas nativas do agente.
    * `security/`: Sanitização de segredos e logs.
  * `pkg/`: SDK público e bibliotecas utilitárias.
* **`docs/`**: Documentação técnica detalhada.

---

## 🧑‍💻 Fluxo de Contribuição

1. **Fork o Repositório**: Faça um fork do repositório oficial para a sua conta do GitHub.
2. **Crie uma Branch de Feature**:
   ```bash
   git checkout -b feat/sua-feature
   ```
3. **Escreva Código e Testes**: Adicione testes unitários correspondentes para qualquer nova ferramenta ou lógica criada no diretório correspondente.
4. **Execute a Suíte de Validação**:
   Antes de abrir um pull request, valide seu código executando:
   * **Testes unitários**: `go test -v -tags headless ./...`
   * **Compilação**: `bash scripts/build.sh --current-only`
   * **Compatibilidade**: `bash scripts/test_docker.sh`
5. **Crie um Commit**:
   Escreva commits claros no padrão *Conventional Commits*:
   * `feat(tools): add new file analyzer tool`
   * `fix(daemon): solve race condition on shutdown`
   * `docs(changelog): update version info`

---

## 📐 Padrões de Código

* **Formatação**: Rode sempre `go fmt ./...` antes de submeter código.
* **Manejo de Permissões**: Qualquer ferramenta nova que execute no disco ou faça rede deve obrigatoriamente chamar `ValidatePath` (para acesso à sandbox) ou validar via `PermissionManager`.
* **Headless-First**: A interface de tray deve permanecer sob build tags `!headless` para garantir compilações leves e portabilidade para sistemas headless sem servidores visuais (X11/Wayland).
