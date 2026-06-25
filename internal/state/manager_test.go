package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/crom/crom-agente/internal/llm"
)

// TestNewStateManager verifica que o StateManager é criado com estado padrão
func TestNewStateManager(t *testing.T) {
	sm := NewStateManager(t.TempDir())
	s := sm.GetState()

	if s.UltimoStatus != "idle" {
		t.Fatalf("esperado status 'idle', obteve '%s'", s.UltimoStatus)
	}
	if s.TokensGastos != 0 {
		t.Fatalf("esperado 0 tokens, obteve %d", s.TokensGastos)
	}
	if len(s.ArquivosFocados) != 0 {
		t.Fatalf("esperado 0 arquivos focados, obteve %d", len(s.ArquivosFocados))
	}
}

// TestLoadState_CreatesDefaultWhenMissing verifica que LoadState cria o arquivo se não existir
func TestLoadState_CreatesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)

	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	// O arquivo JSON deve ter sido criado no disco
	stateFile := filepath.Join(dir, DefaultStateFileName)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatalf("arquivo de estado não foi criado: %s", stateFile)
	}
}

// TestSaveAndLoadState_Persistence verifica que dados persistem e restauram corretamente
func TestSaveAndLoadState_Persistence(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)

	// Configura dados no estado
	if err := sm.SetActiveTask("Refatorar módulo de auth"); err != nil {
		t.Fatalf("SetActiveTask falhou: %v", err)
	}
	if err := sm.RecordTokens(1500); err != nil {
		t.Fatalf("RecordTokens falhou: %v", err)
	}
	if err := sm.AddLog("Iniciou análise do módulo de auth"); err != nil {
		t.Fatalf("AddLog falhou: %v", err)
	}

	// Cria um novo StateManager apontando para o mesmo diretório e carrega do disco
	sm2 := NewStateManager(dir)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState no segundo manager falhou: %v", err)
	}

	s := sm2.GetState()
	if s.TarefaEmAndamento != "Refatorar módulo de auth" {
		t.Fatalf("esperado tarefa 'Refatorar módulo de auth', obteve '%s'", s.TarefaEmAndamento)
	}
	if s.TokensGastos != 1500 {
		t.Fatalf("esperado 1500 tokens, obteve %d", s.TokensGastos)
	}
	if s.TotalTurnos != 1 {
		t.Fatalf("esperado 1 turno, obteve %d", s.TotalTurnos)
	}
	if s.UltimoStatus != "thinking" {
		t.Fatalf("esperado status 'thinking', obteve '%s'", s.UltimoStatus)
	}
	if len(s.LogsRelevantes) != 1 || s.LogsRelevantes[0] != "Iniciou análise do módulo de auth" {
		t.Fatalf("log relevante incorreto: %v", s.LogsRelevantes)
	}
}

// TestAddLog_RotatesAtMax verifica que o histórico de logs é rotacionado ao atingir o limite
func TestAddLog_RotatesAtMax(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)

	// Adiciona MaxRelevantLogs + 5 entradas
	for i := 0; i < MaxRelevantLogs+5; i++ {
		if err := sm.AddLog("log entry"); err != nil {
			t.Fatalf("AddLog falhou na iteração %d: %v", i, err)
		}
	}

	s := sm.GetState()
	if len(s.LogsRelevantes) != MaxRelevantLogs {
		t.Fatalf("esperado %d logs, obteve %d", MaxRelevantLogs, len(s.LogsRelevantes))
	}
}

// TestSaveState_AtomicWrite verifica que a escrita é atômica (não deixa arquivo .tmp solto)
func TestSaveState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)

	if err := sm.SetStatus("executing"); err != nil {
		t.Fatalf("SetStatus falhou: %v", err)
	}

	// Verifica que não há arquivo .tmp remanescente
	tmpFile := filepath.Join(dir, DefaultStateFileName+".tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatalf("arquivo temporário não deveria existir após SaveState: %s", tmpFile)
	}

	// Verifica que o JSON salvo é válido
	data, err := os.ReadFile(filepath.Join(dir, DefaultStateFileName))
	if err != nil {
		t.Fatalf("falha ao ler estado salvo: %v", err)
	}
	var parsed AgentState
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON salvo é inválido: %v", err)
	}
	if parsed.UltimoStatus != "executing" {
		t.Fatalf("esperado status 'executing' no JSON, obteve '%s'", parsed.UltimoStatus)
	}
}

// TestLoadState_InvalidJSON verifica que JSON malformado retorna erro sem crashar
func TestLoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, DefaultStateFileName)

	// Grava JSON inválido no disco
	if err := os.WriteFile(stateFile, []byte("{invalid json!!!}"), 0644); err != nil {
		t.Fatalf("falha ao gravar JSON inválido: %v", err)
	}

	sm := NewStateManager(dir)
	err := sm.LoadState()
	if err == nil {
		t.Fatal("esperado erro ao carregar JSON inválido, mas LoadState retornou nil")
	}
}

