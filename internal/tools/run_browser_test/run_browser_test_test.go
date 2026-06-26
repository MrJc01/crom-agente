package run_browser_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlaybookSteps(t *testing.T) {
	steps := getPlaybookSteps("login")
	if len(steps) != 5 {
		t.Fatalf("esperava 5 passos no login, obteve: %d", len(steps))
	}

	if steps[0].Action != "navigate" || steps[0].URL != "http://localhost:8080/login" {
		t.Errorf("primeiro passo inválido: %+v", steps[0])
	}
}

func TestCompareImages(t *testing.T) {
	// Cria 2 imagens de 10x10 idênticas
	img1 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img2 := image.NewRGBA(image.Rect(0, 0, 10, 10))

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img1.Set(x, y, color.RGBA{255, 0, 0, 255})
			img2.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	var buf1, buf2 bytes.Buffer
	_ = png.Encode(&buf1, img1)
	_ = png.Encode(&buf2, img2)

	sim, err := compareImages(buf1.Bytes(), buf2.Bytes())
	if err != nil {
		t.Fatalf("erro ao comparar imagens idênticas: %v", err)
	}
	if sim != 100.0 {
		t.Errorf("esperava 100%% de similaridade, obteve: %.2f%%", sim)
	}

	// Altera 1 pixel na img2
	img2.Set(0, 0, color.RGBA{0, 255, 0, 255})
	buf2.Reset()
	_ = png.Encode(&buf2, img2)

	sim, err = compareImages(buf1.Bytes(), buf2.Bytes())
	if err != nil {
		t.Fatalf("erro ao comparar imagens: %v", err)
	}
	if sim != 99.0 {
		t.Errorf("esperava 99%% de similaridade, obteve: %.2f%%", sim)
	}
}

func TestRunBrowserTestTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><title>Test Page</title></head>
			<body>
				<h1>Welcome</h1>
				<button id="btn">Click me</button>
				<img src="dummy.png" />
				<input type="text" id="input1" />
			</body>
			</html>
		`))
	}))
	defer server.Close()

	tool := NewRunBrowserTestTool(t.TempDir(), true)

	// Custom steps simulating actions
	input := RunBrowserTestInput{
		Steps: []TestStep{
			{Action: "navigate", URL: server.URL},
			{Action: "assert_visible", Selector: "h1"},
			{Action: "assert_text", Selector: "h1", Text: "Welcome"},
			{Action: "check_accessibility"},
		},
	}

	args, _ := json.Marshal(input)
	res, err := tool.Execute(context.Background(), args)
	
	// Se o navegador não estiver instalado/disponível, tratamos o erro de forma suave
	if err != nil {
		t.Skipf("Navegador não disponível no ambiente de testes (pulando execução rod): %v", err)
		return
	}

	if !res.Success {
		t.Fatalf("teste de browser falhou: %v, data: %s", err, res.Data)
	}
}
