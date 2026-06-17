package security

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// RenderDiff calcula o diff entre oldContent e newContent e retorna uma representação colorida em terminal (ANSI)
func RenderDiff(filename string, oldContent, newContent string) string {
	dmp := diffmatchpatch.New()

	// dmp.DiffMain calcula diffs no nível de caractere.
	// Para obter um diff de linhas mais limpo, usamos o helper de linhas.
	wOld, wNew, lineArray := dmp.DiffLinesToChars(oldContent, newContent)
	diffs := dmp.DiffMain(wOld, wNew, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n\x1b[1m\x1b[34m--- DIFF ZONE: %s ---\x1b[0m\n", filename))

	for _, diff := range diffs {
		lines := strings.Split(diff.Text, "\n")
		// Se a última linha for vazia devido ao split, removemos para evitar newline extra
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		for _, line := range lines {
			switch diff.Type {
			case diffmatchpatch.DiffDelete:
				// Vermelho para exclusões
				sb.WriteString(fmt.Sprintf("\x1b[31m- %s\x1b[0m\n", line))
			case diffmatchpatch.DiffInsert:
				// Verde para inclusões
				sb.WriteString(fmt.Sprintf("\x1b[32m+ %s\x1b[0m\n", line))
			case diffmatchpatch.DiffEqual:
				// Branco/padrão para inalterados (mostrando apenas contexto se for muito grande)
				sb.WriteString(fmt.Sprintf("  %s\n", line))
			}
		}
	}

	sb.WriteString("\x1b[1m\x1b[34m---------------------------------\x1b[0m\n")
	return sb.String()
}
