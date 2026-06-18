# 🧪 Cenário 02: Projeto Node.js — Fullstack (Express + Frontend)

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1  | Ler arquivos completos |
| 2  | Escrever novos arquivos do zero |
| 3  | Injetar deltas (Diffs/Replace) |
| 6  | Mapear árvore de diretórios |
| 7  | Busca semântica (Grep) |
| 10 | Identificação de Stack técnica |
| 11-12 | Terminal/Shell |
| 16 | Análise e extração de erros |
| 17 | Execução de rotinas de teste |
| 31 | Disparo de requisições HTTP |
| 32 | Raspagem de documentação |

---

## Tarefas para o Agente

### Tarefa 1: Criar servidor Express básico
Crie um servidor Express em `src/server.js` com rotas REST para gerenciar uma lista de notas (notes):
- `GET /api/notes` — Lista notas
- `POST /api/notes` — Cria nota
- `DELETE /api/notes/:id` — Remove nota

Use armazenamento em memória (array). Cada nota tem: `id`, `title`, `content`, `createdAt`.

### Tarefa 2: Criar frontend estático
Crie uma página HTML em `public/index.html` com:
- Formulário para adicionar notas
- Lista de notas existentes (carregadas via fetch API)
- Botão de deletar em cada nota
- CSS moderno com design responsivo

### Tarefa 3: Adicionar testes com Node.js test runner
Usando o test runner nativo do Node.js (node:test), crie testes para:
- Criação de notas via POST
- Listagem de notas via GET
- Deleção de notas via DELETE
- Validação de input (nota sem título)

### Tarefa 4: Adicionar middleware de validação
Crie um middleware em `src/middleware/validate.js` que valide o body das requisições POST verificando se `title` e `content` existem e não estão vazios.

### Tarefa 5: Adicionar tratamento de erros global
Crie um error handler centralizado em `src/middleware/errorHandler.js` que capture erros de qualquer rota e retorne respostas JSON estruturadas com status code apropriado.

### Tarefa 6: Documentar a API
Crie um arquivo `API.md` na raiz documentando todos os endpoints, parâmetros, exemplos de request/response e códigos de erro.
