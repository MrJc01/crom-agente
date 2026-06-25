package mock_generator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/tools"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de mock_generator: " + err.Error())
	}
}

// MockGeneratorTool gera structs mock para interfaces Go e payloads JSON mock para schemas
type MockGeneratorTool struct {
	workspaceRoot string
	jail          bool
}

// NewMockGeneratorTool cria uma instância do gerador de mocks
func NewMockGeneratorTool(workspaceRoot string, jail bool) *MockGeneratorTool {
	return &MockGeneratorTool{
		workspaceRoot: workspaceRoot,
		jail:          jail,
	}
}

func (t *MockGeneratorTool) ID() string { return metadata.ID }

func (t *MockGeneratorTool) Description() string {
	return metadata.Description
}

func (t *MockGeneratorTool) RequiresApproval() bool { return true }

func (t *MockGeneratorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo Go contendo interfaces ou structs"
			},
			"interface_name": {
				"type": "string",
				"description": "Nome da interface para gerar mock (opcional, se vazio gera para todas)"
			},
			"output_path": {
				"type": "string",
				"description": "Caminho do arquivo de saída para os mocks (opcional, default: mock_<nome>.go)"
			},
			"generate_json": {
				"type": "boolean",
				"description": "Se true, também gera payloads JSON mock para structs encontradas"
			}
		},
		"required": ["path"]
	}`)
}

// GoInterface representa uma interface Go parseada
type GoInterface struct {
	Name    string
	Methods []GoMethod
}

// GoMethod representa um método de uma interface
type GoMethod struct {
	Name    string
	Params  []GoParam
	Returns []string
}

// GoParam representa um parâmetro
type GoParam struct {
	Name string
	Type string
}

func (t *MockGeneratorTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Path          string `json:"path"`
		InterfaceName string `json:"interface_name"`
		OutputPath    string `json:"output_path"`
		GenerateJSON  bool   `json:"generate_json"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos"}, nil
	}

	targetFile, err := tools.ValidatePath(t.workspaceRoot, input.Path, t.jail)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, targetFile, nil, parser.ParseComments)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("erro ao parsear: %s", err.Error())}, nil
	}

	// Extrair interfaces
	interfaces := extractInterfaces(node)
	if input.InterfaceName != "" {
		var filtered []GoInterface
		for _, iface := range interfaces {
			if iface.Name == input.InterfaceName {
				filtered = append(filtered, iface)
			}
		}
		interfaces = filtered
	}

	var sb strings.Builder
	packageName := node.Name.Name

	// Gerar mocks para interfaces
	if len(interfaces) > 0 {
		mockCode := generateMockCode(packageName, interfaces)
		sb.WriteString(mockCode)

		// Salvar em arquivo se output_path especificado
		if input.OutputPath != "" {
			outFile, err := tools.ValidatePath(t.workspaceRoot, input.OutputPath, t.jail)
			if err != nil {
				return tools.Result{Success: false, Error: err.Error()}, nil
			}
			if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
				return tools.Result{Success: false, Error: fmt.Sprintf("erro ao criar diretórios: %s", err.Error())}, nil
			}
			if err := os.WriteFile(outFile, []byte(mockCode), 0644); err != nil {
				return tools.Result{Success: false, Error: fmt.Sprintf("erro ao gravar mock: %s", err.Error())}, nil
			}
			sb.WriteString(fmt.Sprintf("\n\n// Arquivo salvo em: %s\n", input.OutputPath))
		}
	} else {
		sb.WriteString("// Nenhuma interface encontrada para gerar mocks.\n")
	}

	// Gerar JSON mock para structs se solicitado
	if input.GenerateJSON {
		structs, _ := tools.ParseGoStructs(targetFile)
		if len(structs) > 0 {
			sb.WriteString("\n\n// === JSON Mock Payloads ===\n\n")
			for _, s := range structs {
				jsonMock := generateJSONMock(s)
				sb.WriteString(fmt.Sprintf("// Mock para %s:\n%s\n\n", s.Name, jsonMock))
			}
		}
	}

	return tools.Result{Success: true, Data: sb.String()}, nil
}

// extractInterfaces extrai todas as interfaces de um arquivo Go
func extractInterfaces(node *ast.File) []GoInterface {
	var interfaces []GoInterface

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			ifaceType, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}

			iface := GoInterface{Name: typeSpec.Name.Name}

			if ifaceType.Methods != nil {
				for _, method := range ifaceType.Methods.List {
					if len(method.Names) == 0 {
						continue // embedded interface
					}

					funcType, ok := method.Type.(*ast.FuncType)
					if !ok {
						continue
					}

					gm := GoMethod{Name: method.Names[0].Name}

					// Parâmetros
					if funcType.Params != nil {
						for _, p := range funcType.Params.List {
							typeName := tools.ExprToString(p.Type)
							if len(p.Names) > 0 {
								for _, name := range p.Names {
									gm.Params = append(gm.Params, GoParam{
										Name: name.Name,
										Type: typeName,
									})
								}
							} else {
								gm.Params = append(gm.Params, GoParam{Type: typeName})
							}
						}
					}

					// Retornos
					if funcType.Results != nil {
						for _, r := range funcType.Results.List {
							gm.Returns = append(gm.Returns, tools.ExprToString(r.Type))
						}
					}

					iface.Methods = append(iface.Methods, gm)
				}
			}

			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

