# 🧪 Cenário 08: Projeto Multi-Stack (Go + Node.js)

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 6  | Mapear árvore de diretórios |
| 7  | Busca semântica (Grep) |
| 10 | Identificação de Stack técnica (múltiplas) |
| 15 | Monitorar portas locais |
| 31 | Disparo de requisições HTTP |
| 35 | Tradução de Stack técnica |

---

## Contexto

Projeto com arquitetura separada:
- `backend/` — API REST em Go (porta 8080)
- `frontend/` — SPA estática em HTML/JS (porta 5173)

---

## Tarefas para o Agente

### Tarefa 1: Identificar stacks do projeto
O agente deve:
- Executar `tree` para mapear a estrutura completa
- Identificar automaticamente que existem 2 stacks: Go e Node.js
- Reportar as linguagens e ferramentas de build detectadas

### Tarefa 2: Criar API Go no backend
Crie `backend/cmd/api/main.go` com uma API REST que:
- Serve na porta 8080
- Endpoint `GET /api/products` — Lista produtos
- Endpoint `POST /api/products` — Cria produto
- Adiciona headers CORS para permitir requisições do frontend

### Tarefa 3: Criar frontend que consome a API
Crie `frontend/public/index.html` com:
- Formulário para adicionar produtos
- Lista de produtos carregada via `fetch("http://localhost:8080/api/products")`
- Design responsivo e moderno

### Tarefa 4: Levantar ambos os servidores
Use a ferramenta de terminal para:
- Iniciar o backend Go na porta 8080
- Iniciar o frontend na porta 5173
- Usar `port_monitor` para verificar que ambas as portas estão ativas

### Tarefa 5: Gerar tipos compartilhados
Usando a tradução de stack (cap. 35):
- Leia os structs Go do backend (`Product`)
- Gere interfaces TypeScript equivalentes em `frontend/src/types.ts`
- Crie um cliente HTTP tipado em `frontend/src/api.ts`

### Tarefa 6: Criar Makefile
Crie um `Makefile` na raiz com targets:
- `make dev` — Inicia backend e frontend em paralelo
- `make build` — Compila backend Go
- `make test` — Roda testes de ambos
- `make clean` — Limpa artifacts de build
