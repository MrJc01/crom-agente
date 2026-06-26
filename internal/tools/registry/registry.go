package registry

import (
	"io"
	"os"

	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/ask_user"
	"github.com/crom/crom-agente/internal/tools/call_graph"
	"github.com/crom/crom-agente/internal/tools/code_explainer"
	"github.com/crom/crom-agente/internal/tools/complexity_reducer"
	"github.com/crom/crom-agente/internal/tools/computer_control"
	"github.com/crom/crom-agente/internal/tools/database_tester"
	"github.com/crom/crom-agente/internal/tools/delete_file"
	"github.com/crom/crom-agente/internal/tools/dependency_graph"
	"github.com/crom/crom-agente/internal/tools/edit_file"
	"github.com/crom/crom-agente/internal/tools/edit_file_by_line"
	"github.com/crom/crom-agente/internal/tools/doc_generator"
	"github.com/crom/crom-agente/internal/tools/error_histogram"
	"github.com/crom/crom-agente/internal/tools/find_file"
	"github.com/crom/crom-agente/internal/tools/git_add"
	"github.com/crom/crom-agente/internal/tools/git_branch"
	"github.com/crom/crom-agente/internal/tools/git_commit"
	"github.com/crom/crom-agente/internal/tools/git_conflict"
	"github.com/crom/crom-agente/internal/tools/git_diff"
	"github.com/crom/crom-agente/internal/tools/git_diff_summary"
	"github.com/crom/crom-agente/internal/tools/git_log"
	"github.com/crom/crom-agente/internal/tools/git_status"
	"github.com/crom/crom-agente/internal/tools/grep"
	"github.com/crom/crom-agente/internal/tools/http_client"
	"github.com/crom/crom-agente/internal/tools/manage_plan"
	"github.com/crom/crom-agente/internal/tools/memory_leak_scanner"
	"github.com/crom/crom-agente/internal/tools/mock_generator"
	"github.com/crom/crom-agente/internal/tools/port_monitor"
	"github.com/crom/crom-agente/internal/tools/proxy"
	"github.com/crom/crom-agente/internal/llm"
	"github.com/crom/crom-agente/internal/tools/autofix"
	"github.com/crom/crom-agente/internal/tools/bug_explainer"
	"github.com/crom/crom-agente/internal/tools/checkpoint"
	"github.com/crom/crom-agente/internal/tools/pytest_isolated"
	"github.com/crom/crom-agente/internal/tools/read_file"
	"github.com/crom/crom-agente/internal/tools/read_session_messages"
	"github.com/crom/crom-agente/internal/tools/record_decision"
	"github.com/crom/crom-agente/internal/tools/refactor_auditor"
	"github.com/crom/crom-agente/internal/tools/rename_file"
	"github.com/crom/crom-agente/internal/tools/run_browser_test"
	"github.com/crom/crom-agente/internal/tools/run_tests"
	"github.com/crom/crom-agente/internal/tools/schedule_timer"
	"github.com/crom/crom-agente/internal/tools/scraper"
	"github.com/crom/crom-agente/internal/tools/stack_translator"
	"github.com/crom/crom-agente/internal/tools/syntax_check"
	"github.com/crom/crom-agente/internal/tools/terminal_command"
	"github.com/crom/crom-agente/internal/tools/tree"
	"github.com/crom/crom-agente/internal/tools/undo_last_edit"
	"github.com/crom/crom-agente/internal/tools/view_function"
	"github.com/crom/crom-agente/internal/tools/write_file"
	"github.com/crom/crom-agente/internal/tools/ast_analyzer"
	"github.com/crom/crom-agente/internal/tools/concurrency_lock"
	"github.com/crom/crom-agente/internal/tools/cost_estimator"
	"github.com/crom/crom-agente/internal/tools/git_diff_advanced"
	"github.com/crom/crom-agente/internal/tools/import_validator"
	"github.com/crom/crom-agente/internal/tools/inject_local_env"
	"github.com/crom/crom-agente/internal/tools/legacy_matcher"
	"github.com/crom/crom-agente/internal/tools/mttr_report"
	"github.com/crom/crom-agente/internal/tools/read_log_paginated"
	"github.com/crom/crom-agente/internal/tools/semantic_search"
	"github.com/crom/crom-agente/internal/tools/signature_validator"
)

// RegistrationConfig configura a inicialização em lote de todas as ferramentas nativas
type RegistrationConfig struct {
	WorkspacePath    string
	WorkspaceJail    bool
	BlockedCommands  []string
	TerminalOutput   io.Writer
	OnSchedule       func(task string, durationSeconds int)
	OnBackgroundExit func(bgID, cmdStr, logs string, success bool)

	// Instâncias pré-configuradas e opcionais de navegadores
	BrowserTool        tools.Tool
	SubagentTool       tools.Tool
	StateManager       *state.StateManager
	LLMProvider        llm.Provider
	DisableInteraction bool
}

