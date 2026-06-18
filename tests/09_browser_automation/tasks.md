# 🧪 Cenário 09: Automação de Browser e Computer Use

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 31 | Disparo de requisições HTTP |
| 32 | Raspagem de documentação |
| Browser | `browser_action` (navigate, click, type, screenshot, get_html) |
| Computer | `computer_control` (mouse, keyboard, screenshot) |

---

## Contexto

Este cenário contém páginas web locais e um mini-servidor para testar a automação de browser do agente. As tarefas simulam fluxos reais de interação web.

---

## Tarefas para o Agente

### Tarefa 1: Navegar e extrair dados de formulário
- Inicie um servidor HTTP local servindo a pasta `pages/`
- Navegue até `http://localhost:9090/form.html` usando `browser_action`
- Extraia o HTML da página com `get_html`
- Identifique todos os campos do formulário e seus atributos

### Tarefa 2: Preencher e submeter formulário
- Navegue até `http://localhost:9090/form.html`
- Use `browser_action` com ação `type` para preencher os campos:
  - Nome: "Agente Crom"
  - Email: "agente@crom.dev"
  - Mensagem: "Teste de automação do browser"
- Clique no botão "Enviar" com ação `click`
- Tire screenshot para validar o resultado

### Tarefa 3: Interagir com jogo simples
- Navegue até `http://localhost:9090/game.html`
- O jogo mostra um botão que muda de posição quando clicado
- Use `screenshot` para ver a posição do botão
- Use `click` para clicar no botão
- Repita até atingir 5 pontos (visível no score)

### Tarefa 4: Raspagem de página com tabela
- Navegue até `http://localhost:9090/table.html`
- Use `get_html` para extrair o conteúdo da tabela
- Converta os dados da tabela para formato JSON
- Salve o resultado em `output/table_data.json`

### Tarefa 5: Screenshot e análise visual
- Navegue até `http://localhost:9090/dashboard.html`
- Tire screenshot da página completa
- Analise a imagem via VLM para descrever o layout
- Identifique os gráficos e métricas exibidos
