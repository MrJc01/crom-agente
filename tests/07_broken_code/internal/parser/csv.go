package parser

import (
	"strings"
)

// ParseCSV analisa uma string CSV e retorna uma matriz de strings.
// BUG: Não trata aspas, campos com vírgula dentro, ou linhas vazias.
func ParseCSV(input string) [][]string {
	var result [][]string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		// BUG: Não remove \r de CRLF
		fields := strings.Split(line, ",")

		// BUG: Adiciona linhas vazias ao resultado
		result = append(result, fields)
	}
	return result
}

// CountRows retorna o número de linhas de dados (excluindo header).
// BUG: Off-by-one — não subtrai 1 para o header
func CountRows(input string) int {
	lines := strings.Split(input, "\n")
	return len(lines) // BUG: deveria ser len(lines) - 1 para excluir o header
}

// GetColumn extrai uma coluna pelo índice.
// BUG: Não valida se o índice está dentro dos limites
func GetColumn(data [][]string, col int) []string {
	var result []string
	for _, row := range data {
		// BUG: Panic se col >= len(row)
		result = append(result, row[col])
	}
	return result
}

// FindValue busca um valor na tabela e retorna a posição [linha, coluna].
// BUG: Retorna posição errada (troca linha por coluna)
func FindValue(data [][]string, target string) (int, int) {
	for i, row := range data {
		for j, cell := range row {
			if strings.TrimSpace(cell) == target {
				return j, i // BUG: j,i trocados — deveria ser i,j
			}
		}
	}
	return -1, -1
}
