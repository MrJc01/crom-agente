package loop

import (
	"testing"
)

// Tests for pyExprToJSON
func TestPyExprToJSON(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"True", "true"},
		{"False", "false"},
		{"None", "null"},
		{"'hello'", `"hello"`},
		{`"world"`, `"world"`},
		{`[1, 2, "three", True]`, `[1, 2, "three", true]`},
		{`{"key": "value", "num": 42}`, `{"key": "value", "num": 42}`},
		{"123.45", "123.45"},
		{"-9", "-9"},
		{"", "null"},
		{"[]", "[]"},
		{"{}", "{}"},
		{"unsupported_fallback", `"unsupported_fallback"`}, // Fallback to raw string
	}

	for _, c := range cases {
		out, err := pyExprToJSON(c.input)
		if err != nil {
			t.Errorf("Error parsing %s: %v", c.input, err)
		}
		if out != c.expected {
			t.Errorf("For %s, expected %s, got %s", c.input, c.expected, out)
		}
	}
}

// Tests for parsePythonString
func TestParsePythonString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		hasError bool
	}{
		{`"simple"`, "simple", false},
		{`'simple'`, "simple", false},
		{`"escaped \" quote"`, `escaped " quote`, false},
		{`'escaped \' quote'`, `escaped ' quote`, false},
		{`"new\nline\tand\r"`, "new\nline\tand\r", false},
		{`"backslash \\ "`, `backslash \ `, false},
		{`"invalid`, "", true},
		{`bare_string`, "", true},
		{`""`, "", false},
		{`a`, "", true},
	}

	for _, c := range cases {
		out, err := parsePythonString(c.input)
		if c.hasError && err == nil {
			t.Errorf("Expected error for %s, but got none", c.input)
		}
		if !c.hasError && err != nil {
			t.Errorf("Unexpected error for %s: %v", c.input, err)
		}
		if out != c.expected {
			t.Errorf("For %s, expected %q, got %q", c.input, c.expected, out)
		}
	}
}

// Tests for findUniqueFileInText and findUniqueFileInTextWithExtension
func TestFindUniqueFileInText(t *testing.T) {
	content1 := "Please update the file main.go and add tests"
	if f := findUniqueFileInText(content1); f != "main.go" {
		t.Errorf("Expected main.go, got %s", f)
	}

	content2 := "Check main.go and utils.go"
	if f := findUniqueFileInText(content2); f != "" {
		t.Errorf("Expected empty string (not unique), got %s", f)
	}

	content3 := "Here is the python code for script.py, please apply"
	if f := findUniqueFileInTextWithExtension(content3, "python"); f != "script.py" {
		t.Errorf("Expected script.py, got %s", f)
	}

	content4 := "Update api.ts"
	if f := findUniqueFileInTextWithExtension(content4, "ts"); f != "api.ts" {
		t.Errorf("Expected api.ts, got %s", f)
	}
	
	// Fallback to unknown extension
	if f := findUniqueFileInTextWithExtension("Update query.sql", "unknownlang"); f != "query.sql" {
		t.Errorf("Expected query.sql, got %s", f)
	}
}

// Tests for parseFilePathFromComment
func TestParseFilePathFromComment(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"# FILE: app.py", "app.py"},
		{"// File: main.go", "main.go"},
		{"/* arquivo: styles.css */", "styles.css"},
		{"<!-- path: index.html -->", "index.html"},
		{"# script.sh", "script.sh"},
		{"Just a normal comment", ""},
	}

	for _, c := range cases {
		if out := parseFilePathFromComment(c.input); out != c.expected {
			t.Errorf("For %s, expected %q, got %q", c.input, c.expected, out)
		}
	}
}

// Tests for parseFilePathFromText
func TestParseFilePathFromText(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Please edit file: app.py", "app.py"},
		{"O arquivo: main.go", "main.go"},
		{"Update path: src/index.ts.", "src/index.ts"},
		{"Here is server.js:", "server.js"},
		{"Just text without a file", ""},
	}

	for _, c := range cases {
		if out := parseFilePathFromText(c.input); out != c.expected {
			t.Errorf("For %s, expected %q, got %q", c.input, c.expected, out)
		}
	}
}

func TestSplitTopLevel_Extended(t *testing.T) {
	s := "a='b,c', d=[1, 2, 3], e={'x': 1, 'y': 2}"
	parts := splitTopLevel(s, ',')
	if len(parts) != 3 {
		t.Fatalf("esperava 3 partes, obteve %d: %v", len(parts), parts)
	}
	
	// Test escaped inside quotes
	s2 := `a="b,c\",d", e=f`
	parts2 := splitTopLevel(s2, ',')
	if len(parts2) != 2 {
		t.Fatalf("esperava 2 partes, obteve %d: %v", len(parts2), parts2)
	}
}
