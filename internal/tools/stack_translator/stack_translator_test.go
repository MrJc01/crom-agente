package stack_translator

import (
	"testing"
)

func TestStackTranslator(t *testing.T) {
	src := `package main
// User representa um usuario do sistema
type User struct {
	ID        int      ` + "`" + `json:"id"` + "`" + `
	Name      string   ` + "`" + `json:"name"` + "`" + `
	Email     string   ` + "`" + `json:"email,omitempty"` + "`" + ` // email opcional
	Tags      []string ` + "`" + `json:"tags"` + "`" + `
	IsActive  bool     ` + "`" + `json:"is_active"` + "`" + `
}
`
	structs, err := ParseGoStructsFromSource(src)
	if err != nil {
		t.Fatalf("erro ao parsear structs do fonte: %v", err)
	}

	if len(structs) != 1 {
		t.Fatalf("esperava 1 struct, obteve: %d", len(structs))
	}

	s := structs[0]
	if s.Name != "User" {
		t.Fatalf("esperava struct User, obteve: %s", s.Name)
	}

	// 1. TypeScript
	ts := translateToTypeScript(structs)
	if !contains(ts, "export interface User") || !contains(ts, "email?: string;") || !contains(ts, "tags: string[];") {
		t.Errorf("TypeScript incorreto: %s", ts)
	}

	// 2. Python
	py := translateToPython(structs)
	if !contains(py, "class User:") || !contains(py, "email: str") || !contains(py, "tags: List[str]") {
		t.Errorf("Python incorreto: %s", py)
	}

	// 3. JSON Schema
	schema := translateToJSONSchema(structs)
	if !contains(schema, `"type": "object"`) || !contains(schema, `"email"`) || !contains(schema, `"tags"`) {
		t.Errorf("JSON Schema incorreto: %s", schema)
	}

	// 4. Rust
	rust := translateToRust(structs)
	if !contains(rust, "pub struct User") || !contains(rust, `#[serde(rename = "email")]`) {
		t.Errorf("Rust incorreto: %s", rust)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(s)] == substr || stringsContains(s, substr))
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