// TestConcurrentAccess_NoRaceCondition verifica que leituras e escritas concorrentes são seguras
// Este teste DEVE ser executado com: go test -race ./...
func TestConcurrentAccess_NoRaceCondition(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)
	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 50

	// Dispara escritas concorrentes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = sm.AddLog("concurrent log entry")
			_ = sm.RecordTokens(10)
			_ = sm.SetStatus("thinking")
		}(i)
	}

	// Dispara leituras concorrentes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.GetState()
		}()
	}

	wg.Wait()

	s := sm.GetState()
	if s.TokensGastos != goroutines*10 {
		t.Fatalf("esperado %d tokens, obteve %d", goroutines*10, s.TokensGastos)
	}
}

// TestSessionStateManagerAndMessages verifica a persistência e recuperação de mensagens em sessões específicas
func TestSessionStateManagerAndMessages(t *testing.T) {
	dir := t.TempDir()
	sessionName := "test-session-xyz"
	sm := NewSessionStateManager(dir, sessionName)

	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	expectedFile := filepath.Join(dir, "sessions", sessionName, "session.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("arquivo de sessão não foi criado em %s", expectedFile)
	}

	msgs := []llm.Message{
		{Role: "user", Content: "Olá"},
		{Role: "assistant", Content: "Olá! Como posso ajudar?"},
	}

	if err := sm.SetMessages(msgs); err != nil {
		t.Fatalf("SetMessages falhou: %v", err)
	}

	// Cria um novo manager para carregar
	sm2 := NewSessionStateManager(dir, sessionName)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState no manager2 falhou: %v", err)
	}

	loadedMsgs := sm2.GetMessages()
	if len(loadedMsgs) != 2 {
		t.Fatalf("esperava 2 mensagens, obteve %d", len(loadedMsgs))
	}

	if loadedMsgs[0].Role != "user" || loadedMsgs[0].Content != "Olá" {
		t.Fatalf("primeira mensagem incorreta: %+v", loadedMsgs[0])
	}
	if loadedMsgs[1].Role != "assistant" || loadedMsgs[1].Content != "Olá! Como posso ajudar?" {
		t.Fatalf("segunda mensagem incorreta: %+v", loadedMsgs[1])
	}
}

// TestSubagentsContext verifica a persistencia do resumo do historico do subagente
func TestSubagentsContext(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)

	// Inicialmente vazio
	if res := sm.GetSummaryForAgent("test-agent"); res != "" {
		t.Fatalf("esperava resumo vazio, obteve '%s'", res)
	}

	// Atualiza e verifica no mesmo manager
	if err := sm.UpdateSummaryForAgent("test-agent", "step 1 completed"); err != nil {
		t.Fatalf("UpdateSummaryForAgent falhou: %v", err)
	}

	if res := sm.GetSummaryForAgent("test-agent"); res != "step 1 completed" {
		t.Fatalf("esperava 'step 1 completed', obteve '%s'", res)
	}

	// Carrega em outro manager para garantir persistencia em disco
	sm2 := NewStateManager(dir)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	if res := sm2.GetSummaryForAgent("test-agent"); res != "step 1 completed" {
		t.Fatalf("esperava 'step 1 completed' após recarregar, obteve '%s'", res)
	}
}

func TestStructuredStatusesAndCognitiveModes(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)
	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	// 1. Testa os valores padrão
	s := sm.GetState()
	if s.StatusOperacional != StatusIdle {
		t.Errorf("esperava StatusOperacional padrão '%s', obteve '%s'", StatusIdle, s.StatusOperacional)
	}
	if s.ModoCognitivo != ModoPlanning {
		t.Errorf("esperava ModoCognitivo padrão '%s', obteve '%s'", ModoPlanning, s.ModoCognitivo)
	}

	// 2. Testa SetOperationalStatus e SetCognitiveMode
	if err := sm.SetOperationalStatus(StatusThinking); err != nil {
		t.Fatalf("SetOperationalStatus falhou: %v", err)
	}
	if err := sm.SetCognitiveMode(ModoExecuting); err != nil {
		t.Fatalf("SetCognitiveMode falhou: %v", err)
	}

	// Verifica se persistiu e atualizou os campos legados de compatibilidade
	s2 := sm.GetState()
	if s2.StatusOperacional != StatusThinking || s2.UltimoStatus != StatusThinking || s2.Status != StatusThinking {
		t.Errorf("valores de status operacional ou de compatibilidade não coincidem: %+v", s2)
	}
	if s2.ModoCognitivo != ModoExecuting {
		t.Errorf("ModoCognitivo incorreto: %s", s2.ModoCognitivo)
	}

	// 3. Testa recarregar do disco em novo StateManager
	sm2 := NewStateManager(dir)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}
	s3 := sm2.GetState()
	if s3.StatusOperacional != StatusThinking || s3.ModoCognitivo != ModoExecuting {
		t.Errorf("valores não persistiram no disco corretamente: %+v", s3)
	}
}

