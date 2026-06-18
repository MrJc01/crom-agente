# 🧪 Cenário 04: Projeto Rust — Calculator Library

## Capacidades Testadas
| ID | Capacidade |
|----|------------|
| 1-3 | Manipulação de arquivos |
| 6-7 | Árvore/Grep |
| 10 | Identificação de Stack (Rust) |
| 11-12 | Terminal/Shell |
| 16 | Análise de erros (cargo) |
| 17 | Execução de testes (cargo test) |
| 35 | Tradução de Stack técnica |
| 36 | Geração de documentação (rustdoc) |
| 37 | Explicação de código |

---

## Tarefas para o Agente

### Tarefa 1: Implementar calculadora básica
Crie a lib em `src/lib.rs` com uma struct `Calculator` que suporte:
- Operações: `add`, `subtract`, `multiply`, `divide`
- Tratamento de divisão por zero com `Result<f64, CalcError>`
- Histórico de operações (vetor de strings)
- Método `history()` que retorna as operações realizadas

### Tarefa 2: Criar CLI interativa
Crie `src/main.rs` com uma CLI que:
- Leia expressões do stdin (ex: `2 + 3`)
- Suporte operadores: `+`, `-`, `*`, `/`, `%`
- Exiba resultado formatado
- Comando `history` para ver operações anteriores
- Comando `quit` para sair

### Tarefa 3: Adicionar testes unitários
Adicione testes em `src/lib.rs` usando o módulo `#[cfg(test)]`:
- Testes para cada operação aritmética
- Teste de divisão por zero
- Teste do histórico de operações
- Testes de edge cases (números negativos, zero, floats grandes)

### Tarefa 4: Documentar com rustdoc
Adicione documentação `///` em todas as funções públicas e structs. Inclua exemplos de uso nos doc comments usando `/// # Examples`.

### Tarefa 5: Traduzir para Go
Usando a capacidade de tradução de stack (cap. 35), gere uma versão equivalente em Go da struct Calculator e suas operações, salvando em `translated/calculator.go`.
