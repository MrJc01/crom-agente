package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadFileTool_SandboxJail(t *testing.T) {
	ws := t.TempDir()

	// Escreve arquivo interno
	innerFile := filepath.Join(ws, "file.txt")
	_ = os.WriteFile(innerFile, []byte("conteúdo interno"), 0644)

	// Escreve arquivo externo
	outerFile := filepath.Join(filepath.Dir(ws), "outer.txt")
	_ = os.WriteFile(outerFile, []byte("conteúdo externo"), 0644)

	toolJail := NewReadFileTool(ws, true)
	toolFree := NewReadFileTool(ws, false)

	// 1. Lê arquivo interno com Jail (deve funcionar)
	argsOk := json.RawMessage(`{"path": "file.txt"}`)
	res, err := toolJail.Execute(context.Background(), argsOk)
	if err != nil || !res.Success {
		t.Fatalf("erro ao ler arquivo interno: %v, res: %+v", err, res)
	}
	if res.Data != "conteúdo interno" {
		t.Fatalf("conteúdo incorreto: %s", res.Data)
	}

	// 2. Lê arquivo externo com Jail (deve bloquear)
	argsBad := json.RawMessage(`{"path": "../outer.txt"}`)
	res, err = toolJail.Execute(context.Background(), argsBad)
	if err != nil || res.Success {
		t.Fatalf("esperava bloqueio de jail, res: %+v", res)
	}
	if !strings.Contains(res.Error, "está fora do sandbox") {
		t.Fatalf("mensagem de erro inválida: %s", res.Error)
	}

	// 3. Lê arquivo externo sem Jail (deve funcionar)
	res, err = toolFree.Execute(context.Background(), argsBad)
	if err != nil || !res.Success {
		t.Fatalf("erro ao ler sem jail: %v, res: %+v", err, res)
	}
	if res.Data != "conteúdo externo" {
		t.Fatalf("conteúdo sem jail incorreto: %s", res.Data)
	}
}

func TestWriteFileTool_SandboxJail(t *testing.T) {
	ws := t.TempDir()

	toolJail := NewWriteFileTool(ws, true)

	// Escreve arquivo interno (deve funcionar)
	argsOk := json.RawMessage(`{"path": "subdir/file.txt", "content": "gravado"}`)
	res, err := toolJail.Execute(context.Background(), argsOk)
	if err != nil || !res.Success {
		t.Fatalf("erro ao gravar arquivo interno: %v, res: %+v", err, res)
	}

	// Verifica gravação física
	data, _ := os.ReadFile(filepath.Join(ws, "subdir/file.txt"))
	if string(data) != "gravado" {
		t.Fatalf("conteúdo não foi gravado fisicamente: %s", string(data))
	}

	// Escreve arquivo externo com Jail (deve bloquear)
	argsBad := json.RawMessage(`{"path": "../outer_write.txt", "content": "hack"}`)
	res, _ = toolJail.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava erro de sandbox para escrita externa")
	}
}

func TestTerminalCommandTool(t *testing.T) {
	ws := t.TempDir()

	tool := NewTerminalCommandTool(ws, []string{"sudo", "rm -rf"})

	// 1. Executa comando válido
	args := json.RawMessage(`{"command": "echo 'hello terminal'"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar comando: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "hello terminal") {
		t.Fatalf("saída incorreta: %s", res.Data)
	}

	// 2. Executa comando bloqueado
	argsBad := json.RawMessage(`{"command": "sudo apt update"}`)
	res, _ = tool.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava bloqueio do comando sudo")
	}
	if !strings.Contains(res.Error, "comando bloqueado") {
		t.Fatalf("mensagem de bloqueio inválida: %s", res.Error)
	}
}

func TestDiffReplaceTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewDiffReplaceTool(ws, true)

	content := "linha 1\nlinha 2\nbloco para substituir\nlinha 4\nbloco para substituir\nlinha 6"
	testFile := filepath.Join(ws, "test.txt")
	_ = os.WriteFile(testFile, []byte(content), 0644)

	// 1. Substituição bem-sucedida especificando intervalo de linhas para remover ambiguidade
	args := json.RawMessage(`{
		"path": "test.txt",
		"start_line": 1,
		"end_line": 3,
		"target_content": "bloco para substituir",
		"replacement_content": "bloco modificado"
	}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar diff_replace: %v, res: %+v", err, res)
	}

	// Verificar alteração
	data, _ := os.ReadFile(testFile)
	expected := "linha 1\nlinha 2\nbloco modificado\nlinha 4\nbloco para substituir\nlinha 6"
	if string(data) != expected {
		t.Fatalf("substituição incorreta. Esperava:\n%s\nObteve:\n%s", expected, string(data))
	}

	// 2. Erro de ambiguidade (sem especificar intervalo de linhas)
	// Restaurar arquivo para ter múltiplas ocorrências de target_content
	_ = os.WriteFile(testFile, []byte(content), 0644)

	argsAmbiguous := json.RawMessage(`{
		"path": "test.txt",
		"target_content": "bloco para substituir",
		"replacement_content": "bloco modificado"
	}`)
	res, _ = tool.Execute(context.Background(), argsAmbiguous)
	if res.Success {
		t.Fatal("esperava erro de ambiguidade por ter múltiplas ocorrências")
	}
	if !strings.Contains(res.Error, "substituição ambígua") {
		t.Fatalf("erro esperado 'substituição ambígua', obteve: %s", res.Error)
	}

	// 3. Erro de conteúdo não encontrado
	argsMissing := json.RawMessage(`{
		"path": "test.txt",
		"target_content": "conteudo inexistente",
		"replacement_content": "novo"
	}`)
	res, _ = tool.Execute(context.Background(), argsMissing)
	if res.Success {
		t.Fatal("esperava erro de conteúdo não encontrado")
	}
}

func TestRenameFileTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewRenameFileTool(ws, true)

	srcFile := filepath.Join(ws, "origem.txt")
	_ = os.WriteFile(srcFile, []byte("dados"), 0644)

	// Renomear normal
	args := json.RawMessage(`{
		"src_path": "origem.txt",
		"dest_path": "subdir/destino.txt"
	}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao renomear: %v, res: %+v", err, res)
	}

	// Validar criação e existência
	if _, err := os.Stat(filepath.Join(ws, "subdir/destino.txt")); err != nil {
		t.Fatalf("arquivo de destino não existe: %v", err)
	}
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Fatal("arquivo de origem ainda existe")
	}

	// Validar jail em caminho externo
	argsBad := json.RawMessage(`{
		"src_path": "subdir/destino.txt",
		"dest_path": "../externo.txt"
	}`)
	res, _ = tool.Execute(context.Background(), argsBad)
	if res.Success {
		t.Fatal("esperava erro de jail ao mover para fora do workspace")
	}
}

func TestDeleteFileTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewDeleteFileTool(ws, true)

	file := filepath.Join(ws, "deletar.txt")
	_ = os.WriteFile(file, []byte("deletar"), 0644)

	// 1. Deleção normal
	args := json.RawMessage(`{"path": "deletar.txt"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao deletar arquivo: %v, res: %+v", err, res)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("arquivo ainda existe pós deleção")
	}

	// 2. Travas de segurança (proibir deletar go.mod)
	gomod := filepath.Join(ws, "go.mod")
	_ = os.WriteFile(gomod, []byte("module test"), 0644)

	argsGomod := json.RawMessage(`{"path": "go.mod"}`)
	res, _ = tool.Execute(context.Background(), argsGomod)
	if res.Success {
		t.Fatal("esperava bloqueio de segurança ao tentar deletar go.mod")
	}
	if !strings.Contains(res.Error, "não é permitido deletar o go.mod") {
		t.Fatalf("erro esperado sobre go.mod, obteve: %s", res.Error)
	}

	// 3. Travas de segurança (.git)
	gitdir := filepath.Join(ws, ".git")
	_ = os.Mkdir(gitdir, 0755)
	argsGit := json.RawMessage(`{"path": ".git"}`)
	res, _ = tool.Execute(context.Background(), argsGit)
	if res.Success {
		t.Fatal("esperava bloqueio de segurança ao tentar deletar .git")
	}
}

func TestTreeTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewTreeTool(ws, true)

	_ = os.MkdirAll(filepath.Join(ws, "dir1/subdir"), 0755)
	_ = os.WriteFile(filepath.Join(ws, "dir1/subdir/file.txt"), []byte("txt"), 0644)
	_ = os.MkdirAll(filepath.Join(ws, ".git"), 0755) // Deve ser ignorado

	args := json.RawMessage(`{"max_depth": 3}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar tree: %v, res: %+v", err, res)
	}

	if strings.Contains(res.Data, ".git") {
		t.Fatal("tree listou a pasta oculta .git que deveria ser ignorada")
	}
	if !strings.Contains(res.Data, "dir1") || !strings.Contains(res.Data, "file.txt") {
		t.Fatalf("tree não listou arquivos esperados: %s", res.Data)
	}
}

func TestGrepTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewGrepTool(ws, true)

	_ = os.WriteFile(filepath.Join(ws, "file1.txt"), []byte("este é o padrao que procuramos\noutra linha"), 0644)
	_ = os.WriteFile(filepath.Join(ws, "file2.txt"), []byte("nada aqui\npadrao"), 0644)
	_ = os.WriteFile(filepath.Join(ws, "binary.png"), []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 'a', 'b'}, 0644) // Arquivo com byte nulo (binário)

	// 1. Busca simples (case-insensitive por default)
	args := json.RawMessage(`{"query": "padrao"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao rodar grep: %v, res: %+v", err, res)
	}

	if !strings.Contains(res.Data, "file1.txt") || !strings.Contains(res.Data, "file2.txt") {
		t.Fatalf("grep não encontrou ocorrências nos arquivos de texto: %s", res.Data)
	}
	if strings.Contains(res.Data, "binary.png") {
		t.Fatal("grep buscou no arquivo binário binary.png e retornou correspondência")
	}

	// 2. Busca Regex
	argsRegex := json.RawMessage(`{"query": "este é .*", "is_regex": true}`)
	resRegex, err := tool.Execute(context.Background(), argsRegex)
	if err != nil || !resRegex.Success {
		t.Fatalf("erro ao rodar grep com regex: %v", err)
	}
	if !strings.Contains(resRegex.Data, "este é o padrao") {
		t.Fatalf("regex não encontrou padrão: %s", resRegex.Data)
	}
}

func TestPortMonitorTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewPortMonitorTool(ws)

	// 1. Iniciar um TCP listener local em porta dinâmica do S.O.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("falha ao iniciar listener para teste: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// 2. Testar porta ativa
	args := json.RawMessage(fmt.Sprintf(`{"port": %d, "timeout_ms": 500}`, port))
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("esperava sucesso ao escanear porta ativa: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "aberta") {
		t.Fatalf("mensagem inesperada para porta aberta: %s", res.Data)
	}

	// 3. Fechar listener e testar porta fechada
	listener.Close()
	// Aguardar breve liberação do S.O.
	time.Sleep(50 * time.Millisecond)

	res, err = tool.Execute(context.Background(), args)
	if err != nil || res.Success {
		t.Fatalf("esperava falha ao escanear porta inativa: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Error, "fechada") {
		t.Fatalf("mensagem inesperada para porta fechada: %s", res.Error)
	}
}

func TestTerminalCommandTool_StreamingAndInterrupt(t *testing.T) {
	ws := t.TempDir()

	var buf strings.Builder
	tool := NewTerminalCommandTool(ws, nil, &buf)

	// 1. Testar streaming de um comando longo (sleep 1 com echo)
	args := json.RawMessage(`{"command": "echo 'start' && sleep 0.2 && echo 'end'"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil || !res.Success {
		t.Fatalf("erro ao executar comando com sleep: %v, res: %+v", err, res)
	}

	streamed := buf.String()
	if !strings.Contains(streamed, "start") || !strings.Contains(streamed, "end") {
		t.Fatalf("streaming falhou, dados gravados no buffer: %q", streamed)
	}

	// 2. Testar interrupção (Ctrl+C)
	// Disparamos um sleep longo em background em uma goroutine
	longArgs := json.RawMessage(`{"command": "sleep 10"}`)
	
	errChan := make(chan error, 1)
	resChan := make(chan Result, 1)

	go func() {
		r, e := tool.Execute(context.Background(), longArgs)
		resChan <- r
		errChan <- e
	}()

	// Aguarda um instante para o processo iniciar
	time.Sleep(100 * time.Millisecond)

	// Envia sinal de interrupção
	interruptArgs := json.RawMessage(`{"action": "interrupt"}`)
	intRes, err := tool.Execute(context.Background(), interruptArgs)
	if err != nil || !intRes.Success {
		t.Fatalf("falha ao enviar interrupt: %v, res: %+v", err, intRes)
	}

	// Aguarda resultado do comando interrompido
	r := <-resChan
	e := <-errChan

	if e != nil {
		t.Fatalf("erro inesperado na interrupção: %v", e)
	}
	if r.Success {
		t.Fatal("esperava que o comando interrompido retornasse Success = false")
	}
	if !strings.Contains(r.Error, "interrompido") {
		t.Fatalf("mensagem de erro inesperada para interrupção: %s", r.Error)
	}
}

func TestRunTestsTool(t *testing.T) {
	ws := t.TempDir()
	tool := NewRunTestsTool(ws)

	// 1. Testa detecção de stack vazia
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("erro ao executar run_tests: %v", err)
	}
	if res.Success {
		t.Fatal("esperava falha por falta de testes detectáveis")
	}

	// 2. Simula Go project
	_ = os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module test"), 0644)
	
	// Executa com comando customizado leve que sempre passa
	argsCustom := json.RawMessage(`{"command": "echo 'tests passed'"}`)
	res, err = tool.Execute(context.Background(), argsCustom)
	if err != nil || !res.Success {
		t.Fatalf("falha ao rodar testes customizados: %v, res: %+v", err, res)
	}
	if !strings.Contains(res.Data, "tests passed") {
		t.Fatalf("saída inesperada: %s", res.Data)
	}
}

