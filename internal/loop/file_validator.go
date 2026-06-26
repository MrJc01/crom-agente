package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ValidateCreatedFile verifica se o arquivo criado/editado no caminho especificado possui erros de sintaxe ou compilação.
// Se entryPoint for fornecido, também verifica se a função está definida no arquivo.
// Retorna (true, "") se o arquivo for válido, ou (false, "detalhes do erro") caso contrário.
func ValidateCreatedFile(path string, language string, entryPoint string) (bool, string) {
	// Se a linguagem estiver vazia, detectamos pela extensão do arquivo
	if language == "" {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			language = "go"
		case ".py":
			language = "python"
		case ".json":
			language = "json"
		case ".html", ".htm":
			language = "html"
		case ".yml", ".yaml":
			language = "yaml"
		case ".toml":
			language = "toml"
		case ".sh", ".bash":
			language = "bash"
		case ".sql":
			language = "sql"
		default:
			// Para outras extensões ou se não detectado, retornamos válido por padrão
			return true, ""
		}
	}

	// Ler o conteúdo do arquivo
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("Failed to read file: %v", err)
	}
	contentStr := string(contentBytes)

	// Detecção de Mistura de Tabs e Espaços
	if strings.ToLower(language) == "python" || strings.ToLower(language) == "go" {
		if ok, errStr := checkTabsAndSpaces(contentStr); !ok {
			return false, fmt.Sprintf("Indentation warning:\n%s", errStr)
		}
	}

	switch strings.ToLower(language) {
	case "go":
		// 1. Validação de Sintaxe usando o parser nativo do Go
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return false, fmt.Sprintf("Go syntax error:\n%v", err)
		}

		// 2. Validação usando 'go vet' se o comando estiver disponível
		if _, err := exec.LookPath("go"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", "vet", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				outStr := string(output)
				if strings.Contains(outStr, "named files must be in single package") ||
					strings.Contains(outStr, "package ") && strings.Contains(outStr, "is not in GOROOT") {
					// Ignora esses avisos de infraestrutura e aceita como válido o parse sintático feito anteriormente
					return true, ""
				}
				if outStr == "" {
					// Se o erro do vet for devido ao timeout ou falta de go.mod em pasta temporária, não bloqueamos o parse sintático válido
					return true, ""
				}
				return false, fmt.Sprintf("Go vet error:\n%s", outStr)
			}
		}

	case "python":
		// Validação compilando para AST usando 'python3' ou 'python'
		var pythonCmd string
		if _, err := exec.LookPath("python3"); err == nil {
			pythonCmd = "python3"
		} else if _, err := exec.LookPath("python"); err == nil {
			pythonCmd = "python"
		}

		if pythonCmd != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// AST compilation + EntryPoint + Imports Check
			pyScript := `import ast, sys, importlib.util
try:
    with open(sys.argv[1], 'rb') as f:
        tree = ast.parse(f.read(), filename=sys.argv[1])
    
    # Validação de Assinatura (EntryPoint)
    if len(sys.argv) > 2 and sys.argv[2]:
        ep = sys.argv[2]
        found = any(isinstance(node, ast.FunctionDef) and node.name == ep for node in tree.body)
        if not found:
            print(f"SignatureError: Function '{ep}' is not defined in the module.")
            sys.exit(1)

    # Checagem de Imports
    imported_modules = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for n in node.names:
                imported_modules.append(n.name.split('.')[0])
        elif isinstance(node, ast.ImportFrom):
            if node.module:
                imported_modules.append(node.module.split('.')[0])
    for mod in imported_modules:
        if not mod:
            continue
        try:
            spec = importlib.util.find_spec(mod)
            if spec is None:
                print(f"ImportError: Module '{mod}' is imported but not installed in environment.")
                sys.exit(1)
        except Exception:
            pass

except SyntaxError as e:
    print(f"SyntaxError: {e.msg} at line {e.lineno}, column {e.offset}")
    try:
        with open(sys.argv[1], 'r', encoding='utf-8', errors='replace') as f:
            lines = f.readlines()
            if 1 <= e.lineno <= len(lines):
                print(f"Line {e.lineno}: {lines[e.lineno-1].rstrip()}")
                offset = e.offset if e.offset is not None else 1
                print(" " * (len(str(e.lineno)) + 7 + offset) + "^")
    except Exception as read_err:
        pass
    sys.exit(1)
except Exception as e:
    print(f"Error: {e}")
    sys.exit(1)
`
			var cmd *exec.Cmd
			if entryPoint != "" {
				cmd = exec.CommandContext(ctx, pythonCmd, "-c", pyScript, path, entryPoint)
			} else {
				cmd = exec.CommandContext(ctx, pythonCmd, "-c", pyScript, path)
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("Python syntax/import error:\n%s", string(output))
			}

			// Linter estático Python (Ruff/Flake8/Pyflakes)
			var linterCmd string
			var linterArgs []string
			if _, err := exec.LookPath("ruff"); err == nil {
				linterCmd = "ruff"
				linterArgs = []string{"check", path}
			} else if _, err := exec.LookPath("flake8"); err == nil {
				linterCmd = "flake8"
				linterArgs = []string{path}
			} else if _, err := exec.LookPath("pyflakes"); err == nil {
				linterCmd = "pyflakes"
				linterArgs = []string{path}
			}
			if linterCmd != "" {
				lctx, lcancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer lcancel()
				lcmd := exec.CommandContext(lctx, linterCmd, linterArgs...)
				lout, _ := lcmd.CombinedOutput()
				loutStr := string(lout)
				if strings.Contains(loutStr, "F821") || strings.Contains(loutStr, "undefined name") {
					return false, fmt.Sprintf("Python linter error (undefined name):\n%s", loutStr)
				}
			}
		}

	case "json":
		// Validador de JSON Avançado
		var temp interface{}
		if err := json.Unmarshal(contentBytes, &temp); err != nil {
			line, col := findJSONErrorLineCol(contentStr, err)
			if line > 0 {
				return false, fmt.Sprintf("JSON parse error at line %d, column %d:\n%v", line, col, err)
			}
			return false, fmt.Sprintf("JSON parse error:\n%v", err)
		}

	case "html":
		// Validação de Sintaxe HTML tag balance
		if ok, errStr := checkHTMLTagBalance(contentStr); !ok {
			return false, errStr
		}

	case "yaml", "yaml_spec":
		var pythonCmd string
		if _, err := exec.LookPath("python3"); err == nil {
			pythonCmd = "python3"
		} else if _, err := exec.LookPath("python"); err == nil {
			pythonCmd = "python"
		}
		if pythonCmd != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pyScript := `import sys
try:
    import yaml
    with open(sys.argv[1], 'r', encoding='utf-8') as f:
        yaml.safe_load(f)
except ImportError:
    pass
except Exception as e:
    print(f"YAML Syntax Error: {e}")
    sys.exit(1)
`
			cmd := exec.CommandContext(ctx, pythonCmd, "-c", pyScript, path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("YAML syntax error:\n%s", string(output))
			}
		}

	case "toml":
		var pythonCmd string
		if _, err := exec.LookPath("python3"); err == nil {
			pythonCmd = "python3"
		} else if _, err := exec.LookPath("python"); err == nil {
			pythonCmd = "python"
		}
		if pythonCmd != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pyScript := `import sys
try:
    try:
        import tomllib
    except ImportError:
        import toml as tomllib
    with open(sys.argv[1], 'r', encoding='utf-8') as f:
        tomllib.loads(f.read())
except ImportError:
    pass
except Exception as e:
    print(f"TOML Syntax Error: {e}")
    sys.exit(1)
`
			cmd := exec.CommandContext(ctx, pythonCmd, "-c", pyScript, path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("TOML syntax error:\n%s", string(output))
			}
		}

	case "bash":
		if _, err := exec.LookPath("bash"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-n", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("Bash syntax error:\n%s", string(output))
			}
		}

	case "sql":
		if ok, errStr := checkSQLSyntax(contentStr); !ok {
			return false, fmt.Sprintf("SQL syntax error: %s", errStr)
		}
	}

	return true, ""
}

