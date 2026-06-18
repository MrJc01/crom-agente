# 🧪 Cenário 07: Projeto Go com Bugs Intencionais

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1-3 | Manipulação de arquivos (ler, editar, diff) |
| 7  | Busca semântica (Grep) |
| 16 | Análise e extração de erros |
| 19 | Auto-validação lógica (go vet/go fmt) |
| 37 | Desofuscação e explicação de código |
| 39 | Redução de complexidade de código |

---

## Contexto

Este projeto contém **bugs intencionais** de diferentes categorias:
1. **Erros de compilação** — Código que não compila
2. **Erros de lógica** — Código que compila mas produz resultados errados
3. **Erros de estilo** — Código mal formatado e complexo demais
4. **Erros de concorrência** — Race conditions e deadlocks potenciais
5. **Vazamentos de recursos** — Goroutines e file handles não fechados

---

## Tarefas para o Agente

### Tarefa 1: Corrigir erros de compilação
O arquivo `internal/calculator/math.go` contém erros de compilação. Execute `go build ./...`, analise os erros e corrija-os.

### Tarefa 2: Corrigir erros de lógica
O arquivo `internal/parser/csv.go` compila mas produz resultados incorretos. Execute os testes em `internal/parser/csv_test.go` e corrija a lógica.

### Tarefa 3: Refatorar código complexo
O arquivo `internal/formatter/format.go` contém uma função com complexidade ciclomática altíssima (> 20). Analise e refatore em funções menores e mais legíveis.

### Tarefa 4: Corrigir race condition
O arquivo `internal/calculator/concurrent.go` tem uma race condition. Execute com `-race` flag e corrija.

### Tarefa 5: Auto-validação completa
Execute `go vet ./...` e `go fmt ./...` no projeto inteiro e corrija todos os warnings restantes.
