package stress

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestMemoryConsumption Mocka o consumo de RAM simulando N Agentes simultaneos
func TestMemoryConsumption(t *testing.T) {
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	numAgents := 10
	var wg sync.WaitGroup
	wg.Add(numAgents)

	t.Logf("Iniciando %d Agentes Simulados para medir RAM...", numAgents)

	for i := 0; i < numAgents; i++ {
		go func(id int) {
			defer wg.Done()
			// Simulacao de operacao do agente: geracao de arrays de contexto pesados
			contextTokens := make([][]byte, 0)
			for j := 0; j < 500; j++ {
				// Simula contexto de 1KB por passo
				contextTokens = append(contextTokens, make([]byte, 1024))
			}
			time.Sleep(200 * time.Millisecond) // Simula IO de LLM Local
			_ = len(contextTokens)
		}(i)
	}

	wg.Wait()
	runtime.ReadMemStats(&m2)

	memUsedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memoria Alocada após simulação: %.2f MB", memUsedMB)

	if memUsedMB > 50.0 {
		t.Errorf("FAIL: O consumo de RAM por 10 agentes ultrapassou 50MB (Usado: %.2f MB)", memUsedMB)
	} else {
		t.Logf("SUCCESS: Consumo de RAM altamente otimizado (<50MB).")
	}
}
