# 🧪 Cenário 03: Projeto Python — CLI Tool

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1  | Ler arquivos completos |
| 2  | Escrever novos arquivos do zero |
| 3  | Injetar deltas (Diffs/Replace) |
| 6  | Mapear árvore de diretórios |
| 7  | Busca semântica (Grep) |
| 10 | Identificação de Stack (Python) |
| 11-12 | Terminal/Shell |
| 16 | Análise e extração de erros |
| 17 | Execução de rotinas de teste (pytest) |

---

## Tarefas para o Agente

### Tarefa 1: Criar CLI com Click
Crie uma CLI em `src/cli.py` usando a biblioteca `click` com os seguintes comandos:
- `todo add "titulo" --priority high` — Adiciona tarefa
- `todo list` — Lista tarefas (com formatação colorida via rich)
- `todo done <id>` — Marca tarefa como concluída
- `todo remove <id>` — Remove tarefa

As tarefas devem ser persistidas em um arquivo JSON local (`~/.todo.json`).

### Tarefa 2: Criar modelo de dados
Crie `src/models.py` com classes/dataclasses para:
- `Task`: id, title, priority (low/medium/high), done, created_at
- `TaskStore`: métodos para CRUD do arquivo JSON

### Tarefa 3: Adicionar testes com pytest
Crie `tests/test_models.py` com testes para:
- Criação de tarefas com dados válidos
- Validação de prioridades inválidas
- Persistência e carregamento do arquivo JSON
- Filtragem de tarefas por status (done/pending)

### Tarefa 4: Adicionar formatação com Rich
Use a biblioteca `rich` para:
- Exibir a lista de tarefas em uma tabela colorida
- Mostrar progresso ao carregar tarefas
- Usar cores para prioridades (🔴 high, 🟡 medium, 🟢 low)

### Tarefa 5: Adicionar comando de estatísticas
Crie o comando `todo stats` que exiba:
- Total de tarefas
- Tarefas pendentes vs concluídas
- Distribuição por prioridade
- Tarefa mais antiga pendente
