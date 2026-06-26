package core

import (
	"github.com/crom/crom-agente/internal/llm"
)

// DetectRepetitiveLoop verifica se as últimas mensagens do assistant indicam um loop repetitivo.
// Compara assinaturas que incluem tanto texto quanto tool calls, capturando repetições consecutivas (A->A) e oscilações (A->B->A->B).
func DetectRepetitiveLoop(messages []llm.Message) bool {
	// Coleta as assinaturas das últimas mensagens do assistant (da mais recente para a mais antiga)
	var assistantSigs []string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			sig := assistantSignature(messages[i])
			if sig != "" {
				assistantSigs = append(assistantSigs, sig)
			}
		}
	}

	if len(assistantSigs) < 2 {
		return false
	}

	// 1. Detecção de repetição consecutiva direta (A -> A)
	if assistantSigs[0] == assistantSigs[1] {
		return true
	}

	// 2. Detecção de oscilação repetida (A -> B -> A -> B)
	if len(assistantSigs) >= 4 {
		if assistantSigs[0] == assistantSigs[2] && assistantSigs[1] == assistantSigs[3] {
			return true
		}
	}

	return false
}

// assistantSignature gera uma string de assinatura para uma mensagem do assistant,
// combinando texto e tool calls para permitir comparação de repetição.
func assistantSignature(msg llm.Message) string {
	sig := msg.Content
	for _, tc := range msg.ToolCalls {
		sig += "|" + tc.Function.Name + ":" + tc.Function.Arguments
	}
	return sig
}
