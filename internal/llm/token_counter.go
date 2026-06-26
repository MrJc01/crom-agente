package llm

import (
	"log"

	"github.com/pkoukk/tiktoken-go"
)

// CountTokens estima a quantidade de tokens usados usando o tokenizer padrão (cl100k_base para o4/gpt-4)
func CountTokens(messages []Message, response string, provider, model string) int {
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		log.Printf("[tiktoken] falha ao carregar encoding: %v", err)
		return 0
	}

	var total int
	// Soma tokens do histórico (aproximado)
	for _, m := range messages {
		total += len(tkm.Encode(m.Content, nil, nil))
		// Adicionar overhead por mensagem
		total += 4
	}

	// Soma tokens da resposta
	total += len(tkm.Encode(response, nil, nil))

	// Overhead extra para formatar o chat
	total += 3

	return total
}
