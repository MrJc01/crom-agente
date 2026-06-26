package cli

import (
        "fmt"
        "os"
        "os/exec"

        "github.com/spf13/cobra"
)

var (
        suiteFlag string
)

func init() {
        benchCmd.Flags().StringVar(&suiteFlag, "suite", "swe", "Benchmark suite to run (e.g., swe, humaneval, mbpp, mbbp)")
        rootCmd.AddCommand(benchCmd)
}

var benchCmd = &cobra.Command{
        Use:   "bench",
        Short: "Executa os benchmarks do agente (aos poucos substituindo o script Python)",
        RunE: func(cmd *cobra.Command, args []string) error {
                fmt.Printf("🚀 Iniciando benchmark suite: %s\n", suiteFlag)
                
                // TODO: no futuro, migrar lógica do run_all.py inteiramente para o Go.
                // Por agora, para manter compatibilidade e não quebrar, chamamos o runner python filtrando
                
                pythonArgs := []string{"benchmark/run_all.py"}
                
                // Mapear suite (por enquanto não temos flag específica no run_all.py para rodar só uma suite sem alterar o código,
                // mas podemos passar flags no futuro).
                
                c := exec.Command("python3", pythonArgs...)
                c.Stdout = os.Stdout
                c.Stderr = os.Stderr
                c.Stdin = os.Stdin
                
                if err := c.Run(); err != nil {
                        return fmt.Errorf("erro ao rodar benchmark python: %v", err)
                }
                
                return nil
        },
}
