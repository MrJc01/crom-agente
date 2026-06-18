package parser

import (
	"testing"
)

func TestParseCSV_Basic(t *testing.T) {
	input := "name,age,city\nAlice,30,SP\nBob,25,RJ"
	result := ParseCSV(input)

	if len(result) != 3 {
		t.Errorf("esperava 3 linhas, obteve %d", len(result))
	}
	if len(result[0]) != 3 {
		t.Errorf("esperava 3 colunas, obteve %d", len(result[0]))
	}
	if result[1][0] != "Alice" {
		t.Errorf("esperava 'Alice', obteve %q", result[1][0])
	}
}

func TestCountRows_ExcludesHeader(t *testing.T) {
	input := "name,age\nAlice,30\nBob,25\nCharlie,35"
	count := CountRows(input)

	// Deve contar apenas linhas de dados (sem header)
	if count != 3 {
		t.Errorf("esperava 3 linhas de dados, obteve %d", count)
	}
}

func TestFindValue_CorrectPosition(t *testing.T) {
	data := [][]string{
		{"name", "age", "city"},
		{"Alice", "30", "SP"},
		{"Bob", "25", "RJ"},
	}

	row, col := FindValue(data, "Bob")

	if row != 2 {
		t.Errorf("esperava linha 2, obteve %d", row)
	}
	if col != 0 {
		t.Errorf("esperava coluna 0, obteve %d", col)
	}
}

func TestParseCSV_EmptyLines(t *testing.T) {
	input := "name,age\nAlice,30\n\nBob,25"
	result := ParseCSV(input)

	// Linhas vazias não deveriam ser incluídas como dados
	dataRows := 0
	for _, row := range result {
		if len(row) > 0 && row[0] != "" {
			dataRows++
		}
	}
	if dataRows != 3 { // header + 2 dados
		t.Errorf("esperava 3 linhas não-vazias, obteve %d", dataRows)
	}
}

func TestParseCSV_CRLF(t *testing.T) {
	input := "name,age\r\nAlice,30\r\nBob,25"
	result := ParseCSV(input)

	if result[1][1] != "30" {
		t.Errorf("esperava '30' sem \\r, obteve %q", result[1][1])
	}
}
