package loop

import (
	"encoding/json"
	"testing"
)

func TestTryParseToolCode_Valid(t *testing.T) {
	content := `Aqui está o plano:
- [ ] Passo 1

/tool_code
print(browser_subagent.execute(
	task='Navegue ate tabnews.com.br, clique em Recentes, tire um screenshot e extraia os ultimos 6 posts.',
	steps=[
		{'action': 'navigate', 'url': 'https://www.tabnews.com.br/'},
		{'action': 'click', 'selector': 'a[href="/recentes"]', 'verify_contains': 'Recentes'},
		{'action': 'screenshot', 'path': 'tabnews_recentes.png', 'seconds': 2.5},
		{'action': 'get_html'}
	],
	capture_final_screenshot=False
))`

	calls := TryParseToolCode(content)
	if len(calls) != 1 {
		t.Fatalf("esperava 1 tool call, obteve %d", len(calls))
	}

	call := calls[0]
	if call.Function.Name != "browser_subagent" {
		t.Errorf("esperava nome browser_subagent, obteve %s", call.Function.Name)
	}

	// Faz parse do JSON arguments para validar
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		t.Fatalf("falha ao unmarshal arguments JSON: %v. JSON obtido: %s", err, call.Function.Arguments)
	}

	// Valida task
	task, ok := args["task"].(string)
	if !ok || task != "Navegue ate tabnews.com.br, clique em Recentes, tire um screenshot e extraia os ultimos 6 posts." {
		t.Errorf("task inválido: %v", args["task"])
	}

	// Valida capture_final_screenshot
	cfs, ok := args["capture_final_screenshot"].(bool)
	if !ok || cfs != false {
		t.Errorf("capture_final_screenshot inválido: %v", args["capture_final_screenshot"])
	}

	// Valida steps
	steps, ok := args["steps"].([]interface{})
	if !ok || len(steps) != 4 {
		t.Fatalf("steps inválido, tamanho esperado 4, obteve: %v", args["steps"])
	}

	step0 := steps[0].(map[string]interface{})
	if step0["action"] != "navigate" || step0["url"] != "https://www.tabnews.com.br/" {
		t.Errorf("step 0 inválido: %v", step0)
	}

	step1 := steps[1].(map[string]interface{})
	if step1["action"] != "click" || step1["selector"] != "a[href=\"/recentes\"]" || step1["verify_contains"] != "Recentes" {
		t.Errorf("step 1 inválido: %v", step1)
	}

	step2 := steps[2].(map[string]interface{})
	if step2["action"] != "screenshot" || step2["path"] != "tabnews_recentes.png" || step2["seconds"] != 2.5 {
		t.Errorf("step 2 inválido: %v", step2)
	}
}

func TestTryParseToolCode_NoToolCode(t *testing.T) {
	content := "Apenas conversando amigavelmente sem bloco de código."
	calls := TryParseToolCode(content)
	if len(calls) != 0 {
		t.Errorf("esperava 0 chamadas para conteúdo sem /tool_code, obteve %d", len(calls))
	}
}

func TestTryParseToolCode_InvalidSyntax(t *testing.T) {
	content := `/tool_code
browser_subagent.execute(invalid_python_code...`
	calls := TryParseToolCode(content)
	if len(calls) != 0 {
		t.Errorf("esperava 0 chamadas para código Python inválido, obteve %d", len(calls))
	}
}

func TestSplitTopLevel(t *testing.T) {
	s := "a='b,c', d=[1, 2, 3], e={'x': 1, 'y': 2}"
	parts := splitTopLevel(s, ',')
	if len(parts) != 3 {
		t.Fatalf("esperava 3 partes, obteve %d: %v", len(parts), parts)
	}
}

func TestTryParseMarkdownToolCalls(t *testing.T) {
	content := "Aqui está o primeiro arquivo, index.html:\n" +
		"```html\n" +
		"<html><body>Hello</body></html>\n" +
		"```\n\n" +
		"E aqui está o segundo script python:\n" +
		"```python\n" +
		"# FILE: scripts/hello.py\n" +
		"import os\n" +
		"print(\"hello\")\n" +
		"```\n"

	calls := TryParseMarkdownToolCalls(content)
	if len(calls) != 2 {
		t.Fatalf("esperava 2 chamadas de ferramentas extraídas, obteve %d", len(calls))
	}

	// 1. Validar index.html
	call1 := calls[0]
	if call1.Function.Name != "write_file" {
		t.Errorf("esperava write_file, obteve %s", call1.Function.Name)
	}
	var args1 map[string]string
	if err := json.Unmarshal([]byte(call1.Function.Arguments), &args1); err != nil {
		t.Fatalf("erro ao desestuturar argumentos 1: %v", err)
	}
	if args1["path"] != "index.html" {
		t.Errorf("esperava index.html, obteve %s", args1["path"])
	}
	if args1["content"] != "<html><body>Hello</body></html>" {
		t.Errorf("conteúdo incorreto: %q", args1["content"])
	}

	// 2. Validar scripts/hello.py
	call2 := calls[1]
	if call2.Function.Name != "write_file" {
		t.Errorf("esperava write_file, obteve %s", call2.Function.Name)
	}
	var args2 map[string]string
	if err := json.Unmarshal([]byte(call2.Function.Arguments), &args2); err != nil {
		t.Fatalf("erro ao desestuturar argumentos 2: %v", err)
	}
	if args2["path"] != "scripts/hello.py" {
		t.Errorf("esperava scripts/hello.py, obteve %s", args2["path"])
	}
	expectedPyContent := "import os\nprint(\"hello\")"
	if args2["content"] != expectedPyContent {
		t.Errorf("conteúdo incorreto: %q", args2["content"])
	}
}

func TestTryParseMarkdownToolCalls_UniqueFallback(t *testing.T) {
	content := "Para resolver a tarefa, precisamos atualizar o arquivo test_solution.py com a função adequada.\n" +
		"Aqui está o novo código:\n" +
		"```python\n" +
		"def solution():\n" +
		"    return True\n" +
		"```\n"

	calls := TryParseMarkdownToolCalls(content)
	if len(calls) != 1 {
		t.Fatalf("esperava 1 chamada de ferramenta extraída via fallback, obteve %d", len(calls))
	}

	call := calls[0]
	if call.Function.Name != "write_file" {
		t.Errorf("esperava write_file, obteve %s", call.Function.Name)
	}

	var args map[string]string
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		t.Fatalf("erro ao desestruturar argumentos: %v", err)
	}

	if args["path"] != "test_solution.py" {
		t.Errorf("esperava test_solution.py extraído via fallback, obteve %q", args["path"])
	}

	if args["content"] != "def solution():\n    return True" {
		t.Errorf("conteúdo incorreto: %q", args["content"])
	}
}
