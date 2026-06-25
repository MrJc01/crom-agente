import re

with open("internal/loop/agentic/core/execute.go", "r") as f:
    content = f.read()

# Add imports
imports = """
	"github.com/crom/crom-agente/internal/i18n"
	"github.com/crom/crom-agente/internal/loop/agentic/prompting"
	"github.com/crom/crom-agente/internal/loop/agentic/workspace"
	"github.com/crom/crom-agente/internal/loop/agentic/tooling"
"""
content = content.replace('"github.com/crom/crom-agente/internal/state"', '"github.com/crom/crom-agente/internal/state"\n' + imports)

# Replace method calls
content = content.replace("al.OptimizePrompt(ctx, intent)", "prompting.OptimizePrompt(ctx, al.provider, intent)")
content = content.replace("al.detectStack(workspaceDir)", "workspace.DetectStack(workspaceDir)")
content = content.replace("al.loadLocalRules(workspaceDir)", "workspace.LoadLocalRules(workspaceDir)")
content = content.replace("al.compactMessages(ctx, runMessages)", "prompting.CompactMessages(ctx, al.provider, al.config.MaxMessageHistory, al.handler, runMessages)")
content = content.replace("al.buildRequestOptions(intent)", "tooling.BuildRequestOptions(al.tools, intent)")
content = content.replace("formatToolError(toolID, errMsg)", "tooling.FormatToolError(toolID, errMsg)")
content = content.replace("formatToolError(toolID, execErr.Error())", "tooling.FormatToolError(toolID, execErr.Error())")
content = content.replace("al.renderDiffZone(tc.Function.Arguments, toolID, workspaceDir)", "tooling.RenderDiffZone(al.handler, workspaceDir, tc.Function.Arguments, toolID)")
content = content.replace("detectRepetitiveLoop(messages)", "DetectRepetitiveLoop(messages)") # now in core/loop_detector.go

# Replace i18n strings
content = content.replace('"Loop cancelado pelo contexto."', 'i18n.Get("system.loop_canceled")')
content = content.replace('" Loop cancelado pelo contexto"', 'i18n.Get("system.loop_canceled")')
content = content.replace('fmt.Sprintf("Prompt otimizado pelo Agente:\\n%s", optimized)', 'i18n.Get("system.optimized_prompt_log", optimized)')
content = content.replace('fmt.Sprintf("[SYSTEM STACK DETECTED] A stack técnica deste projeto foi identificada como: %s. Priorize comandos e validações desta stack.", stack)', 'i18n.Get("system.stack_detected", stack)')
content = content.replace('fmt.Sprintf("[SYSTEM LOCAL RULES] Respeite estritamente as seguintes regras do workspace:\\n\\n%s", localRules)', 'i18n.Get("system.local_rules", localRules)')
content = content.replace('fmt.Sprintf("[SYSTEM SESSION ISOLATION] Qualquer arquivo de planejamento interno adicional (exceto o plan.md automático), scripts temporários internos do agente, rascunhos de testes ou checklists de tarefas internas (como task.md) devem ser salvos OBRIGATORIAMENTE dentro do diretório desta sessão: %s/. No entanto, arquivos de código fonte do projeto, capturas de tela/imagens solicitadas pelo usuário, relatórios finais ou quaisquer ativos/entregáveis que façam parte do projeto do usuário DEVEM ser salvos na pasta raiz do workspace ou no caminho explicitamente solicitado pelo usuário, e NÃO na pasta da sessão.", displayDir)', 'i18n.Get("system.session_isolation", displayDir)')
content = content.replace('" Loop repetitivo detectado. Injetando correção."', 'i18n.Get("system.repetitive_loop_detected")')
content = content.replace('" [REPETITIVE_LOOP_WARNING] Você está repetindo ações anteriores. Mude sua estratégia imediatamente."', 'i18n.Get("system.repetitive_correction_prompt", 3)')
content = content.replace('fmt.Sprintf("Erro na requisição ao LLM: %s", errMsg)', 'i18n.Get("errors.llm_error", i+1) + ": " + errMsg')
content = content.replace('"O modelo retornou uma resposta em branco."', 'i18n.Get("errors.optimizer_blank_response")')
content = content.replace('fmt.Sprintf("Abortando: %d falhas consecutivas.", al.config.MaxConsecutiveFail)', 'i18n.Get("errors.abort_consecutive_failures", al.config.MaxConsecutiveFail)')
content = content.replace('fmt.Sprintf("Ferramenta \'%s\' não encontrada.", toolID)', 'i18n.Get("errors.tool_not_found", toolID)')
content = content.replace('fmt.Sprintf("Exceção na ferramenta %s: %s", toolID, execErr.Error())', 'i18n.Get("errors.tool_execution_failed", toolID) + ": " + execErr.Error()')

with open("internal/loop/agentic/core/execute.go", "w") as f:
    f.write(content)

