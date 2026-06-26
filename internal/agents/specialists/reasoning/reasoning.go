package reasoning

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/crom/crom-agente/internal/agents"
	"github.com/crom/crom-agente/internal/agents/core"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/loop"
	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados do reasoning_agent: " + err.Error())
	}

	// Registro automático do agente
	agents.RegisterAgent("reasoning", func(cfg agents.Config) core.Agent {
		return NewReasoningAgent(cfg)
	})
}

// ReasoningAgent executa loops puramente cognitivos sem ferramentas
type ReasoningAgent struct {
	core.BaseAgent
	workspacePath string
}

// NewReasoningAgent cria um novo ReasoningAgent
func NewReasoningAgent(cfg agents.Config) *ReasoningAgent {
	ra := &ReasoningAgent{
		workspacePath: cfg.WorkspacePath,
	}
	ra.AgentName = "reasoning"
	ra.AgentDescription = metadata.Description
	ra.LLMProvider = cfg.LLMProvider
	ra.AllowedToolIDs = []string{} // No-tools
	return ra
}

// Name retorna o nome do especialista
func (r *ReasoningAgent) Name() string {
	return r.AgentName
}

// Description retorna a descrição
func (r *ReasoningAgent) Description() string {
	return r.AgentDescription
}

// SystemPrompt retorna o prompt
func (r *ReasoningAgent) SystemPrompt() string {
	return r.AgentSysPrompt
}

// Execute executa o loop cognitivo sequencial
func (r *ReasoningAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	// Padrão de 5 rodadas cognitivas
	iterations := 5
	cleanedPrompt := prompt

	// Permite especificar opcionalmente a quantidade de rodadas via prefixo [RODADAS: N]
	var parsedIterations int
	if _, err := fmt.Sscanf(prompt, "[RODADAS: %d]", &parsedIterations); err == nil && parsedIterations > 0 {
		iterations = parsedIterations
		prefix := fmt.Sprintf("[RODADAS: %d]", parsedIterations)
		if len(prompt) > len(prefix) {
			cleanedPrompt = prompt[len(prefix):]
		}
	}

	systemPrompt := `Você é o Reasoning Agent, um especialista em raciocínio lógico avançado e reflexão profunda.
Sua tarefa é analisar o problema de forma extremamente sistemática, crítica e metódica através de um processo de pensamento iterativo em múltiplos passos.
Você NÃO tem acesso a ferramentas nesta fase. Concentre-se exclusivamente em analisar teorias, testar hipóteses, refutar ideias incorretas, analisar contradições e formular uma resposta final sólida.

Instruções para seu fluxo cognitivo:
- Divida o problema em partes e trace um caminho lógico.
- A cada iteração de raciocínio, revise criticamente o que pensou na rodada anterior. Tente achar falhas e refine suas conclusões.
- Descreva detalhadamente seu processo de raciocínio passo a passo na tag <pensamento>...</pensamento> a cada rodada.
- Não conclua ou dê a resposta final antes da iteração definitiva.
- Na iteração definitiva/final, consolide todos os pensamentos e forneça a conclusão clara e direta para o usuário fora das tags de pensamento.`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}

	if priorSummary != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: fmt.Sprintf("Histórico resumido das interações anteriores:\n%s", priorSummary),
		})
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: cleanedPrompt,
	})

	// Recuperar o callback de telemetria se disponível
	var telemetryCallback func(loop.AgentEvent)
	if cb, ok := ctx.Value("telemetry_callback").(func(loop.AgentEvent)); ok {
		telemetryCallback = cb
	}

	var finalOutput string

	// Executa o loop cognitivo
	for i := 0; i < iterations; i++ {
		isFinal := i == iterations-1

		var iterPrompt string
		if isFinal {
			iterPrompt = fmt.Sprintf("[RODADA DE RACIOCÍNIO %d de %d - FINAL]\nEsta é a rodada de raciocínio final. Com base em todas as reflexões anteriores, formule a conclusão consolidada definitiva para o usuário. Seja preciso e direto.", i+1, iterations)
		} else {
			iterPrompt = fmt.Sprintf("[RODADA DE RACIOCÍNIO %d de %d]\nAnalise criticamente o status atual e expanda o raciocínio. Descreva suas reflexões sob a tag <pensamento>...</pensamento>. Identifique o que falta descobrir ou refinar.", i+1, iterations)
		}

		// Copia o histórico para enviar ao LLM com a instrução específica da iteração
		iterMessages := make([]llm.Message, len(messages))
		copy(iterMessages, messages)
		iterMessages = append(iterMessages, llm.Message{
			Role:    "system",
			Content: iterPrompt,
		})

		// Chamar o LLM sem ferramentas
		opts := llm.RequestOptions{
			ToolChoice: "none",
		}
		resp, err := r.LLMProvider.SendMessages(ctx, iterMessages, opts)
		if err != nil {
			return core.AgentResult{}, fmt.Errorf("erro ao gerar pensamento na rodada %d: %w", i+1, err)
		}

		thoughtContent := resp.Message.Content

		// Emitir evento de telemetria para o frontend
		if telemetryCallback != nil {
			telemetryCallback(loop.AgentEvent{
				Timestamp: time.Now(),
				Event:     "reasoning_thinking",
				Iteration: i + 1,
				Data: map[string]interface{}{
					"agent":     "reasoning",
					"content":   thoughtContent,
					"is_final":  isFinal,
					"iteration": i + 1,
					"total":     iterations,
				},
			})
		}

		// Adiciona ao histórico para a próxima rodada
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: thoughtContent,
		})

		if isFinal {
			finalOutput = thoughtContent
		}
	}

	newSummary, _ := core.CompressHistory(ctx, r.LLMProvider, cleanedPrompt, finalOutput, priorSummary)

	return core.AgentResult{
		Success:        true,
		Output:         finalOutput,
		ContextSummary: newSummary,
	}, nil
}