// generateMockCode gera código Go com structs mock para as interfaces
func generateMockCode(packageName string, interfaces []GoInterface) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package %s\n\n", packageName))
	sb.WriteString("// Auto-generated mocks by crom-agente mock_generator\n")
	sb.WriteString("// DO NOT EDIT - regenerate with: mock_generator tool\n\n")

	for _, iface := range interfaces {
		mockName := "Mock" + iface.Name
		sb.WriteString(fmt.Sprintf("// %s é uma implementação mock de %s para testes\n", mockName, iface.Name))
		sb.WriteString(fmt.Sprintf("type %s struct {\n", mockName))

		// Campos de callback para cada método
		for _, m := range iface.Methods {
			callbackType := generateCallbackType(m)
			sb.WriteString(fmt.Sprintf("\t%sFunc %s\n", m.Name, callbackType))
			sb.WriteString(fmt.Sprintf("\t%sCalls int\n", m.Name))
		}
		sb.WriteString("}\n\n")

		// Implementação dos métodos
		for _, m := range iface.Methods {
			params := generateParamList(m.Params)
			returns := generateReturnList(m.Returns)

			sig := fmt.Sprintf("func (m *%s) %s(%s)", mockName, m.Name, params)
			if returns != "" {
				sig += " " + returns
			}
			sb.WriteString(sig + " {\n")
			sb.WriteString(fmt.Sprintf("\tm.%sCalls++\n", m.Name))

			if len(m.Returns) > 0 {
				sb.WriteString(fmt.Sprintf("\tif m.%sFunc != nil {\n", m.Name))

				// Chamar o callback
				callArgs := generateCallArgs(m.Params)
				sb.WriteString(fmt.Sprintf("\t\treturn m.%sFunc(%s)\n", m.Name, callArgs))
				sb.WriteString("\t}\n")

				// Retorno default
				sb.WriteString(fmt.Sprintf("\treturn %s\n", generateZeroValues(m.Returns)))
			} else {
				sb.WriteString(fmt.Sprintf("\tif m.%sFunc != nil {\n", m.Name))
				callArgs := generateCallArgs(m.Params)
				sb.WriteString(fmt.Sprintf("\t\tm.%sFunc(%s)\n", m.Name, callArgs))
				sb.WriteString("\t}\n")
			}

			sb.WriteString("}\n\n")
		}

		// Verificação de interface em tempo de compilação
		sb.WriteString(fmt.Sprintf("var _ %s = (*%s)(nil)\n\n", iface.Name, mockName))
	}

	return sb.String()
}

func generateCallbackType(m GoMethod) string {
	params := generateParamList(m.Params)
	returns := generateReturnList(m.Returns)

	sig := fmt.Sprintf("func(%s)", params)
	if returns != "" {
		sig += " " + returns
	}
	return sig
}

func generateParamList(params []GoParam) string {
	var parts []string
	for _, p := range params {
		if p.Name != "" {
			parts = append(parts, fmt.Sprintf("%s %s", p.Name, p.Type))
		} else {
			parts = append(parts, p.Type)
		}
	}
	return strings.Join(parts, ", ")
}

func generateCallArgs(params []GoParam) string {
	var parts []string
	for i, p := range params {
		if p.Name != "" {
			parts = append(parts, p.Name)
		} else {
			parts = append(parts, fmt.Sprintf("arg%d", i))
		}
	}
	return strings.Join(parts, ", ")
}

func generateReturnList(returns []string) string {
	if len(returns) == 0 {
		return ""
	}
	if len(returns) == 1 {
		return returns[0]
	}
	return "(" + strings.Join(returns, ", ") + ")"
}

func generateZeroValues(returns []string) string {
	var zeros []string
	for _, r := range returns {
		zeros = append(zeros, zeroValue(r))
	}
	return strings.Join(zeros, ", ")
}

func zeroValue(goType string) string {
	switch goType {
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return "0"
	case "bool":
		return "false"
	case "error":
		return "nil"
	}

	if strings.HasPrefix(goType, "*") || strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[") {
		return "nil"
	}

	return goType + "{}"
}

// generateJSONMock gera um payload JSON mock baseado em uma struct Go
func generateJSONMock(s tools.GoStruct) string {
	mock := make(map[string]interface{})

	for _, f := range s.Fields {
		fieldName := f.JSONTag
		if fieldName == "" || fieldName == "-" {
			fieldName = f.Name
		}
		if fieldName == "-" {
			continue
		}

		mock[fieldName] = mockValueForType(f.Type, f.Name)
	}

	data, _ := json.MarshalIndent(mock, "", "  ")
	return string(data)
}

func mockValueForType(goType string, fieldName string) interface{} {
	switch goType {
	case "string":
		return fmt.Sprintf("mock_%s_value", strings.ToLower(fieldName))
	case "int", "int8", "int16", "int32", "int64":
		return 42
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return 1
	case "float32", "float64":
		return 3.14
	case "bool":
		return true
	case "time.Time":
		return "2025-01-01T00:00:00Z"
	}

	if strings.HasPrefix(goType, "[]") {
		inner := mockValueForType(goType[2:], fieldName)
		return []interface{}{inner}
	}

	if strings.HasPrefix(goType, "*") {
		return mockValueForType(goType[1:], fieldName)
	}

	if strings.HasPrefix(goType, "map[") {
		return map[string]interface{}{"key": "value"}
	}

	return nil
}
