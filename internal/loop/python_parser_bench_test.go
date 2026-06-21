package loop

import (
	"testing"
)

var benchmarkOutput interface{}

func BenchmarkTryParseToolCode(b *testing.B) {
	content := "Algum texto introdutório do LLM.\n/tool_code\n```python\n# Comentário\nread_file.execute(path=\"src/main.go\")\nwrite_file.execute(\n	path=\"src/test.go\",\n	content=\"\"\"func Test() {}\"\"\",\n	overwrite=True\n)\n```\nE um pouco de texto final.\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := tryParseToolCode(content)
		benchmarkOutput = res
	}
}

func BenchmarkParsePythonKeywordArgs(b *testing.B) {
	argsStr := `path="src/test.go", content="""func Test() {}""", overwrite=True`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, _ := parsePythonKeywordArgs(argsStr)
		benchmarkOutput = res
	}
}

func BenchmarkPyExprToJSON(b *testing.B) {
	exprs := []string{
		`"uma string simples"`,
		`1234.56`,
		`True`,
		`["a", "b", 123]`,
		`{"key": "value", "num": 42}`,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, expr := range exprs {
			res, _ := pyExprToJSON(expr)
			benchmarkOutput = res
		}
	}
}

func BenchmarkSplitTopLevel(b *testing.B) {
	s := `path="src/test.go", content="func(a,b) { return a,b }", overwrite=True`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := splitTopLevel(s, ',')
		benchmarkOutput = res
	}
}
