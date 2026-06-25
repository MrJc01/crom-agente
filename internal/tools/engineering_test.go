package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crom/crom-agente/internal/tools/code_explainer"
	"github.com/crom/crom-agente/internal/tools/complexity_reducer"
	"github.com/crom/crom-agente/internal/tools/doc_generator"
	"github.com/crom/crom-agente/internal/tools/memory_leak_scanner"
	"github.com/crom/crom-agente/internal/tools/mock_generator"
	"github.com/crom/crom-agente/internal/tools/stack_translator"
)

// === Cap 35: Stack Translator Tests ===

func TestStackTranslator_GoToTypeScript(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "types.go")
	src := `package model

// User representa um usuário do sistema
type User struct {
	ID        int64    ` + "`json:\"id\"`" + `
	Name      string   ` + "`json:\"name\"`" + `
	Email     string   ` + "`json:\"email\"`" + `
	Active    bool     ` + "`json:\"active\"`" + `
	Tags      []string ` + "`json:\"tags,omitempty\"`" + `
	Score     float64  ` + "`json:\"score\"`" + `
	ProfileID *int     ` + "`json:\"profile_id,omitempty\"`" + `
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := stack_translator.NewStackTranslatorTool(dir, false)
	args, _ := json.Marshal(map[string]string{
		"path":            "types.go",
		"target_language": "typescript",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	// Verificar elementos TypeScript
	if !strings.Contains(result.Data, "export interface User") {
		t.Error("deveria conter 'export interface User'")
	}
	if !strings.Contains(result.Data, "id: number") {
		t.Error("deveria traduzir int64 para number")
	}
	if !strings.Contains(result.Data, "name: string") {
		t.Error("deveria traduzir string para string")
	}
	if !strings.Contains(result.Data, "active: boolean") {
		t.Error("deveria traduzir bool para boolean")
	}
	if !strings.Contains(result.Data, "tags?: string[]") {
		t.Error("deveria traduzir []string com omitempty para tags?: string[]")
	}
}

func TestStackTranslator_GoToPython(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "types.go")
	src := `package model

