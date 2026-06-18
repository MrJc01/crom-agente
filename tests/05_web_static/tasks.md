# 🧪 Cenário 05: Projeto Web Estático (HTML/CSS/JS)

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1-3 | Manipulação de arquivos |
| 6-7 | Árvore/Grep |
| 31 | Requisições HTTP |
| 32 | Raspagem de documentação |
| Browser | Automação de browser (navigate, screenshot) |

---

## Tarefas para o Agente

### Tarefa 1: Criar landing page moderna
Crie `index.html` com:
- Header com logo e navegação
- Hero section com título, descrição e CTA button
- Seção de features (3 cards com ícones)
- Footer com links e copyright
- Design responsivo (mobile-first)

### Tarefa 2: Estilizar com CSS moderno
Crie `css/style.css` com:
- CSS Variables para cores e espaçamento
- Dark mode usando `@media (prefers-color-scheme: dark)`
- Animações suaves (fade-in, slide-up)
- Layout com CSS Grid e Flexbox
- Tipografia usando Google Fonts

### Tarefa 3: Adicionar interatividade com JavaScript
Crie `js/main.js` com:
- Toggle de dark mode manual (botão no header)
- Smooth scroll para links de navegação
- Animação de entrada ao scroll (Intersection Observer)
- Formulário de contato com validação client-side

### Tarefa 4: Criar página de blog
Crie `blog.html` que:
- Carrega posts de uma API pública (ex: JSONPlaceholder)
- Exibe em formato de cards com título, resumo e autor
- Permite filtrar posts por busca (input de texto)
- Paginação (10 posts por página)

### Tarefa 5: Verificar com browser automation
Use a ferramenta `browser_action` para:
- Navegar até a página local (via file:// ou servidor local)
- Tirar screenshot da landing page
- Verificar se os elementos existem via `get_html`
- Clicar no botão de dark mode e tirar outro screenshot
