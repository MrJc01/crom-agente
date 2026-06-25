package registry

import (
	"io"

	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools"
	"github.com/crom/crom-agente/internal/tools/ask_user"
	"github.com/crom/crom-agente/internal/tools/code_explainer"
	"github.com/crom/crom-agente/internal/tools/complexity_reducer"
	"github.com/crom/crom-agente/internal/tools/computer_control"
	"github.com/crom/crom-agente/internal/tools/database_tester"
	"github.com/crom/crom-agente/internal/tools/delete_file"
	"github.com/crom/crom-agente/internal/tools/diff_replace"
	"github.com/crom/crom-agente/internal/tools/doc_generator"
	"github.com/crom/crom-agente/internal/tools/git_add"
	"github.com/crom/crom-agente/internal/tools/git_branch"
	"github.com/crom/crom-agente/internal/tools/git_commit"
	"github.com/crom/crom-agente/internal/tools/git_conflict"
	"github.com/crom/crom-agente/internal/tools/git_diff"
	"github.com/crom/crom-agente/internal/tools/git_log"
	"github.com/crom/crom-agente/internal/tools/git_status"
	"github.com/crom/crom-agente/internal/tools/grep"
	"github.com/crom/crom-agente/internal/tools/http_client"
	"github.com/crom/crom-agente/internal/tools/manage_plan"
	"github.com/crom/crom-agente/internal/tools/memory_leak_scanner"
	"github.com/crom/crom-agente/internal/tools/mock_generator"
	"github.com/crom/crom-agente/internal/tools/port_monitor"
	"github.com/crom/crom-agente/internal/tools/proxy"
	"github.com/crom/crom-agente/internal/tools/read_file"
	"github.com/crom/crom-agente/internal/tools/rename_file"
	"github.com/crom/crom-agente/internal/tools/run_tests"
	"github.com/crom/crom-agente/internal/tools/schedule_timer"
	"github.com/crom/crom-agente/internal/tools/scraper"
	"github.com/crom/crom-agente/internal/tools/stack_translator"
	"github.com/crom/crom-agente/internal/tools/terminal_command"
	"github.com/crom/crom-agente/internal/tools/tree"
	"github.com/crom/crom-agente/internal/tools/write_file"
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
	BrowserTool  tools.Tool
	SubagentTool tools.Tool
	StateManager *state.StateManager
}

// GetBuiltinTools retorna a lista completa de ferramentas nativas instanciadas e prontas para registro
func GetBuiltinTools(cfg RegistrationConfig) []tools.Tool {
	var list []tools.Tool

	// 1. Ferramenta de agendamento e interacao
	list = append(list, schedule_timer.NewScheduleTimerTool(cfg.WorkspacePath, cfg.OnSchedule))
	list = append(list, ask_user.NewAskUserTool(cfg.WorkspacePath))

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
	list = append(list, diff_replace.NewDiffReplaceTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, rename_file.NewRenameFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, delete_file.NewDeleteFileTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, tree.NewTreeTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, grep.NewGrepTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	// 5. Port monitor & Git Tools
	list = append(list, port_monitor.NewPortMonitorTool(cfg.WorkspacePath))
	list = append(list, git_status.NewGitStatusTool(cfg.WorkspacePath))
	list = append(list, git_log.NewGitLogTool(cfg.WorkspacePath))
	list = append(list, git_diff.NewGitDiffTool(cfg.WorkspacePath))
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
	list = append(list, stack_translator.NewStackTranslatorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, doc_generator.NewDocGeneratorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, code_explainer.NewCodeExplainerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, mock_generator.NewMockGeneratorTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, complexity_reducer.NewComplexityReducerTool(cfg.WorkspacePath, cfg.WorkspaceJail))
	list = append(list, memory_leak_scanner.NewMemoryLeakScannerTool(cfg.WorkspacePath, cfg.WorkspaceJail))

	if cfg.StateManager != nil {
		list = append(list, manage_plan.NewManagePlanTool(cfg.WorkspacePath, cfg.StateManager))
	}

	return list
}
