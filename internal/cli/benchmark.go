package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	benchType      string
	benchModel     string
	benchProvider  string
	benchLimit     int
	benchMaxIter   int
	benchTimeout   int
	benchMock             bool
	benchOutputDir        string
	benchTemp             float64
	benchTopP             float64
	benchMaxContextTokens int
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Executa testes de benchmark contra o agente",
	Long: `Gerencia a execução e análise de benchmarks (como SWE-bench, Terminal-Bench, 
LiveCodeBench, EvalPlus e BigCodeBench) medindo a taxa de acerto do agente e gerando 
índices de eficiência DeepSWE e Kilo Bench de custo-benefício.`,
}

var benchmarkRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Inicia a bateria de testes de um benchmark específico",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Localiza a raiz do projeto e o script benchmark/main.py
		scriptPath := filepath.Join(workspacePath, "benchmark", "main.py")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return fmt.Errorf("script de benchmark não encontrado no caminho: %s. Certifique-se de estar rodando na raiz do projeto", scriptPath)
		}

		// Garante que o python3 está instalado no host
		if _, err := exec.LookPath("python3"); err != nil {
			return fmt.Errorf("o comando 'python3' é necessário para rodar os benchmarks mas não foi encontrado no sistema")
		}

		// Monta os argumentos para rodar o script python
		pyArgs := []string{scriptPath, "run", "--benchmark", benchType}

		if benchProvider != "" {
			pyArgs = append(pyArgs, "--provider", benchProvider)
		}
		if benchModel != "" {
			pyArgs = append(pyArgs, "--model", benchModel)
		}
		if benchLimit > 0 {
			pyArgs = append(pyArgs, "--limit", strconv.Itoa(benchLimit))
		}
		if benchMaxIter > 0 {
			pyArgs = append(pyArgs, "--max-iterations", strconv.Itoa(benchMaxIter))
		}
		if benchTimeout > 0 {
			pyArgs = append(pyArgs, "--timeout", strconv.Itoa(benchTimeout))
		}
		if benchMock {
			pyArgs = append(pyArgs, "--mock-agent")
		}
		if benchOutputDir != "" {
			pyArgs = append(pyArgs, "--output-dir", benchOutputDir)
		}
		if benchTemp > 0.0 {
			pyArgs = append(pyArgs, "--temp", fmt.Sprintf("%f", benchTemp))
		}
		if benchTopP > 0.0 {
			pyArgs = append(pyArgs, "--top-p", fmt.Sprintf("%f", benchTopP))
		}
		if benchMaxContextTokens > 0 {
			pyArgs = append(pyArgs, "--max-context-tokens", strconv.Itoa(benchMaxContextTokens))
		}

		cmd.Printf("⚡ Executando wrapper Python do benchmark em: %s...\n", scriptPath)
		
		// Executa redirecionando stdout e stderr para o console do usuário
		execCmd := exec.Command("python3", pyArgs...)
		execCmd.Stdout = cmd.OutOrStdout()
		execCmd.Stderr = cmd.OutOrStderr()
		execCmd.Stdin = os.Stdin

		// Mantém variáveis de ambiente do sistema, incluindo chaves de API
		execCmd.Env = os.Environ()

		if err := execCmd.Run(); err != nil {
			return fmt.Errorf("falha ao rodar o benchmark: %w", err)
		}

		return nil
	},
}

var benchmarkCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Gera relatórios de comparação de modelos agregando os resultados salvos",
	RunE: func(cmd *cobra.Command, args []string) error {
		scriptPath := filepath.Join(workspacePath, "benchmark", "main.py")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return fmt.Errorf("script de benchmark não encontrado: %s", scriptPath)
		}

		pyArgs := []string{scriptPath, "compare"}
		if benchOutputDir != "" {
			pyArgs = append(pyArgs, "--output-dir", benchOutputDir)
		}

		cmd.Println("📊 Agregando resultados e compilando relatórios compostos...")
		execCmd := exec.Command("python3", pyArgs...)
		execCmd.Stdout = cmd.OutOrStdout()
		execCmd.Stderr = cmd.OutOrStderr()
		
		if err := execCmd.Run(); err != nil {
			return fmt.Errorf("falha ao gerar comparação composta: %w", err)
		}

		return nil
	},
}

func init() {
	benchmarkRunCmd.Flags().StringVar(&benchType, "type", "evalplus", "Tipo do benchmark (swe-bench|terminal-bench|livecode-bench|evalplus|bigcodebench)")
	benchmarkRunCmd.Flags().StringVar(&benchProvider, "provider", "", "Override de provedor de LLM (openai|gemini|anthropic|ollama|openrouter)")
	benchmarkRunCmd.Flags().StringVar(&benchModel, "model", "", "Override de modelo de LLM (ex: gpt-4o)")
	benchmarkRunCmd.Flags().IntVar(&benchLimit, "limit", 5, "Limite de instâncias a executar")
	benchmarkRunCmd.Flags().IntVar(&benchMaxIter, "max-iterations", 0, "Override de turnos ReAct máximos")
	benchmarkRunCmd.Flags().IntVar(&benchTimeout, "timeout", 0, "Override de timeout em segundos por tarefa")
	benchmarkRunCmd.Flags().BoolVar(&benchMock, "mock", false, "Usa simulação rápida mock sem consumir API")
	benchmarkRunCmd.Flags().StringVar(&benchOutputDir, "output-dir", "", "Diretório alternativo para salvar relatórios")
	benchmarkRunCmd.Flags().Float64Var(&benchTemp, "temp", 0.0, "Temperatura do modelo")
	benchmarkRunCmd.Flags().Float64Var(&benchTopP, "top-p", 0.0, "Top-P do modelo")
	benchmarkRunCmd.Flags().IntVar(&benchMaxContextTokens, "max-context-tokens", 0, "Máximo de tokens de contexto")

	benchmarkCompareCmd.Flags().StringVar(&benchOutputDir, "output-dir", "", "Diretório contendo relatórios JSON")

	benchmarkCmd.AddCommand(benchmarkRunCmd, benchmarkCompareCmd)
	rootCmd.AddCommand(benchmarkCmd)
}