type Config struct {
	Port    int    ` + "`json:\"port\"`" + `
	Host    string ` + "`json:\"host\"`" + `
	Debug   bool   ` + "`json:\"debug\"`" + `
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := stack_translator.NewStackTranslatorTool(dir, false)
	args, _ := json.Marshal(map[string]string{
		"path":            "types.go",
		"target_language": "python",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "@dataclass") {
		t.Error("deveria conter @dataclass")
	}
	if !strings.Contains(result.Data, "class Config:") {
		t.Error("deveria conter class Config")
	}
	if !strings.Contains(result.Data, "port: int") {
		t.Error("deveria traduzir int para int")
	}
	if !strings.Contains(result.Data, "host: str") {
		t.Error("deveria traduzir string para str")
	}
}

func TestStackTranslator_GoToRust(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "types.go")
	src := `package model

type Item struct {
	Name  string  ` + "`json:\"name\"`" + `
	Price float64 ` + "`json:\"price\"`" + `
	Qty   int     ` + "`json:\"qty\"`" + `
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := stack_translator.NewStackTranslatorTool(dir, false)
	args, _ := json.Marshal(map[string]string{
		"path":            "types.go",
		"target_language": "rust",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "pub struct Item") {
		t.Error("deveria conter pub struct Item")
	}
	if !strings.Contains(result.Data, "String") {
		t.Error("deveria traduzir string para String")
	}
	if !strings.Contains(result.Data, "f64") {
		t.Error("deveria traduzir float64 para f64")
	}
	if !strings.Contains(result.Data, "Serialize") {
		t.Error("deveria ter derive Serialize")
	}
}

func TestStackTranslator_GoToJSONSchema(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "types.go")
	src := `package model

type Response struct {
	Status  string ` + "`json:\"status\"`" + `
	Code    int    ` + "`json:\"code\"`" + `
	Success bool   ` + "`json:\"success\"`" + `
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := stack_translator.NewStackTranslatorTool(dir, false)
	args, _ := json.Marshal(map[string]string{
		"path":            "types.go",
		"target_language": "json_schema",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, `"type": "object"`) {
		t.Error("deveria conter tipo object no schema")
	}
	if !strings.Contains(result.Data, `"status"`) {
		t.Error("deveria conter campo status")
	}
}

func TestStackTranslator_StructNotFound(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "types.go")
	os.WriteFile(goFile, []byte(`package model
type Foo struct { X int }
`), 0644)

	tool := stack_translator.NewStackTranslatorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":            "types.go",
		"target_language": "typescript",
		"struct_name":     "NotExist",
	})

	result, _ := tool.Execute(context.Background(), args)
	if result.Success {
		t.Error("deveria falhar para struct inexistente")
	}
}

// === Cap 36: Doc Generator Tests ===

func TestDocGenerator_PreviewMode(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "funcs.go")
	src := `package example

func NewService(name string) *Service {
	return &Service{Name: name}
}

type Service struct {
	Name string
}

func LoadConfig(path string) (string, error) {
	return "", nil
}

func ValidateInput(data string) bool {
	return len(data) > 0
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := doc_generator.NewDocGeneratorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":  "funcs.go",
		"apply": false,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "funções sem documentação") {
		t.Error("deveria reportar funções sem documentação")
	}
	if !strings.Contains(result.Data, "NewService") {
		t.Error("deveria incluir NewService")
	}
}

func TestDocGenerator_ApplyMode(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "funcs.go")
	src := `package example

func ParseData(input string) (string, error) {
	return input, nil
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := doc_generator.NewDocGeneratorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":  "funcs.go",
		"apply": true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	// Verificar que o arquivo foi modificado
	data, _ := os.ReadFile(goFile)
	content := string(data)
	if !strings.Contains(content, "// ParseData") {
		t.Error("deveria ter inserido comentário GoDoc no arquivo")
	}
}

// === Cap 37: Code Explainer Tests ===

func TestCodeExplainer_Detailed(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "example.go")
	src := `package example

import (
	"fmt"
	"strings"
)

// Service gerencia operações de negócio
type Service struct {
	Name string
	Port int
}

// Handler interface de manipulação
type Handler interface {
	Handle(req string) (string, error)
}

// NewService cria um novo serviço
func NewService(name string, port int) *Service {
	return &Service{Name: name, Port: port}
}

func (s *Service) Start() error {
	fmt.Printf("Starting %s on port %d\n", s.Name, s.Port)
	for i := 0; i < 10; i++ {
		if strings.Contains(s.Name, "debug") {
			fmt.Println("Debug mode")
		}
	}
	return nil
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := code_explainer.NewCodeExplainerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":         "example.go",
		"detail_level": "detailed",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "Análise de Código") {
		t.Error("deveria conter cabeçalho de análise")
	}
	if !strings.Contains(result.Data, "Service") {
		t.Error("deveria listar struct Service")
	}
	if !strings.Contains(result.Data, "Handler") {
		t.Error("deveria listar interface Handler")
	}
	if !strings.Contains(result.Data, "NewService") {
		t.Error("deveria listar função NewService")
	}
	if !strings.Contains(result.Data, "Métricas") {
		t.Error("deveria conter seção de métricas")
	}
}

func TestCodeExplainer_FunctionFilter(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "funcs.go")
	os.WriteFile(goFile, []byte(`package example

func FuncA() {}
func FuncB() {}
`), 0644)

	tool := code_explainer.NewCodeExplainerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":          "funcs.go",
		"function_name": "FuncA",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "FuncA") {
		t.Error("deveria conter FuncA")
	}
	if strings.Contains(result.Data, "### `FuncB`") {
		t.Error("não deveria conter FuncB quando filtrado")
	}
}

// === Cap 38: Mock Generator Tests ===

func TestMockGenerator_Interfaces(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "interfaces.go")
	src := `package example

type Repository interface {
	FindByID(id int) (string, error)
	Save(data string) error
	Delete(id int) error
}

type Logger interface {
	Log(msg string)
	LogError(msg string, err error)
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := mock_generator.NewMockGeneratorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path": "interfaces.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "MockRepository") {
		t.Error("deveria gerar MockRepository")
	}
	if !strings.Contains(result.Data, "MockLogger") {
		t.Error("deveria gerar MockLogger")
	}
	if !strings.Contains(result.Data, "FindByIDFunc") {
		t.Error("deveria gerar campo de callback FindByIDFunc")
	}
	if !strings.Contains(result.Data, "FindByIDCalls") {
		t.Error("deveria gerar contador FindByIDCalls")
	}
	if !strings.Contains(result.Data, "var _ Repository = (*MockRepository)(nil)") {
		t.Error("deveria ter verificação de interface em tempo de compilação")
	}
}

func TestMockGenerator_WithJSONOutput(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "model.go")
	src := `package example

