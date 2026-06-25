package external

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess é a função executada como subprocesso mock nos testes.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "no command\n")
		os.Exit(2)
	}

	cmd := args[0]
	switch cmd {
	case "success_json":
		var input struct {
			Prompt       string `json:"prompt"`
			PriorSummary string `json:"prior_summary"`
		}
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			fmt.Fprintf(os.Stderr, "stdin decode error: %v\n", err)
			os.Exit(1)
		}

		res := struct {
			Success        bool   `json:"success"`
			Output         string `json:"output"`
			ContextSummary string `json:"context_summary"`
		}{
			Success:        true,
			Output:         "Received prompt: " + input.Prompt,
			ContextSummary: "Prior: " + input.PriorSummary,
		}
		json.NewEncoder(os.Stdout).Encode(res)

	case "success_plain":
		fmt.Println("plain output here")

	case "fail_process":
		fmt.Fprintln(os.Stderr, "critical error occurred")
		os.Exit(3)

	case "timeout_process":
		time.Sleep(2 * time.Second)
		fmt.Println("should not reach here")
	}
}

func TestExternalAgent_Execute(t *testing.T) {
	// Caso de sucesso com JSON
	agent := NewExternalAgent(
		"ext-agent",
		"Ext desc",
		"Ext prompt",
		[]string{"tool-1"},
		os.Args[0],
		[]string{"-test.run=TestHelperProcess", "--", "success_json"},
		5*time.Second,
	)

	// Injeta a variável de ambiente para ativar o helper process
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")

	ctx := context.Background()
	res, err := agent.Execute(ctx, "hello target", "step one")
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("esperava success=true, obteve false")
	}
	if res.Output != "Received prompt: hello target" {
		t.Errorf("output inesperado: %s", res.Output)
	}
	if res.ContextSummary != "Prior: step one" {
		t.Errorf("context summary inesperado: %s", res.ContextSummary)
	}

	// Caso de sucesso com texto simples (fallback)
	agentPlain := NewExternalAgent(
		"ext-agent",
		"Ext desc",
		"Ext prompt",
		nil,
		os.Args[0],
		[]string{"-test.run=TestHelperProcess", "--", "success_plain"},
		5*time.Second,
	)

	resPlain, err := agentPlain.Execute(ctx, "hello target", "")
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}
	if !resPlain.Success {
		t.Errorf("esperava success=true")
	}
	if strings.TrimSpace(resPlain.Output) != "plain output here" {
		t.Errorf("output inesperado: %q", resPlain.Output)
	}

	// Caso de falha (stderr / exit code não zero)
	agentFail := NewExternalAgent(
		"ext-agent",
		"Ext desc",
		"Ext prompt",
		nil,
		os.Args[0],
		[]string{"-test.run=TestHelperProcess", "--", "fail_process"},
		5*time.Second,
	)

	_, err = agentFail.Execute(ctx, "hello target", "")
	if err == nil {
		t.Fatal("esperava erro de execução do subprocesso")
	}
	if !strings.Contains(err.Error(), "critical error occurred") {
		t.Errorf("mensagem de erro deve conter o stderr, obteve: %v", err)
	}

	// Caso de timeout
	agentTimeout := NewExternalAgent(
		"ext-agent",
		"Ext desc",
		"Ext prompt",
		nil,
		os.Args[0],
		[]string{"-test.run=TestHelperProcess", "--", "timeout_process"},
		100*time.Millisecond, // timeout bem curto
	)

	_, err = agentTimeout.Execute(ctx, "hello target", "")
	if err == nil {
		t.Fatal("esperava erro de timeout")
	}
}

func TestExternalAgent_PythonIntegration(t *testing.T) {
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 não está disponível no sistema, pulando teste de integração real")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("erro ao obter cwd: %v", err)
	}

	// Como estamos em internal/agents/specialists/external, o script está em ../../../../tests/test_agent.py
	scriptPath := filepath.Join(cwd, "../../../../tests/test_agent.py")
	if _, err := os.Stat(scriptPath); err != nil {
		scriptPath = "../../../../tests/test_agent.py"
		if _, err2 := os.Stat(scriptPath); err2 != nil {
			t.Skipf("script test_agent.py não encontrado, pulando")
		}
	}

	agent := NewExternalAgent(
		"py-agent",
		"Python specialist",
		"Guidelines",
		nil,
		pythonPath,
		[]string{scriptPath},
		5*time.Second,
	)

	ctx := context.Background()
	res, err := agent.Execute(ctx, "run calculations", "prior text")
	if err != nil {
		t.Fatalf("Execute falhou: %v", err)
	}

	if !res.Success {
		t.Errorf("esperava success=true")
	}

	if !strings.Contains(res.Output, "Python agent execution success!") {
		t.Errorf("saída inesperada: %s", res.Output)
	}

	if !strings.Contains(res.ContextSummary, "prior text") {
		t.Errorf("context summary inesperado: %s", res.ContextSummary)
	}
}
