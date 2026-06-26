package finalizer

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
		panic("falha ao carregar metadados do finalizer_agent: " + err.Error())
	}

	// Registro automático do agente
	agents.RegisterAgent("finalizer", func(cfg agents.Config) core.Agent {
		return NewFinalizerAgent(cfg)
	})
}

// FinalizerAgent gera a resposta explicativa final e consolidada ao término das tarefas
type FinalizerAgent struct {
	core.BaseAgent
	workspacePath string
}

// NewFinalizerAgent cria um novo FinalizerAgent
func NewFinalizerAgent(cfg agents.Config) *FinalizerAgent {
	fa := &FinalizerAgent{
		workspacePath: cfg.WorkspacePath,
	}
	fa.AgentName = "finalizer"
	fa.AgentDescription = metadata.Description
	fa.LLMProvider = cfg.LLMProvider
	fa.AllowedToolIDs = []string{} // No-tools
	return fa
}

// Name retorna o nome do especialista
func (f *FinalizerAgent) Name() string {
	return f.AgentName
}

// Description retorna a descrição
func (f *FinalizerAgent) Description() string {
	return f.AgentDescription
}

// SystemPrompt retorna o prompt de sistema
func (f *FinalizerAgent) SystemPrompt() string {
	return f.AgentSysPrompt
}

// Execute executa o agente finalizador para gerar a resposta consolidada
func (f *FinalizerAgent) Execute(ctx context.Context, prompt string, priorSummary string) (core.AgentResult, error) {
	systemPrompt := `Você é o Finalizer Agent, especialista em consolidação de tarefas concluídas e comunicação estruturada.
Sua missão é analisar o histórico de ações que foram realizadas e formular uma resposta de encerramento amigável, clara, muito bem organizada e autoexplicativa sobre a conclusão do trabalho.

Diretrizes obrigatórias para sua resposta:
1. Comece de forma amigável informando o que foi concluído (ex: "Terminei o...", "Concluí o...").
2. Liste de forma organizada e estruturada (usando tópicos/bullet points) o que foi feito (quais arquivos foram criados/modificados/deletados, comandos executados, etc.).
3. Explique onde o usuário pode encontrar as alterações no projeto (por exemplo, no painel Explorer à esquerda ou em um arquivo específico).
4. Utilize links markdown no formato correto de arquivo local se mencionar arquivos (ex: [Nome do Arquivo](file:///caminho/completo/do/arquivo)).
5. Seja natural, profissional e muito claro. Não invente ações que não estejam explícitas no histórico fornecido.`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}

	var telemetryCallback func(loop.AgentEvent)
	if cb, ok := ctx.Value("telemetry_callback").(func(loop.AgentEvent)); ok {
		telemetryCallback = cb
	}

	if telemetryCallback != nil {
		telemetryCallback(loop.AgentEvent{
			Timestamp: time.Now(),
			Event:     "thinking",
			Iteration: 1,
			Data: map[string]interface{}{
				"agent": "finalizer",
			},
		})
	}

	opts := llm.RequestOptions{
		ToolChoice: "none",
	}

	resp, err := f.LLMProvider.SendMessages(ctx, messages, opts)
	if err != nil {
		return core.AgentResult{}, fmt.Errorf("finalizer falhou ao gerar resposta: %w", err)
	}

	finalOutput := resp.Message.Content

	return core.AgentResult{
		Success: true,
		Output:  finalOutput,
	}, nil
}
