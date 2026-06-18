# SDK e Sistema de Agendamento (Cron) em Go

Este documento especifica o design da API do SDK em Go e o mecanismo de agendamento de tarefas recorrentes (Cronjobs) integrados ao orquestrador.

---

## 📦 1. Estrutura e API do SDK em Go

O SDK é projetado para permitir a criação programática de agentes altamente configuráveis de forma simples e padronizada.

### Exemplo de Uso do SDK

Abaixo, apresentamos como o SDK Go real pode ser importado e consumido por aplicações externas. O SDK está localizado em `github.com/crom/crom-agente/pkg/sdk` e as configurações básicas em `github.com/crom/crom-agente/pkg/config`.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/crom/crom-agente/pkg/config"
	"github.com/crom/crom-agente/pkg/sdk"
)

func main() {
	// 1. Inicializa a configuração do SDK
	sdkConfig := config.Config{
		WorkspaceRoot:  "/home/j/Área de trabalho/GitHub/meu-projeto",
		StoragePath:    "/home/j/Área de trabalho/GitHub/meu-projeto/.crom",
		AllowSystem:    false, // Restringe acesso apenas ao WorkspaceRoot por segurança
		
		// Modos de permissão disponíveis em config.PermissionMode:
		// - config.ModeTotalAccess      -> Acesso total sem perguntar (HITL desativado)
		// - config.ModeAskEveryTime     -> HITL estrito (pergunta sempre)
		// - config.ModeScopedPermissions -> Permissões específicas salvas (HITL inteligente)
		PermissionMode: config.ModeScopedPermissions,
	}
	manager := sdk.NewManager(sdkConfig)

	// 2. Registra um Agente com configurações customizadas
	agentConfig := sdk.AgentConfig{
		AgentID:     "analista-seguranca",
		Model:       "gemini-1.5-pro",
		Provider:    "google",
		Temperature: 0.1,
		SystemRules: []string{
			"Foque na segurança e integridade do código.",
			"Não commite credenciais ou chaves sob nenhuma circunstância.",
		},
	}
	
	agent, err := manager.CreateAgent(agentConfig)
	if err != nil {
		log.Fatalf("Erro ao criar agente: %v", err)
	}

	// 3. Opcional: Definir SessionName para isolamento da sessão e histórico
	agent.SessionName = "sessao-seguranca-diaria"

	// 4. Executa uma tarefa direta
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := agent.ExecuteTask(ctx, "Faça uma varredura estática por chaves de API expostas no código.")
	if err != nil {
		log.Fatalf("Erro na execução da tarefa: %v", err)
	}

	fmt.Printf("Resultado da análise: %s (Status: %s)\n", result.Summary, result.Status)
}
```

---

## ⏰ 2. Sistema de Agendamento de Tarefas (Cronjobs)

O orquestrador conta com um módulo de agendamento em `github.com/crom/crom-agente/internal/cron` (`cron.CronScheduler`) para disparar tarefas recorrentes associadas a agentes específicos e workspaces.

### Fluxo de Funcionamento do Agendador
1. **Configuração**: O desenvolvedor cadastra uma expressão cron padrão de 6 campos com suporte a segundos (ex: `0 0 * * * *` para rodar de hora em hora).
2. **Ciclo de Vida**: O agendador roda em uma goroutine background duradoura e utiliza timers com base na precisão da biblioteca `robfig/cron/v3`.
3. **Disparo de Agente**: Ao bater o horário, o agendador dispara a função callback associada, que executa o `ExecuteTask` do agente correspondente em uma goroutine segura.

### Exemplo Completo de Integração de Cron e SDK

Abaixo está um exemplo funcional combinando o SDK de agentes e o agendador de tarefas recorrentes em Go:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/crom/crom-agente/internal/cron"
	"github.com/crom/crom-agente/pkg/config"
	"github.com/crom/crom-agente/pkg/sdk"
)

func main() {
	// 1. Inicializa o SDK Manager
	sdkConfig := config.Config{
		WorkspaceRoot:  "/home/j/Área de trabalho/GitHub/meu-projeto",
		StoragePath:    "/home/j/Área de trabalho/GitHub/meu-projeto/.crom",
		AllowSystem:    false,
		PermissionMode: config.ModeTotalAccess, // Para automações cron em background, total access é recomendado
	}
	manager := sdk.NewManager(sdkConfig)

	// 2. Cria o agente
	agentConfig := sdk.AgentConfig{
		AgentID:     "analista-seguranca",
		Provider:    "google",
		Model:       "gemini-1.5-pro",
		Temperature: 0.1,
		SystemRules: []string{"Foque na análise de segurança e conformidade."},
	}
	agent, err := manager.CreateAgent(agentConfig)
	if err != nil {
		log.Fatalf("Erro ao criar agente: %v", err)
	}

	// 3. Inicializa o agendador do Orquestrador
	scheduler := cron.NewCronScheduler()
	scheduler.Start()
	defer scheduler.Stop()

	// 4. Agenda a tarefa para rodar a cada 30 minutos
	// Formato: segundo minuto hora dia_mes mes dia_semana
	err = scheduler.AddJob("scan-diario", "0 */30 * * * *", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		log.Println("[Cron] Iniciando tarefa agendada de auditoria de segurança...")
		result, err := agent.ExecuteTask(ctx, "Verifique se novos arquivos foram commitados com chaves expostas ou dependências vulneráveis.")
		if err != nil {
			log.Printf("[Cron] Erro ao executar varredura: %v", err)
			return
		}
		log.Printf("[Cron] Varredura concluída. Status: %s. Sumário: %s", result.Status, result.Summary)
	})
	if err != nil {
		log.Fatalf("Falha ao agendar tarefa: %v", err)
	}

	// Mantém o programa principal ativo
	log.Println("Agendador Cron ativo e aguardando disparos...")
	select {}
}
```

### Casos de Uso Comuns para Cronjobs:
- **Auditoria de Código Periódica**: Analisar no fim do dia novas alterações que possam ter gerado problemas de lint ou complexidade.
- **Limpeza de Logs e Artefatos**: Agentes configurados para expirar logs e consolidar arquivos de memória a cada semana.
- **Relatório de Progresso**: Rodar testes unitários periodicamente e gerar um arquivo markdown consolidado de sanidade do sistema.

