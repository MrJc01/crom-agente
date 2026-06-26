package signature_validator

import (
	"context"
	_ "embed"
	"encoding/json"
	"os/exec"
	"regexp"
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
		panic("falha ao carregar metadados de signature_validator: " + err.Error())
	}
}

type SignatureValidatorTool struct {
	workspaceRoot string
}

func NewSignatureValidatorTool(workspaceRoot string) *SignatureValidatorTool {
	return &SignatureValidatorTool{
		workspaceRoot: workspaceRoot,
	}
}

func (t *SignatureValidatorTool) ID() string { return metadata.ID }

func (t *SignatureValidatorTool) Description() string { return metadata.Description }

func (t *SignatureValidatorTool) RequiresApproval() bool { return false }

func (t *SignatureValidatorTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"function_name": {
				"type": "string",
				"description": "Nome da função a verificar chamadores locais (opcional)"
			}
		},
		"required": []
	}`)
}

type BrokenCall struct {
	FilePath   string `json:"file_path"`
	LineNumber string `json:"line_number"`
	ErrorMsg   string `json:"error_message"`
}

type ValidationResult struct {
	Valid       bool         `json:"valid"`
	BrokenCalls []BrokenCall `json:"broken_calls"`
}

func (t *SignatureValidatorTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		FunctionName string `json:"function_name"`
	}
	_ = json.Unmarshal(args, &input)

	// Rodar compilação Go para testar se há erros de assinatura/chamada
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", "headless", "./...")
	cmd.Dir = t.workspaceRoot
	outBytes, err := cmd.CombinedOutput()
	outStr := string(outBytes)

	var broken []BrokenCall

	// Regexes para parse de erros do compilador Go sobre assinatura de funções
	// Ex: "./main.go:25:31: not enough arguments in call to myFunc"
	// Ex: "./main.go:26:31: too many arguments in call to myFunc"
	// Ex: "./main.go:27:31: cannot use x (type int) as type string in argument to myFunc"
	lines := strings.Split(outStr, "\n")
	errRegex := regexp.MustCompile(`^([^:]+):(\d+):(?:\d+:)?\ (.*(?:argument|call|type|cannot\ use).*)`)

	for _, line := range lines {
		if m := errRegex.FindStringSubmatch(line); m != nil {
			file := m[1]
			lineNum := m[2]
			errMsg := m[3]

			// Se filtrou por nome da função, checa se a mensagem contem o nome
			if input.FunctionName != "" && !strings.Contains(errMsg, input.FunctionName) {
				continue
			}

			broken = append(broken, BrokenCall{
				FilePath:   strings.TrimPrefix(file, "./"),
				LineNumber: lineNum,
				ErrorMsg:   errMsg,
			})
		}
	}

	result := ValidationResult{
		Valid:       err == nil && len(broken) == 0,
		BrokenCalls: broken,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return tools.Result{
		Success: true,
		Data:    string(data),
	}, nil
}
