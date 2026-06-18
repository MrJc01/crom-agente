# 🧪 Testes de Execução End-to-End — crom-agente

Este diretório contém **cenários de teste realistas** para validar as 40 capacidades do `crom-agente` em projetos de diferentes stacks e contextos.

Cada subpasta é um **mini-projeto isolado** que pode ser usado como workspace do agente para verificar se as ferramentas, o loop ReAct, e a integração com LLMs funcionam corretamente em cenários reais.

---

## 📂 Estrutura dos Cenários

```
tests/
├── README.md                          # Este arquivo
├── run_all.sh                         # Script para executar todos os cenários
│
├── 01_go_api/                         # Projeto Go (API REST)
│   ├── setup.sh                       # Inicializa o projeto
│   ├── tasks.md                       # Tarefas para o agente executar
│   └── ...                            # Código-fonte do projeto
│
├── 02_node_fullstack/                 # Projeto Node.js (Frontend + Backend)
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 03_python_cli/                     # Projeto Python (CLI Tool)
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 04_rust_calculator/                # Projeto Rust (Calculator lib)
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 05_web_static/                     # Projeto HTML/CSS/JS puro
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 06_git_conflicts/                  # Cenário de conflitos Git
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 07_broken_code/                    # Projeto com bugs intencionais
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 08_multi_stack/                    # Projeto multi-linguagem (Go + Node)
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
├── 09_browser_automation/             # Cenário de automação de browser
│   ├── setup.sh
│   ├── tasks.md
│   └── ...
│
└── 10_security_sandbox/               # Testes de segurança e sandbox
    ├── setup.sh
    ├── tasks.md
    └── ...
```

---

## 🎯 Mapeamento de Capacidades por Cenário

| Cenário | Capacidades Testadas (IDs) | Stack |
|---|---|---|
| `01_go_api` | 1-7, 9-12, 15-20, 26-30, 36-40 | Go |
| `02_node_fullstack` | 1-7, 10-12, 16-17, 31-32 | Node.js |
| `03_python_cli` | 1-7, 10-12, 16-17 | Python |
| `04_rust_calculator` | 1-7, 10-12, 16-17, 35-37 | Rust |
| `05_web_static` | 1-3, 6-7, 31-32 | HTML/CSS/JS |
| `06_git_conflicts` | 26-30 | Git |
| `07_broken_code` | 1-3, 7, 16, 19, 37, 39 | Go (buggy) |
| `08_multi_stack` | 6-7, 10, 35 | Go + Node.js |
| `09_browser_automation` | 31-32, Browser Tool, Computer Control | Web |
| `10_security_sandbox` | 5, 21-24, 34 | Segurança |

---

## 🚀 Como Usar

### Executar um cenário individual

```bash
# 1. Entre no cenário desejado
cd tests/01_go_api

# 2. Execute o setup para preparar o workspace
bash setup.sh

# 3. Execute o agente com a tarefa descrita no tasks.md
crom-agente run --workspace . "$(head -1 tasks.md)"
```

### Executar todos os cenários

```bash
bash tests/run_all.sh
```

---

## 📋 Formato dos Arquivos

### `setup.sh`
Script que inicializa o mini-projeto: cria arquivos, dependências, repositório git, etc.

### `tasks.md`
Lista de tarefas que o agente deve executar naquele workspace. Cada tarefa é independente e testa capacidades específicas.

### `expected/`
(Opcional) Diretório com outputs esperados para validação automática.