// GetBuiltinTools retorna a lista completa de ferramentas nativas instanciadas e prontas para registro
func GetBuiltinTools(cfg RegistrationConfig) []tools.Tool {
	var list []tools.Tool

	// 1. Ferramenta de agendamento e interacao
	list = append(list, schedule_timer.NewScheduleTimerTool(cfg.WorkspacePath, cfg.OnSchedule))
	if !cfg.DisableInteraction && os.Getenv("CROM_DISABLE_INTERACTION") != "true" && os.Getenv("CROM_DISABLE_INTERACTION") != "1" {
		list = append(list, ask_user.NewAskUserTool(cfg.WorkspacePath))
	}

	// 2. Leitura e Escrita
	list = append(list, read_file.NewReadFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, write_file.NewWriteFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	// 3. Comando de Terminal (com suporte a background exit)
	termTool := terminal_command.NewTerminalCommandTool(cfg.WorkspacePath, cfg.BlockedCommands, cfg.TerminalOutput)
	if cfg.OnBackgroundExit != nil {
		termTool.SetOnBackgroundExit(cfg.OnBackgroundExit)
	}
	if cfg.StateManager != nil {
		termTool.SetStateManager(cfg.StateManager)
	}
	list = append(list, termTool)

	// 4. Edição, renomeação, deleção e navegação de arquivos
	list = append(list, edit_file.NewEditFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, edit_file_by_line.NewEditFileByLineTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, undo_last_edit.NewUndoLastEditTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, view_function.NewViewFunctionTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, rename_file.NewRenameFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, delete_file.NewDeleteFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, tree.NewTreeTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, grep.NewGrepTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, find_file.NewFindFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	// 5. Port monitor & Git Tools
	list = append(list, port_monitor.NewPortMonitorTool(cfg.WorkspacePath))
	list = append(list, git_status.NewGitStatusTool(cfg.WorkspacePath))
	list = append(list, git_log.NewGitLogTool(cfg.WorkspacePath))
	list = append(list, git_diff.NewGitDiffTool(cfg.WorkspacePath))
	list = append(list, git_diff_summary.NewGitDiffSummaryTool(cfg.WorkspacePath))
	list = append(list, git_add.NewGitAddTool(cfg.WorkspacePath))
	list = append(list, git_commit.NewGitCommitTool(cfg.WorkspacePath))
	list = append(list, git_branch.NewGitBranchTool(cfg.WorkspacePath))
	list = append(list, git_conflict.NewGitConflictTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	// 6. Network & Web Scraping
	list = append(list, http_client.NewHTTPClientTool(cfg.WorkspacePath))
	list = append(list, scraper.NewScraperTool(cfg.WorkspacePath))

	// 7. Navegadores (se fornecidos externamente)
	if cfg.BrowserTool != nil {
		list = append(list, cfg.BrowserTool)
	}
	if cfg.SubagentTool != nil {
		list = append(list, cfg.SubagentTool)
	}

	// 8. Ferramentas do sistema e engenharia de software
	list = append(list, computer_control.NewComputerControlTool(cfg.WorkspacePath))
	list = append(list, database_tester.NewDatabaseTesterTool(cfg.WorkspacePath))
	list = append(list, proxy.NewProxyTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, run_tests.NewRunTestsTool(cfg.WorkspacePath))
	list = append(list, run_browser_test.NewRunBrowserTestTool(cfg.WorkspacePath, true))
	list = append(list, stack_translator.NewStackTranslatorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, doc_generator.NewDocGeneratorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, code_explainer.NewCodeExplainerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, mock_generator.NewMockGeneratorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, complexity_reducer.NewComplexityReducerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, memory_leak_scanner.NewMemoryLeakScannerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, bug_explainer.NewBugExplainerTool(cfg.WorkspacePath, cfg.LLMProvider))
	list = append(list, checkpoint.NewCheckpointTool(cfg.WorkspacePath, cfg.StateManager))
	list = append(list, refactor_auditor.NewRefactorAuditorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, autofix.NewAutofixTool(cfg.WorkspacePath, cfg.WorkspaceJail, cfg.LLMProvider))
	list = append(list, syntax_check.NewSyntaxCheckTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	if cfg.StateManager != nil {
		list = append(list, manage_plan.NewManagePlanTool(cfg.WorkspacePath, cfg.StateManager))
	}

	list = append(list, read_session_messages.NewReadSessionMessagesTool(cfg.WorkspacePath))
	list = append(list, record_decision.NewRecordDecisionTool(cfg.WorkspacePath))

	// Novas ferramentas nativas avançadas
	list = append(list, ast_analyzer.NewASTAnalyzerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, pytest_isolated.NewPytestIsolatedTool(cfg.WorkspacePath))
	list = append(list, call_graph.NewCallGraphTool(cfg.WorkspacePath))
	list = append(list, concurrency_lock.NewConcurrencyLockTool(cfg.WorkspacePath))
	list = append(list, cost_estimator.NewCostEstimatorTool(cfg.WorkspacePath, cfg.StateManager))
	list = append(list, dependency_graph.NewDependencyGraphTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, error_histogram.NewErrorHistogramTool(cfg.WorkspacePath, cfg.StateManager))
	list = append(list, git_diff_advanced.NewGitDiffAdvancedTool(cfg.WorkspacePath))
	list = append(list, import_validator.NewImportValidatorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, inject_local_env.NewInjectLocalEnvTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, legacy_matcher.NewLegacyMatcherTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, mttr_report.NewMTTRReportTool(cfg.WorkspacePath, cfg.StateManager))
	list = append(list, read_log_paginated.NewReadLogPaginatedTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, semantic_search.NewSemanticSearchTool(cfg.WorkspacePath))
	list = append(list, signature_validator.NewSignatureValidatorTool(cfg.WorkspacePath))

	return list
}
