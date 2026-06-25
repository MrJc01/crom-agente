package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/llm/providers"
	"github.com/crom/crom-agente/internal/loop/agentic/core"
	"github.com/crom/crom-agente/internal/permission"
	"github.com/crom/crom-agente/internal/state"
	"github.com/crom/crom-agente/internal/tools/browser"
	"github.com/crom/crom-agente/internal/tools/registry"
)

type Options struct {
	WorkspacePath  string
	StoragePath    string
	SessionName    string
	Provider       string
	Model          string
	TimeoutSeconds int
	PermissionMode string
}

type TUIModel struct {
	options      Options
	stateManager *state.StateManager
	agentLoop    *core.AgenticLoop
	attachments  map[string]string // Nome do arquivo -> Conteúdo
	shouldExit   bool
	spinner      *InlineSpinner
	promptColor  string
	reader       *bufio.Reader
}

// InlineSpinner gerencia a renderização de um spinner assíncrono na linha atual do terminal
type InlineSpinner struct {
	mu       sync.Mutex
	active   bool
	status   string
	stopChan chan struct{}
}

func NewInlineSpinner() *InlineSpinner {
	return &InlineSpinner{
		stopChan: make(chan struct{}),
	}
}

func (s *InlineSpinner) Start(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		s.status = status
		return
	}
	s.active = true
	s.status = status
	s.stopChan = make(chan struct{})

	go func() {
		chars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			s.mu.Lock()
			if !s.active {
				s.mu.Unlock()
				return
			}
			statusText := s.status
			s.mu.Unlock()

			// Imprime o spinner e o status na linha atual, limpando a linha
			fmt.Printf("\r\033[K\033[1;36m%s\033[0m %s...", chars[i], statusText)
			i = (i + 1) % len(chars)

			select {
			case <-s.stopChan:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
}

func (s *InlineSpinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	close(s.stopChan)
	// Limpa a linha do spinner
	fmt.Print("\r\033[K")
}

func (s *InlineSpinner) Update(status string) {
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
}

// Start inicializa o REPL inline interactivo do crom-agente-cli
func Start(opts Options) error {
	// Capturar interrupções Ctrl+C de forma amigável
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println("\n\n\033[33mEncerrando sessão interativa. Até logo!\033[0m")
		os.Exit(0)
	}()

	model := &TUIModel{
		options:     opts,
		attachments: make(map[string]string),
		spinner:     NewInlineSpinner(),
		promptColor: "\033[1;36m", // Ciano padrão
		reader:      bufio.NewReader(os.Stdin),
	}

	// 1. Carrega o diretório global
	gDir, err := config.GlobalDir()
	if err != nil {
		return fmt.Errorf("falha ao obter diretório global: %w", err)
	}

	// 2. Carrega configuração global
	global, err := config.LoadGlobalConfig(gDir)
	if err != nil {
		return fmt.Errorf("falha ao carregar configuração global: %w", err)
	}

	// 3. Carrega variáveis do arquivo .env
	env, err := config.LoadEnvVars(gDir)
	if err != nil {
		return fmt.Errorf("falha ao carregar variáveis de ambiente: %w", err)
	}

	// 4. Carrega configuração do workspace
	workspace, err := config.LoadWorkspaceConfig(opts.WorkspacePath)
	if err != nil {
		return fmt.Errorf("falha ao carregar configuração do workspace: %w", err)
	}

	// 5. Configura flags de override
	var flags config.CLIFlags
	if opts.Provider != "" {
		flags.Provider = opts.Provider
	}
	if opts.Model != "" {
		flags.Model = opts.Model
	}
	if opts.TimeoutSeconds > 0 {
		flags.ToolTimeoutSeconds = &opts.TimeoutSeconds
	}
	if opts.PermissionMode != "" {
		flags.PermissionMode = opts.PermissionMode
	}

	resolved := config.Resolve(global, workspace, flags)

	// 6. Instancia o LLM Provider
	provider, err := providers.NewProvider(resolved.Provider, resolved.Model, func(key string) string {
		return env.Get(key)
	})
	if err != nil {
		return fmt.Errorf("falha ao criar provedor de LLM: %w", err)
	}

	// 7. Instancia o StateManager
	sm := state.NewSessionStateManager(opts.StoragePath, opts.SessionName)
	if err := sm.LoadState(); err != nil {
		return fmt.Errorf("falha ao carregar estado: %w", err)
	}
	model.stateManager = sm

	// 8. Inicializa o loop ReAct
	handler := &tuiEventHandler{spinner: model.spinner}
	al := core.New(provider, sm, handler, resolved)

	// Instanciar e pré-configurar o navegador
	browserTool := browser.NewBrowserTool(opts.WorkspacePath, resolved.BrowserHeadless)
	browserTool.SetOnNavigate(func(url string) {
		_ = sm.SetBrowserURL(url)
	})
	browserTool.SetRestoreURL(func() string {
		return sm.GetBrowserURL()
	})

	// Instanciar e registrar as ferramentas nativas unificadas via registro centralizado
	builtinTools := registry.GetBuiltinTools(registry.RegistrationConfig{
		WorkspacePath:   opts.WorkspacePath,
		WorkspaceJail:   resolved.WorkspaceJail,
		BlockedCommands: resolved.BlockedCommands,
		TerminalOutput:  nil,
		OnSchedule:      nil,
		BrowserTool:     browserTool,
	})

	for _, t := range builtinTools {
		al.RegisterTool(t)
	}

	// Configurar permission manager
	askFunc := func(ctx context.Context, action, target string) (bool, bool) {
		model.spinner.Stop()
		fmt.Printf("\n\033[33m⚠️  [HITL] crom-agente solicita permissão para a ação [%s] no alvo: %q\033[0m\n", action, target)
		fmt.Print("👉 Pressione \033[1;32m[a]\033[0m para aprovar uma vez, \033[1;36m[s]\033[0m para sempre permitir, \033[1;31m[r]\033[0m para rejeitar: ")

		response, err := model.reader.ReadString('\n')
		if err != nil {
			model.spinner.Start("Processando")
			return false, false
		}
		response = strings.TrimSpace(strings.ToLower(response))

		approved := false
		remember := false
		if response == "s" {
			approved = true
			remember = true
		} else if response == "a" {
			approved = true
		}

		model.spinner.Start("Processando")
		return approved, remember
	}
	pm := permission.NewPermissionManager(opts.WorkspacePath, resolved.PermissionMode, askFunc)
	al.SetPermissionManager(pm)
	model.agentLoop = al

	// Imprimir banner de inicialização
	fmt.Println("\033[1;35m════════════════════════════════════════════════════════════════════\033[0m")
	fmt.Printf("\033[1;35m  CROM AGENTE CLI v0.1.0 (REPL Mode)\033[0m\n")
	fmt.Printf("  Sessão: \033[1;36m%s\033[0m | Provedor: \033[1;36m%s\033[0m | Modelo: \033[1;36m%s\033[0m\n", opts.SessionName, resolved.Provider, resolved.Model)
	fmt.Println("  Digite \033[1;33m/help\033[0m para comandos ou \033[1;31m/exit\033[0m para sair.")
	fmt.Println("\033[1;35m════════════════════════════════════════════════════════════════════\033[0m")

	for {
		fmt.Printf("%scrom-agente >\033[0m ", model.promptColor)
		line, err := model.reader.ReadString('\n')
		if err != nil {
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Processar slash commands
		if strings.HasPrefix(input, "/") {
			resp, handled := HandleSlashCommand(input, model)
			if handled {
				fmt.Printf("\033[33m%s\033[0m\n", resp)
				if model.shouldExit {
					break
				}
				continue
			}
		}

		// Anexar arquivos de contexto se houver
		prompt := input
		if len(model.attachments) > 0 {
			var b strings.Builder
			for name, content := range model.attachments {
				b.WriteString(fmt.Sprintf("\n[Arquivo Anexado: %s]\n```\n%s\n```\n", name, content))
			}
			prompt = b.String() + "\n" + input
			model.attachments = make(map[string]string) // Consumido
			fmt.Println("📎 Arquivos anexados injetados no contexto do envio.")
		}

		// Rodar o loop de agente
		model.spinner.Start("Pensando")
		ctx := context.Background()
		err = model.agentLoop.Execute(ctx, prompt)
		model.spinner.Stop()

		if err != nil {
			fmt.Printf("\033[1;31mError: %v\033[0m\n", err)
		} else {
			fmt.Println("\033[1;32m✓ Execução concluída.\033[0m")
		}
	}

	return nil
}
