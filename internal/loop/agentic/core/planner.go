package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/state"
)

// PlannerLoop representa a camada superior de raciocínio (Fase 2)
// Ele não executa ferramentas que modificam o sistema. Ele delega para o AgenticLoop (Executor).
type PlannerLoop struct {
	executor *AgenticLoop
}

func NewPlannerLoop(executor *AgenticLoop) *PlannerLoop {
	return &PlannerLoop{
		executor: executor,
	}
}

// Execute assume o controle da tarefa de alto nível
func (p *PlannerLoop) Execute(ctx context.Context, intent string) error {
	p.executor.handler.OnStatusChange("planning")
	p.executor.handler.OnMessage("system", "Iniciando modo de Decomposição Estrutural (Planner-Executor).")

	sm := p.executor.stateManager
	if sm != nil {
		_ = sm.SetCognitiveMode(state.ModoPlanning)
	}

	// 1. O Planner avalia a intenção e quebra em uma lista inicial de Tasks.
	toolsInstructions := prompting.BuildToolsInstructions(p.executor.promptManager, p.executor.GetTools())
	planPrompt := fmt.Sprintf("Como um arquiteto pragmático, decomponha a seguinte intenção do usuário em tarefas atômicas estritamente sequenciais. NUNCA invente etapas mentais, passos de 'autenticação' ou over-engineering corporativo. Cada tarefa deve ser algo resolvível diretamente com as ferramentas de sistema (bash, leitura/escrita de arquivo, busca). Limite o plano ao mínimo essencial (máximo 4 tarefas). Retorne APENAS um JSON array de strings, onde cada string é uma task técnica clara e acionável mapeada para uma ferramenta real.\n%s\n\nIntenção: %s", toolsInstructions, intent)

	// Usar o provider do executor (sem tools para o planner forçar saída JSON pura)
	opts := llm.RequestOptions{
		MaxTokens: func() *int { t := 1000; return &t }(),
	}

	resp, err := p.executor.provider.SendMessages(ctx, []llm.Message{{Role: "user", Content: planPrompt}}, opts)
	if err != nil {
		return fmt.Errorf("falha no planner ao decompor tarefa: %w", err)
	}

	// Limpar possíveis blocos markdown (ex: ```json ... ```)
	content := resp.Message.Content
	startIdx := strings.Index(content, "[")
	endIdx := strings.LastIndex(content, "]")
	if startIdx != -1 && endIdx != -1 && endIdx >= startIdx {
		content = content[startIdx : endIdx+1]
	}

	var taskStrings []string
	if err := json.Unmarshal([]byte(content), &taskStrings); err != nil {
		return fmt.Errorf("falha ao parsear tarefas do planner (saída: %s): %w", content, err)
	}

	var plan []state.TaskItem
	for _, ts := range taskStrings {
		plan = append(plan, state.TaskItem{
			Title:  ts,
			Status: "pending",
		})
	}

	p.executor.handler.OnMessage("system", fmt.Sprintf("Plano gerado com %d tarefas. Iniciando execução iterativa.", len(plan)))

	if sm != nil {
		_ = sm.SetPlan(plan)
		_ = sm.SetCognitiveMode(state.ModoExecuting)
	}

	p.executor.handler.OnStatusChange("executing_subtask")

	for i, task := range plan {
		p.executor.handler.OnMessage("system", fmt.Sprintf("Executando Tarefa %d/%d: %s", i+1, len(plan), task.Title))

		// Marca como in_progress
		if sm != nil {
			plan[i].Status = "in_progress"
			_ = sm.SetPlan(plan)
		}

		// Executa a tarefa usando o executor "cego" com blindagem
		taskIntent := fmt.Sprintf(`Tarefa atual: %s

Contexto original: %s

[REGRA ESTRITA ANTI-BYPASS]
Para concluir esta tarefa, você DEVE obrigatoriamente utilizar a ferramenta/nativo adequado.
Respostas puramente textuais simulando que você "já concluiu" a tarefa sem invocar ferramentas serão consideradas FALHAS e abortarão o processo.`, task.Title, intent)
		err := p.executor.executeCoreLoop(ctx, taskIntent)

		if sm != nil {
			if err != nil {
				plan[i].Status = "failed"
				_ = sm.SetPlan(plan)
				return fmt.Errorf("falha ao executar tarefa '%s': %w", task.Title, err)
			}
			plan[i].Status = "completed"
			_ = sm.SetPlan(plan)
		}
	}

	p.executor.handler.OnMessage("system", "Decomposição Estrutural e execução concluídas com sucesso.")
	return nil
}
