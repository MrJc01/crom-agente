package core

import (
	"encoding/json"

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

	// 1. Detecção de repetição consecutiva direta (A -> A -> A)
	if len(assistantSigs) >= 3 {
		if assistantSigs[0] == assistantSigs[1] && assistantSigs[1] == assistantSigs[2] {
			return true
		}
	}

	// 2. Detecção de oscilação repetida (A -> B -> A -> B)
	if len(assistantSigs) >= 4 {
		if assistantSigs[0] == assistantSigs[2] && assistantSigs[1] == assistantSigs[3] {
			return true
		}
	}

	return false
}

// DetectRepetitiveWarning verifica se as últimas 2 assinaturas do assistant são idênticas (A -> A).
func DetectRepetitiveWarning(messages []llm.Message) bool {
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

	return assistantSigs[0] == assistantSigs[1]
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

// DetectIneffectiveCorrectionLoop verifica se o assistente está tentando corrigir o mesmo arquivo/linha com o mesmo erro 3 vezes seguidas.
func DetectIneffectiveCorrectionLoop(messages []llm.Message) bool {
	type attempt struct {
		file string
		err  string
	}
	var attempts []attempt

	for i := 0; i < len(messages)-1; i++ {
		msg := messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var editedFile string
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "write_file" || tc.Function.Name == "edit_file" || tc.Function.Name == "autofix" {
					var args struct {
						Path string `json:"path"`
					}
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					if args.Path != "" {
						editedFile = args.Path
					}
				}
			}

			if editedFile != "" {
				var testErr string
				for j := i + 1; j < len(messages); j++ {
					nextMsg := messages[j]
					if nextMsg.Role == "tool" && (nextMsg.Name == "run_tests" || nextMsg.Name == "autofix") {
						testErr = nextMsg.Content
						break
					}
					if nextMsg.Role == "assistant" {
						break
					}
				}
				if testErr != "" {
					attempts = append(attempts, attempt{file: editedFile, err: testErr})
				}
			}
		}
	}

	if len(attempts) < 3 {
		return false
	}

	lastIdx := len(attempts) - 1
	a1 := attempts[lastIdx]
	a2 := attempts[lastIdx-1]
	a3 := attempts[lastIdx-2]

	e1 := simplifyError(a1.err)
	e2 := simplifyError(a2.err)
	e3 := simplifyError(a3.err)

	return a1.file == a2.file && a2.file == a3.file && e1 == e2 && e2 == e3 && e1 != ""
}

func simplifyError(err string) string {
	if len(err) > 200 {
		return err[:200]
	}
	return err
}
