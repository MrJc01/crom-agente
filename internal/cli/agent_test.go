package cli

import (
	"strings"
	"testing"
)

func TestCLI_AgentLifecycle(t *testing.T) {
	tempWorkspace := t.TempDir()
	workspacePath = tempWorkspace // Atualiza a variável global definida em root.go

	// 1. Testa list (deve usar a topologia padrão já que o arquivo não existe)
	listOut, err := executeCommand(rootCmd, "agent", "list")
	if err != nil {
		t.Fatalf("erro ao executar agent list: %v", err)
	}
	if !strings.Contains(listOut, "browser") || !strings.Contains(listOut, "spawn") {
		t.Errorf("esperava listar os agentes padrão 'browser' e 'spawn', obteve: %s", listOut)
	}

	// 2. Testa validate (deve avisar que o arquivo não existe)
	valOut1, err := executeCommand(rootCmd, "agent", "validate")
	if err != nil {
		t.Fatalf("erro ao executar agent validate inicial: %v", err)
	}
	if !strings.Contains(valOut1, "não existe") {
		t.Errorf("esperava aviso de arquivo inexistente, obteve: %s", valOut1)
	}

	// 3. Testa add (adiciona especialista do tipo external)
	addOut, err := executeCommand(rootCmd, "agent", "add", "my_coder",
		"--type", "external",
		"--exec-path", "/usr/bin/python3",
		"--description", "Especialista em codificação Python",
		"--args", "script.py,--verbose",
		"--tools", "read_file,write_file",
	)
	if err != nil {
		t.Fatalf("erro ao adicionar agente: %v", err)
	}
	if !strings.Contains(addOut, "my_coder") {
		t.Errorf("esperava mensagem de sucesso com my_coder, obteve: %s", addOut)
	}

	// 4. Testa list novamente para ver se my_coder está lá
	listOut2, err := executeCommand(rootCmd, "agent", "list")
	if err != nil {
		t.Fatalf("erro ao executar agent list após add: %v", err)
	}
	if !strings.Contains(listOut2, "my_coder") || !strings.Contains(listOut2, "external") {
		t.Errorf("esperava encontrar my_coder na lista, obteve: %s", listOut2)
	}

	// 5. Testa validate com o arquivo agora criado
	valOut2, err := executeCommand(rootCmd, "agent", "validate")
	if err != nil {
		t.Fatalf("erro ao executar agent validate após add: %v", err)
	}
	if !strings.Contains(valOut2, "semanticamente válido") {
		t.Errorf("esperava validação com sucesso, obteve: %s", valOut2)
	}

	// 6. Testa remove
	removeOut, err := executeCommand(rootCmd, "agent", "remove", "my_coder")
	if err != nil {
		t.Fatalf("erro ao remover agente: %v", err)
	}
	if !strings.Contains(removeOut, "removido com sucesso") {
		t.Errorf("esperava mensagem de sucesso ao remover, obteve: %s", removeOut)
	}

	// 7. Testa list após remove
	listOut3, err := executeCommand(rootCmd, "agent", "list")
	if err != nil {
		t.Fatalf("erro ao executar agent list após remove: %v", err)
	}
	if strings.Contains(listOut3, "my_coder") {
		t.Errorf("não esperava encontrar my_coder após remoção, obteve: %s", listOut3)
	}
}
