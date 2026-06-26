package loop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCreatedFile(t *testing.T) {
	// Criar um diretório temporário para os testes
	tempDir, err := os.MkdirTemp("", "file_validator_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name     string
		filename string
		content  string
		lang     string
		wantOk   bool
	}{
		{
			name:     "Valid Go file",
			filename: "valid.go",
			content: `package main
import "fmt"
func main() {
	fmt.Println("Hello")
}`,
			lang:   "go",
			wantOk: true,
		},
		{
			name:     "Invalid Go file syntax",
			filename: "invalid.go",
			content: `package main
import "fmt"
func main() {
	fmt.Println("Hello"
}`,
			lang:   "go",
			wantOk: false,
		},
		{
			name:     "Valid Python file",
			filename: "valid.py",
			content: `def hello():
    print("Hello")
hello()
`,
			lang:   "python",
			wantOk: true,
		},
		{
			name:     "Invalid Python file syntax",
			filename: "invalid.py",
			content: `def hello(
    print("Hello")
`,
			lang:   "python",
			wantOk: false,
		},
		{
			name:     "Valid JSON file",
			filename: "valid.json",
			content:  `{"name": "test", "value": 123}`,
			lang:     "json",
			wantOk:   true,
		},
		{
			name:     "Invalid JSON file",
			filename: "invalid.json",
			content:  `{"name": "test", "value": 123,}`,
			lang:     "json",
			wantOk:   false,
		},
		{
			name:     "Valid HTML file",
			filename: "valid.html",
			content:  `<!DOCTYPE html><html><head><title>Test</title></head><body><h1>Hello</h1></body></html>`,
			lang:     "html",
			wantOk:   true,
		},
		{
			name:     "Auto-detect Go from extension",
			filename: "detect.go",
			content:  `package main`,
			lang:     "",
			wantOk:   true,
		},
		{
			name:     "Mixed tabs and spaces Python",
			filename: "mixed.py",
			content:  "def test():\n\tprint('tab')\n    print('spaces')",
			lang:     "python",
			wantOk:   false,
		},
		{
			name:     "HTML Mismatched tags",
			filename: "mismatched.html",
			content:  "<html><body><h1>Title</h2></body></html>",
			lang:     "html",
			wantOk:   false,
		},
		{
			name:     "HTML Unclosed tag",
			filename: "unclosed.html",
			content:  "<html><body><h1>Title",
			lang:     "html",
			wantOk:   false,
		},
		{
			name:     "SQL valid",
			filename: "query.sql",
			content:  "SELECT * FROM users WHERE name = 'John' AND age = 30",
			lang:     "sql",
			wantOk:   true,
		},
		{
			name:     "SQL unbalanced single quote",
			filename: "query_bad.sql",
			content:  "SELECT * FROM users WHERE name = 'John",
			lang:     "sql",
			wantOk:   false,
		},
		{
			name:     "SQL unbalanced parentheses",
			filename: "query_bad_paren.sql",
			content:  "SELECT * FROM users WHERE (age > 18))",
			lang:     "sql",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			ok, errMsg := ValidateCreatedFile(filePath, tt.lang, "")
			if ok != tt.wantOk {
				t.Errorf("ValidateCreatedFile() got ok = %v, want %v; error message: %q", ok, tt.wantOk, errMsg)
			}
		})
	}

	// Test entry point validation
	t.Run("Python EntryPoint presence", func(t *testing.T) {
		pyPath := filepath.Join(tempDir, "ep_check.py")
		content := "def target_func():\n    return 42\n"
		err := os.WriteFile(pyPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		// target_func exists -> true
		ok, errStr := ValidateCreatedFile(pyPath, "python", "target_func")
		if !ok {
			t.Errorf("expected target_func to exist, got error: %s", errStr)
		}

		// missing_func doesn't exist -> false
		ok, errStr = ValidateCreatedFile(pyPath, "python", "missing_func")
		if ok {
			t.Errorf("expected missing_func to be missing, but validation passed")
		}
	})
}
