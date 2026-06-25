# Arquitetura Modular de Agentes e Especialistas (Subagents)

O CROM-Agente adota uma arquitetura modular de múltiplos agentes, onde o agente orquestrador principal (Supervisor) pode delegar tarefas complexas a agentes especialistas (Subagents) dedicados. Cada subagente especialista é encapsulado dinamicamente como uma ferramenta padrão (`tools.Tool`) utilizando a camada adaptadora `AgentToolAdapter`.

---

## 1. Topologia de Agentes (`crom_agents.json`)

A topologia dos especialistas do workspace é definida por meio do arquivo de configuração `.crom/crom_agents.json`.

### Schema da Configuração

```json
{
  "supervisor": {
    "model": "gpt-4o",
    "system_prompt": "Diretrizes adicionais customizadas para o supervisor..."
  },
  "specialists": [
    {
      "name": "browser",
      "type": "native",
      "description": "Especialista em navegação web e automação visual de navegadores",
      "tool_ids": ["scraper", "http_client"]
    },
    {
      "name": "py_analyzer",
      "type": "external",
      "description": "Analisador estático de código Python",
      "exec_path": "/usr/bin/python3",
      "args": ["scripts/analyzer.py"],
      "system_prompt": "Você é o Analisador Estático...",
      "tool_ids": ["read_file"]
    }
  ]
}
```

### Propriedades do Especialista (`SpecialistConfig`)

| Campo | Tipo | Descrição |
| :--- | :--- | :--- |
| `name` | `string` | Identificador único do especialista (usado como o ID da tool). |
| `type` | `string` | Tipo de agente: `native` (Go compilado), `external` (subprocesso) ou `mcp`. |
| `description`| `string` | Descrição do especialista para guiar a delegação do Supervisor. |
| `exec_path` | `string` | Caminho do executável (obrigatório para `external` ou `mcp`). |
| `args` | `[]string`| Argumentos de linha de comando para o processo filho. |
| `url` | `string` | URL SSE para conexões de rede de servidores MCP. |
| `system_prompt`| `string` | Diretrizes de sistema que guiam o comportamento do especialista. |
| `tool_ids` | `[]string`| IDs das ferramentas que este subagente tem permissão para chamar. |

---

## 2. Comandos CLI `crom agent`

A CLI do CROM-Agente fornece comandos integrados para gerenciar e validar os especialistas do workspace.

### Listar Especialistas
Lista todos os agentes carregados na topologia ativa:
```bash
crom agent list
```

### Adicionar Especialista
Adiciona um novo subagente especialista à configuração local `crom_agents.json`:
```bash
crom agent add my_helper \
  --type external \
  --exec-path /usr/bin/python3 \
  --description "Especialista auxiliar em scripts" \
  --args "scripts/helper.py,--verbose" \
  --tools "read_file,write_file"
```

### Remover Especialista
Remove um especialista registrado:
```bash
crom agent remove my_helper
```

### Validar Topologia
Valida a consistência de sintaxe e semântica do arquivo `crom_agents.json`:
```bash
crom agent validate
```

---

## 3. Desenvolvendo Especialistas Nativos (Go SDK)

Para registrar um novo subagente especialista nativo compilado diretamente no binário do CROM-Agente, implemente a interface `core.Agent` e registre o inicializador no registry global do pacote `agents`:

```go
package my_specialist

import (
	"context"
	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/agents/core"
)

func init() {
	agents.RegisterAgent("my_specialist", func(cfg agents.Config) core.Agent {
		return &MySpecialist{
			workspacePath: cfg.WorkspacePath,
			provider:      cfg.LLMProvider,
		}
	})
}

type MySpecialist struct {
	core.BaseAgent
	workspacePath string
	provider      interface{}
}

func (m *MySpecialist) Name() string {
	return "my_specialist"
}

func (m *MySpecialist) Description() string {
	return "Especialista nativo para tarefas customizadas do SDK."
}

func (m *MySpecialist) SystemPrompt() string {
	return "Você é o especialista nativo..."
}

func (m *MySpecialist) ToolIDs() []string {
	return []string{"read_file", "write_file"}
}

func (m *MySpecialist) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	// Lógica de execução do agente (ex: loop ReAct interno ou chamada direta)
	return core.AgentResult{
		Success:        true,
		Output:         "Resultado do processamento nativo.",
		ContextSummary: "Resumo compacto do estado da memória do especialista.",
	}, nil
}
```

---

## 4. Desenvolvendo Especialistas Externos (Python / Node.js)

Especialistas externos rodam em subprocessos independentes e se comunicam com o CROM-Agente via entradas e saídas JSON estruturadas passadas por `stdin` e lidas em `stdout`.

### Protocolo de Comunicação (IPC JSON)

1. **Entrada (Enviado pelo CROM-Agente via `stdin`)**:
   ```json
   {
     "prompt": "Tarefa específica enviada ao especialista",
     "prior_summary": "Histórico resumido acumulado de chamadas anteriores nesta sessão"
   }
   ```

2. **Saída (Retornado pelo Subprocesso via `stdout`)**:
   ```json
   {
     "success": true,
     "output": "Explicação técnica detalhada e resultado final da execução",
     "context_summary": "Resumo atualizado contendo apenas o estado e memórias relevantes para a próxima chamada"
   }
   ```

### Exemplo de Especialista em Python (`test_agent.py`)

```python
#!/usr/bin/env python3
import sys
import json

def main():
    # 1. Carrega dados de entrada do stdin
    try:
        input_data = json.load(sys.stdin)
    except Exception as e:
        print(json.dumps({"success": False, "output": f"Erro de JSON no stdin: {e}", "context_summary": ""}))
        sys.exit(1)

    prompt = input_data.get("prompt", "")
    prior = input_data.get("prior_summary", "")

    # 2. Executa processamento técnico ou chamada de LLM própria
    output_message = f"Processado com sucesso o prompt: {prompt}"
    new_summary = f"Memória mantida sobre o prompt executado. Anterior era: {prior}"

    # 3. Retorna saída estruturada via stdout
    result = {
        "success": True,
        "output": output_message,
        "context_summary": new_summary
    }
    print(json.dumps(result))

if __name__ == "__main__":
    main()
```
