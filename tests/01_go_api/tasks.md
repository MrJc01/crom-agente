# 🧪 Cenário 01: Projeto Go — API REST

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1  | Ler arquivos completos |
| 2  | Escrever novos arquivos do zero |
| 3  | Injetar deltas (Diffs/Replace) |
| 4  | Renomear e mover arquivos |
| 5  | Excluir arquivos e pastas |
| 6  | Mapear árvore de diretórios |
| 7  | Busca semântica (Grep) |
| 9  | Carregar regras locais |
| 10 | Identificação de Stack técnica |
| 11-12 | Terminal/Shell |
| 15 | Monitorar portas locais |
| 16 | Análise e extração de erros |
| 17 | Execução de rotinas de teste |
| 19 | Auto-validação lógica |
| 20 | Planejamento em múltiplas etapas |
| 26-30 | Git (log, commit, branches, conflitos) |
| 36 | Geração de documentação nativa |
| 37 | Desofuscação e explicação de código |
| 38 | Mocking de dependências |
| 39 | Redução de complexidade |
| 40 | Varredura de vazamentos de memória |

---

## Tarefas para o Agente

### Tarefa 1: Criar uma API REST completa
Crie uma API REST em Go usando apenas a biblioteca padrão (`net/http`) com os seguintes endpoints:
- `GET /api/tasks` — Lista todas as tarefas
- `POST /api/tasks` — Cria uma nova tarefa
- `GET /api/tasks/{id}` — Busca uma tarefa por ID
- `PUT /api/tasks/{id}` — Atualiza uma tarefa
- `DELETE /api/tasks/{id}` — Remove uma tarefa

Use um armazenamento em memória (slice/map). O modelo Task deve ter: `ID`, `Title`, `Description`, `Done`, `CreatedAt`.

### Tarefa 2: Adicionar testes unitários
Crie testes unitários abrangentes em `internal/handlers/handlers_test.go` usando `httptest` para cobrir todos os endpoints: criação, listagem, atualização e deleção.

### Tarefa 3: Adicionar middleware de logging
Crie um middleware em `internal/middleware/logger.go` que registre cada requisição com método, path, status code e duração em milissegundos.

### Tarefa 4: Documentar o código
Adicione comentários GoDoc completos em todas as funções públicas e structs exportadas do projeto.

### Tarefa 5: Refatorar handler complexo
O handler de criação de tarefas deve validar input, gerar ID, setar timestamps e retornar JSON. Analise a complexidade ciclomática e sugira refatorações se necessário.

### Tarefa 6: Git workflow
Crie uma branch `feature/add-pagination`, implemente paginação no endpoint `GET /api/tasks` com query params `?page=1&limit=10`, e faça commit com mensagem Conventional Commits.