func TestStateManager_Metrics(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)
	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	if err := sm.RecordFileCreated(); err != nil {
		t.Fatalf("RecordFileCreated falhou: %v", err)
	}
	if err := sm.RecordFileValidated(); err != nil {
		t.Fatalf("RecordFileValidated falhou: %v", err)
	}
	if err := sm.RecordToolCallEmitted(); err != nil {
		t.Fatalf("RecordToolCallEmitted falhou: %v", err)
	}
	if err := sm.RecordToolCallsFromTextParse(3); err != nil {
		t.Fatalf("RecordToolCallsFromTextParse falhou: %v", err)
	}
	if err := sm.SetCircuitBreakerTriggered(true); err != nil {
		t.Fatalf("SetCircuitBreakerTriggered falhou: %v", err)
	}

	// Novo manager para validar a persistência em disco
	sm2 := NewStateManager(dir)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	s := sm2.GetState()
	if s.FilesCreated != 1 {
		t.Errorf("esperava FilesCreated=1, obteve %d", s.FilesCreated)
	}
	if s.FilesValidated != 1 {
		t.Errorf("esperava FilesValidated=1, obteve %d", s.FilesValidated)
	}
	if s.ToolCallsEmitted != 1 {
		t.Errorf("esperava ToolCallsEmitted=1, obteve %d", s.ToolCallsEmitted)
	}
	if s.ToolCallsFromTextParse != 3 {
		t.Errorf("esperava ToolCallsFromTextParse=3, obteve %d", s.ToolCallsFromTextParse)
	}
	if !s.CircuitBreakerTriggered {
		t.Errorf("esperava CircuitBreakerTriggered=true, obteve %v", s.CircuitBreakerTriggered)
	}
}

func TestStateManager_Telemetry(t *testing.T) {
	dir := t.TempDir()
	sm := NewStateManager(dir)
	if err := sm.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	// Testa SetCurrentStep
	if err := sm.SetCurrentStep("Executando teste"); err != nil {
		t.Fatalf("SetCurrentStep falhou: %v", err)
	}
	if err := sm.SetCurrentStepDurationMs(120); err != nil {
		t.Fatalf("SetCurrentStepDurationMs falhou: %v", err)
	}

	terms := []TerminalTelemetry{
		{ID: "term1", PID: 1234, Name: "bash", Closed: false},
	}
	if err := sm.UpdateActiveTerminals(terms); err != nil {
		t.Fatalf("UpdateActiveTerminals falhou: %v", err)
	}

	procs := []ProcessTelemetry{
		{ID: "bg-1", Command: "make build --api-key=sk-123456789012345678901234567890123456789012345678", PID: 4567, Status: "running", IsBackground: true},
	}
	if err := sm.UpdateActiveProcesses(procs); err != nil {
		t.Fatalf("UpdateActiveProcesses falhou: %v", err)
	}

	// Valida no manager 2
	sm2 := NewStateManager(dir)
	if err := sm2.LoadState(); err != nil {
		t.Fatalf("LoadState falhou: %v", err)
	}

	s := sm2.GetState()
	if s.CurrentStep != "Executando teste" {
		t.Errorf("esperava CurrentStep='Executando teste', obteve '%s'", s.CurrentStep)
	}
	if s.CurrentStepDurationMs != 120 {
		t.Errorf("esperava CurrentStepDurationMs=120, obteve %d", s.CurrentStepDurationMs)
	}
	if len(s.ActiveTerminals) != 1 || s.ActiveTerminals[0].ID != "term1" {
		t.Errorf("ActiveTerminals incorretos: %+v", s.ActiveTerminals)
	}
	if len(s.ActiveProcesses) != 1 || s.ActiveProcesses[0].ID != "bg-1" {
		t.Errorf("ActiveProcesses incorretos: %+v", s.ActiveProcesses)
	}
	// A senha/segredo no comando deve ser redigida!
	if s.ActiveProcesses[0].Command == "make build --api-key=sk-123456789012345678901234567890123456789012345678" {
		t.Errorf("esperava que o comando do processo estivesse redigido, obteve '%s'", s.ActiveProcesses[0].Command)
	}

	// Testa limpeza
	if err := sm.ClearActiveTerminals(); err != nil {
		t.Fatalf("ClearActiveTerminals falhou: %v", err)
	}
	if err := sm.ClearActiveProcesses(); err != nil {
		t.Fatalf("ClearActiveProcesses falhou: %v", err)
	}

	s2 := sm.GetState()
	if len(s2.ActiveTerminals) != 0 {
		t.Errorf("ActiveTerminals deveriam estar vazios, obteve %+v", s2.ActiveTerminals)
	}
	if len(s2.ActiveProcesses) != 0 {
		t.Errorf("ActiveProcesses deveriam estar vazios, obteve %+v", s2.ActiveProcesses)
	}
}


