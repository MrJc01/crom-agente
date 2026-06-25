package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed strings.json
var stringsJSON []byte

var dictionary map[string]interface{}

func init() {
	if err := json.Unmarshal(stringsJSON, &dictionary); err != nil {
		panic(fmt.Sprintf("falha ao carregar strings.json embutido: %v", err))
	}
}

// Get recupera uma string pelo caminho exato (ex: "system.optimizer_system_prompt")
// e opcionalmente aplica formatação Sprintf com os argumentos passados.
func Get(keyPath string, args ...interface{}) string {
	parts := strings.Split(keyPath, ".")

	var current interface{} = dictionary
	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return keyPath // fallback para a própria chave se não encontrar
		}
	}

	str, ok := current.(string)
	if !ok {
		return keyPath
	}

	if len(args) > 0 {
		return fmt.Sprintf(str, args...)
	}
	return str
}