type User struct {
	ID    int    ` + "`json:\"id\"`" + `
	Name  string ` + "`json:\"name\"`" + `
	Email string ` + "`json:\"email\"`" + `
}

type Storer interface {
	Store(u User) error
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := mock_generator.NewMockGeneratorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":          "model.go",
		"generate_json": true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "JSON Mock Payloads") {
		t.Error("deveria conter seção de JSON mock")
	}
	if !strings.Contains(result.Data, `"id"`) || !strings.Contains(result.Data, `"name"`) {
		t.Error("deveria gerar campos JSON mock")
	}
}

func TestMockGenerator_SaveToFile(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "iface.go")
	src := `package example

type Service interface {
	Start() error
	Stop()
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := mock_generator.NewMockGeneratorTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":        "iface.go",
		"output_path": "mock_service.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	// Verificar que o arquivo foi criado
	outFile := filepath.Join(dir, "mock_service.go")
	if _, err := os.Stat(outFile); os.IsNotExist(err) {
		t.Error("deveria ter criado o arquivo mock_service.go")
	}

	data, _ := os.ReadFile(outFile)
	if !strings.Contains(string(data), "MockService") {
		t.Error("arquivo de mock deveria conter MockService")
	}
}

// === Cap 39: Complexity Reducer Tests ===

func TestComplexityReducer_SimpleFunction(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "simple.go")
	src := `package example

func Hello() string {
	return "hello"
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := complexity_reducer.NewComplexityReducerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":      "simple.go",
		"threshold": 15,
		"show_all":  true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "Excedem limiar | 0") {
		t.Error("função simples não deveria exceder limiar")
	}
	if !strings.Contains(result.Data, "✅") {
		t.Error("deveria mostrar status verde para função simples")
	}
}

func TestComplexityReducer_ComplexFunction(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "complex.go")
	src := `package example

func ComplexFunc(x int) string {
	if x > 0 {
		if x > 10 {
			if x > 20 {
				for i := 0; i < x; i++ {
					if i%2 == 0 {
						if i%3 == 0 {
							switch {
							case i < 5:
								return "a"
							case i < 10:
								return "b"
							case i < 15:
								return "c"
							case i < 20:
								return "d"
							case i < 25:
								return "e"
							case i < 30:
								return "f"
							}
						}
					}
				}
			}
		}
	} else if x < -10 {
		for j := x; j < 0; j++ {
			if j%2 == 0 || j%3 == 0 {
				return "negative"
			}
		}
	}
	return "default"
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := complexity_reducer.NewComplexityReducerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":      "complex.go",
		"threshold": 5,
		"show_all":  true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "Alta Complexidade") {
		t.Error("deveria detectar alta complexidade")
	}
	if !strings.Contains(result.Data, "🔴") {
		t.Error("deveria mostrar ícone vermelho para alta complexidade")
	}
	if !strings.Contains(result.Data, "Sugestões de refatoração") {
		t.Error("deveria incluir sugestões de refatoração")
	}
}

// === Cap 40: Memory Leak Scanner Tests ===

func TestMemoryLeakScanner_GoroutineWithoutDone(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "leak.go")
	src := `package example

import "fmt"

func StartWorker() {
	go func() {
		for {
			fmt.Println("working...")
		}
	}()
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := memory_leak_scanner.NewMemoryLeakScannerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path": "leak.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "goroutine_leak") {
		t.Error("deveria detectar vazamento de goroutine")
	}
	if !strings.Contains(result.Data, "🔴") {
		t.Error("deveria ter severidade alta")
	}
}

func TestMemoryLeakScanner_GoroutineWithContext(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "safe.go")
	src := `package example

import (
	"context"
	"fmt"
)

func StartWorkerSafe(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				fmt.Println("working...")
			}
		}
	}()
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := memory_leak_scanner.NewMemoryLeakScannerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path": "safe.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	// Com context, não deveria detectar vazamento de goroutine
	if strings.Contains(result.Data, "goroutine_leak") {
		t.Error("não deveria detectar vazamento quando context é usado")
	}
}

func TestMemoryLeakScanner_UnclosedChannel(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "chan_leak.go")
	src := `package example

func ProcessData() {
	ch1 := make(chan int)
	ch2 := make(chan string)
	close(ch1)
	// ch2 never closed
	_ = ch2
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := memory_leak_scanner.NewMemoryLeakScannerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path": "chan_leak.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "unclosed_channel") {
		t.Error("deveria detectar channel não fechado")
	}
}

func TestMemoryLeakScanner_RuntimeProfile(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "app.go")
	os.WriteFile(goFile, []byte(`package example
func Hello() {}
`), 0644)

	tool := memory_leak_scanner.NewMemoryLeakScannerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path":            "app.go",
		"runtime_profile": true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "Perfil de Runtime") {
		t.Error("deveria conter seção de perfil de runtime")
	}
	if !strings.Contains(result.Data, "Goroutines ativas") {
		t.Error("deveria reportar goroutines ativas")
	}
	if !strings.Contains(result.Data, "Alloc") {
		t.Error("deveria reportar métricas de memória")
	}
}

func TestMemoryLeakScanner_NoLeaks(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "clean.go")
	src := `package example

func Add(a, b int) int {
	return a + b
}

func Multiply(a, b int) int {
	return a * b
}
`
	os.WriteFile(goFile, []byte(src), 0644)

	tool := memory_leak_scanner.NewMemoryLeakScannerTool(dir, false)
	args, _ := json.Marshal(map[string]interface{}{
		"path": "clean.go",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("falhou: %s", result.Error)
	}

	if !strings.Contains(result.Data, "Nenhum padrão de vazamento detectado") {
		t.Error("deveria indicar que não há vazamentos")
	}
}