// checkTabsAndSpaces verifica mistura de tabs e espaços
func checkTabsAndSpaces(content string) (bool, string) {
	hasTabs := false
	hasSpaces := false
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > 0 {
			if line[0] == '\t' {
				hasTabs = true
			} else if line[0] == ' ' {
				hasSpaces = true
			}
		}
		if hasTabs && hasSpaces {
			return false, fmt.Sprintf("Mixed tabs and spaces detected around line %d", i+1)
		}
	}
	return true, ""
}

// findJSONErrorLineCol extrai linha/coluna do SyntaxError do JSON
func findJSONErrorLineCol(content string, err error) (int, int) {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		offset := syntaxErr.Offset
		line := 1
		col := 1
		for i, char := range content {
			if int64(i) >= offset {
				break
			}
			if char == '\n' {
				line++
				col = 1
			} else {
				col++
			}
		}
		return line, col
	}
	return 0, 0
}

// checkHTMLTagBalance valida balanceamento de tags abertas/fechadas
func checkHTMLTagBalance(content string) (bool, string) {
	z := html.NewTokenizer(strings.NewReader(content))
	var tagStack []string
	voidTags := map[string]bool{
		"area": true, "base": true, "br": true, "col": true, "embed": true,
		"hr": true, "img": true, "input": true, "link": true, "meta": true,
		"param": true, "source": true, "track": true, "wbr": true,
	}

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			err := z.Err()
			if err == io.EOF {
				if len(tagStack) > 0 {
					return false, fmt.Sprintf("HTML syntax error: Unclosed tags remaining: %s", strings.Join(tagStack, ", "))
				}
				return true, ""
			}
			return false, fmt.Sprintf("HTML tokenization error: %v", err)
		case html.StartTagToken:
			tn, _ := z.TagName()
			tagName := string(tn)
			if !voidTags[strings.ToLower(tagName)] {
				tagStack = append(tagStack, tagName)
			}
		case html.EndTagToken:
			tn, _ := z.TagName()
			tagName := string(tn)
			if len(tagStack) == 0 {
				return false, fmt.Sprintf("HTML syntax error: Unexpected closing tag </%s> with no open tag", tagName)
			}
			lastOpen := tagStack[len(tagStack)-1]
			if strings.ToLower(lastOpen) != strings.ToLower(tagName) {
				return false, fmt.Sprintf("HTML syntax error: Mismatched tag: expected </%s> but got </%s>", lastOpen, tagName)
			}
			tagStack = tagStack[:len(tagStack)-1]
		}
	}
}

// checkSQLSyntax valida sintaxe básica de SQL
func checkSQLSyntax(sql string) (bool, string) {
	sQuotes := 0
	dQuotes := 0
	parens := 0
	inSQuote := false
	inDQuote := false

	for i := 0; i < len(sql); i++ {
		char := sql[i]
		if char == '\'' && !inDQuote {
			if i > 0 && sql[i-1] == '\\' {
				continue
			}
			inSQuote = !inSQuote
			sQuotes++
		} else if char == '"' && !inSQuote {
			if i > 0 && sql[i-1] == '\\' {
				continue
			}
			inDQuote = !inDQuote
			dQuotes++
		} else if !inSQuote && !inDQuote {
			if char == '(' {
				parens++
			} else if char == ')' {
				parens--
				if parens < 0 {
					return false, "unbalanced parentheses (excess closing parenthesis)"
				}
			}
		}
	}
	if inSQuote {
		return false, "unclosed single quote"
	}
	if inDQuote {
		return false, "unclosed double quote"
	}
	if parens != 0 {
		return false, "unbalanced parentheses"
	}
	return true, ""
}
