package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/loop/agentic/core"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/registry"
)

type TestResult struct {
	Success bool
	Reason  string
	Tasks   int
}

func main() {
	models := []string{
		"meta-llama/llama-3.1-8b-instruct",
		"meta-llama/llama-3.2-3b-instruct",
	}

	intents := []string{
		"Escreva a string 'Bateria de Testes' na Core Memory usando a ferramenta apropriada.",
		"Use a ferramenta de matemática ou bash para calcular 25 * 4 e armazene o resultado na Core Memory.",
		"Liste os arquivos do diretório atual usando comandos bash.",
		"Adicione um novo fato no Knowledge Graph sobre 'O usuário adora testar modelos de IA'.",
		"Escreva uma nota 'Testando o sistema OS' em um arquivo temporário '/tmp/crom_teste.txt'.",
		"Leia o conteúdo do arquivo temporário '/tmp/crom_teste.txt' que você acabou de criar.",
		"Crie um diretório chamado 'crom_stress' na pasta /tmp e liste o conteúdo da pasta /tmp.",
		"Busque na memória se há algo escrito sobre 'Bateria de Testes' ou 'Testando'.",
		"Simule um erro intencional tentando usar uma ferramenta que não existe chamada 'ferramenta_falsa_123'. Você deve notar que ela falha.",
		"Escreva 'FIM DOS TESTES' na Core Memory para encerrar a bateria com chave de ouro.",
	}

	for _, modelName := range models {
		fmt.Printf("\n======================================================\n")
		fmt.Printf("Iniciando Bateria Stress com Modelo: %s\n", modelName)
		fmt.Printf("======================================================\n")

		score := 0
		
		for i, intent := range intents {
			fmt.Printf("\n[Teste %d/10] Intent: %s\n", i+1, 10, intent)
			
			// Setup
			cfg := &config.ResolvedConfig{
				Provider:      "openrouter",
				Model:         modelName,
				MaxIterations: 4,
				MaxConsecutiveFail: 3,
				ToolTimeoutSeconds:      60,
				CognitiveArchitecture: config.CognitiveArchitecture{
					MemoryStyle:             "os_style",
					StructuralDecomposition: false,
					KnowledgeGraph: config.KnowledgeGraphConfig{
						Enabled:     true,
						StorageType: "json_file",
					},
				},
			}

			apiKey := os.Getenv("OPENROUTER_API_KEY")
			provider := providers.NewOpenAIProvider(apiKey, modelName)
			provider.URL = "https://openrouter.ai/api/v1/chat/completions"
			
			if provider == nil {
				fmt.Printf("[ERRO] Falha ao criar provider\n")
				return
			}

			workspaceDir := "/tmp/crom-test-" + strings.ReplaceAll(modelName, ":", "-") + fmt.Sprintf("-t%d", i)
			os.MkdirAll(workspaceDir, 0755)

			sm := state.NewStateManager(workspaceDir)
			if sm == nil {
				fmt.Printf("[ERRO] Falha ao criar state\n")
				return
			}

			executor := core.New(provider, sm, nil, cfg)
			
			// Registrar todas as ferramentas nativas
			builtinTools := registry.GetBuiltinTools(registry.RegistrationConfig{
				WorkspacePath: workspaceDir,
			})
			for _, t := range builtinTools {
				executor.RegisterTool(t)
			}
			executor.RegisterTool(tools.NewCoreMemoryAppendTool(sm))
			executor.RegisterTool(tools.NewCoreMemoryReplaceTool(sm))
			executor.RegisterTool(tools.NewCoreMemorySearchTool(sm))

			start := time.Now()
			err := executor.Execute(context.Background(), intent)
			duration := time.Since(start)

			if err != nil {
				fmt.Printf("❌ [FALHA] Tempo: %v | Erro: %v\n", duration, err)
			} else {
				fmt.Printf("✅ [SUCESSO] Tempo: %v\n", duration)
				score++
			}
			
			plan := sm.GetPlan()
			if len(plan) > 0 {
				fmt.Printf("  Plano Gerado: %d tarefas\n", len(plan))
				for _, t := range plan {
					fmt.Printf("    - [%s] %s\n", t.Status, t.Title)
				}
			}
		}
		
		fmt.Printf("\n>>> Resultado Final do %s: %d/10 <<<\n", modelName, score)
	}
}
