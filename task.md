# Tarefas - Arquitetura Modular de Agentes e Especialistas

Este é o checklist granular com mais de 200 etapas para implementar a nova arquitetura do CROM-Agente sem quebrar o sistema atual.

## Fase 1: Fundação e Reorganização de Pastas
- `[ ]` 1. Criar o diretório `internal/agents` na raiz do projeto.
- `[ ]` 2. Criar subdiretório `internal/agents/core` para as abstrações centrais.
- `[ ]` 3. Criar subdiretório `internal/agents/specialists` para os agentes especialistas.
- `[ ]` 4. Criar subdiretório `internal/agents/supervisor` para o agente orquestrador principal.
- `[ ]` 5. Mover o diretório inteiro de `internal/tools/browser_subagent` para `internal/agents/specialists/browser`.
- `[ ]` 6. Mover o arquivo `spawn_subagent.go` para `internal/agents/specialists/spawn`.
- `[ ]` 7. Atualizar o `package` em `internal/agents/specialists/browser/browser_subagent.go` para `browser`.
- `[ ]` 8. Atualizar o `package` em `internal/agents/specialists/spawn/spawn_subagent.go` para `spawn`.
- `[ ]` 9. Atualizar imports de `browser_subagent` no registro central de ferramentas (`internal/tools/registry.go`).
- `[ ]` 10. Atualizar imports de `spawn_subagent` no registro central de ferramentas (`internal/tools/registry.go`).
- `[ ]` 11. Atualizar testes unitários referentes ao `browser_subagent` com novos paths e pacotes.
- `[ ]` 12. Atualizar testes unitários referentes ao `spawn_subagent` com novos paths e pacotes.
- `[ ]` 13. Executar testes para validar que a movimentação base não quebrou ferramentas.
- `[ ]` 14. Corrigir eventuais erros de linting de pacotes movidos.
- `[ ]` 15. Atualizar chamadas do `browser_subagent` no `Manager` (caso existam referências diretas).
- `[ ]` 16. Atualizar referências no `cmd/` que dependam destas ferramentas, se aplicável.
- `[ ]` 17. Excluir as pastas vazias que restarem em `internal/tools/`.
- `[ ]` 18. Verificar integridade da compilação inicial `go build ./...`.
- `[ ]` 19. Rodar `golangci-lint run`.
- `[ ]` 20. Fazer commit da movimentação de base.

## Fase 2: Core Abstractions (Abstrações do Agente)
- `[ ]` 21. Criar arquivo `internal/agents/core/agent.go`.
- `[ ]` 22. Definir a struct `AgentResult` com campos `Output string` e `ContextSummary string`.
- `[ ]` 23. Definir a interface `Agent` com método `Execute(ctx, prompt, priorSummary) (AgentResult, error)`.
- `[ ]` 24. Definir interface `Agent` com método `Name() string`.
- `[ ]` 25. Definir interface `Agent` com método `Description() string`.
- `[ ]` 26. Definir interface `Agent` com método `SystemPrompt() string`.
- `[ ]` 27. Criar arquivo `internal/agents/core/agent_test.go` para mock definitions.
- `[ ]` 28. Criar um MockAgent em `agent_test.go` que implementa a interface `Agent`.
- `[ ]` 29. Implementar retorno dummy no `MockAgent.Execute`.
- `[ ]` 30. Implementar retorno dummy no `MockAgent.Name`.
- `[ ]` 31. Implementar retorno dummy no `MockAgent.Description`.
- `[ ]` 32. Criar struct base abstrata `BaseAgent` em `internal/agents/core/base_agent.go`.
- `[ ]` 33. Implementar campos comuns em `BaseAgent` (Name, Description, LLMProvider).
- `[ ]` 34. Adicionar getter para Provider no `BaseAgent`.
- `[ ]` 35. Criar testes para instanciar e testar campos do `BaseAgent` em `base_agent_test.go`.
- `[ ]` 36. Definir struct de metadados `AgentMetadata` em `internal/agents/core/agent.go` (versão, autor, versão MCP).
- `[ ]` 37. Criar método em `BaseAgent` para validar o provider configurado (`Validate() error`).
- `[ ]` 38. Escrever teste para validação de provedor vazio no `BaseAgent`.
- `[ ]` 39. Revisar interface com suporte para injetar `tools.Tool` customizadas no especialista.
- `[ ]` 40. Adicionar método `Tools() []tools.Tool` na interface `Agent`.

## Fase 3: Camada Adaptadora (Agent as a Tool)
- `[ ]` 41. Criar arquivo `internal/tools/agent_tool_adapter.go`.
- `[ ]` 42. Importar o pacote `internal/agents/core` no adapter.
- `[ ]` 43. Criar a struct `AgentToolAdapter` que embute a interface `tools.Tool`.
- `[ ]` 44. Adicionar campo `InnerAgent core.Agent` na struct `AgentToolAdapter`.
- `[ ]` 45. Implementar o método `Execute(ctx context.Context, input json.RawMessage)` no Adapter.
- `[ ]` 46. Adicionar decodificação do input JSON para uma struct interna contendo `Prompt` e `PriorSummary`.
- `[ ]` 47. Fazer o método `Execute` chamar o `InnerAgent.Execute()`.
- `[ ]` 48. Estruturar o retorno formatado em JSON contendo a resposta e o novo summary.
- `[ ]` 49. Implementar tratamento de erro no Adapter ao fazer parse do JSON de entrada.
- `[ ]` 50. Implementar o método `Name()` no Adapter, retornando `InnerAgent.Name()`.
- `[ ]` 51. Implementar o método `Description()` no Adapter, retornando a descrição e a indicação de como enviar o Summary.
- `[ ]` 52. Definir o `InputSchema` do Adapter utilizando jsonschema estrito para o LLM entender os campos esperados (prompt, session_context).
- `[ ]` 53. Criar arquivo de teste `internal/tools/agent_tool_adapter_test.go`.
- `[ ]` 54. Escrever teste para `Adapter.Execute` em caso de sucesso.
- `[ ]` 55. Escrever teste para `Adapter.Execute` com JSON malformado.
- `[ ]` 56. Escrever teste para `Adapter.Execute` garantindo que o ContextSummary é retornado no JSON final.
- `[ ]` 57. Validar log de execução no Adapter.
- `[ ]` 58. Integrar telemetria ou logging básico da interface Tool no Adapter.
- `[ ]` 59. Validar que o Adapter atende perfeitamente a assinatura `tools.Tool` do sistema atual.
- `[ ]` 60. Fazer testes de compilação da camada de Adapter.

## Fase 4: O Supervisor e a Lógica de Sessão / Memória
- `[ ]` 61. Criar o diretório `internal/agents/supervisor`.
- `[ ]` 62. Criar `supervisor.go`.
- `[ ]` 63. Mover a lógica principal de invocação e ReAct loop para dentro da arquitetura Supervisor (ou manter interligado ao Manager atual).
- `[ ]` 64. Criar struct `SupervisorAgent` em `supervisor.go`.
- `[ ]` 65. Criar arquivo para gerenciamento de memória em sessão local `internal/session/memory.go`.
- `[ ]` 66. Definir struct `AgentSessionState` que armazena pares de chave-valor: `SubagentName -> ContextSummary`.
- `[ ]` 67. Criar método `GetSummaryForAgent(name string) string` em `memory.go`.
- `[ ]` 68. Criar método `UpdateSummaryForAgent(name, summary string)` em `memory.go`.
- `[ ]` 69. Conectar `memory.go` ao sistema de persistência de sessão `.crom/sessions/<id>`.
- `[ ]` 70. Atualizar a rotina de salvar sessão no disco para incluir os subagents summaries.
- `[ ]` 71. Atualizar a rotina de carregar sessão do disco para restaurar as memórias.
- `[ ]` 72. Criar testes unitários para a serialização JSON da `AgentSessionState`.
- `[ ]` 73. Criar testes unitários para o `GetSummaryForAgent`.
- `[ ]` 74. Criar testes unitários para o `UpdateSummaryForAgent`.
- `[ ]` 75. Alterar o fluxo do Supervisor: antes de chamar um subagente, invocar `GetSummaryForAgent`.
- `[ ]` 76. Injetar este summary no prompt da chamada do Adapter.
- `[ ]` 77. Após o subagente retornar, extrair o novo summary do JSON.
- `[ ]` 78. Salvar o novo summary via `UpdateSummaryForAgent`.
- `[ ]` 79. Validar se o loop de retentativas do ReAct trata corretamente falhas de serialização da memória.
- `[ ]` 80. Criar mocks para teste ponta-a-ponta do ciclo de memória.

## Fase 5: Mecanismo de Compressão de Contexto (O "Cérebro" do Especialista)
- `[ ]` 81. Criar `internal/agents/core/summarizer.go`.
- `[ ]` 82. Adicionar uma função utilitária `CompressHistory(llm Provider, fullHistory, newResult) string`.
- `[ ]` 83. Criar o template de prompt do summarizer em `strings.json` (i18n).
- `[ ]` 84. Definir a chave `system.agents.summarizer_prompt` no i18n para PT-BR.
- `[ ]` 85. Definir a chave `system.agents.summarizer_prompt` no i18n para EN.
- `[ ]` 86. Implementar a lógica de chamada interna de LLM no summarizer.
- `[ ]` 87. Tratar erros de Timeout do provider ao sumarizar.
- `[ ]` 88. Configurar max_tokens de saída baixo para a sumarização (para economizar e forçar síntese).
- `[ ]` 89. Criar testes unitários mockando o provedor no Summarizer.
- `[ ]` 90. Incorporar a chamada do Summarizer no ciclo final de execução da interface `Agent.Execute()` padrão.

## Fase 6: Topologias Baseadas em JSON (Config Engine)
- `[ ]` 91. Criar pacote `internal/config/topology`.
- `[ ]` 92. Definir a struct `TopologyConfig` contendo o Supervisor e uma lista de Specialists.
- `[ ]` 93. Definir campos para Especialistas: Name, Type (Native/MCP), ExecPath ou URI.
- `[ ]` 94. Criar parser para `crom_agents.json`.
- `[ ]` 95. Adicionar função `LoadTopology(path string) (*TopologyConfig, error)`.
- `[ ]` 96. Testar `LoadTopology` com um arquivo JSON válido.
- `[ ]` 97. Testar `LoadTopology` com um arquivo inexistente (fallbacks para topologia default).
- `[ ]` 98. Testar `LoadTopology` com JSON sintaticamente incorreto.
- `[ ]` 99. Definir arquivo padrão na inicialização do repositório em `.crom/crom_agents.json`.
- `[ ]` 100. Implementar suporte a variáveis de ambiente dentro do arquivo JSON (ex: `$CROM_MCP_PATH`).
- `[ ]` 101. Configurar validação de schema do JSON.
- `[ ]` 102. Criar método `GetSpecialists() []SpecialistConfig`.
- `[ ]` 103. Criar função para injetar a Topologia instanciada no `Manager`.
- `[ ]` 104. Modificar estrutura global `Config` para referenciar o caminho da Topology.
- `[ ]` 105. Escrever bateria de testes para manipulação de variáveis de ambiente no parser JSON.

## Fase 7: Refatoração do Orquestrador (Manager)
- `[ ]` 106. Atualizar a injeção de dependências do `internal/orchestrator/manager.go`.
- `[ ]` 107. Substituir a inicialização hardcoded de tools pela leitura da Topologia.
- `[ ]` 108. Criar função `BootstrapAgents(topo *TopologyConfig)` no Manager.
- `[ ]` 109. Percorrer os Specialists Native configurados e instanciá-los via Reflection ou Map Registration.
- `[ ]` 110. Criar um Registro Global de Especialistas Nativos (`internal/agents/registry.go`).
- `[ ]` 111. Adicionar `RegisterAgent(name string, factory func() core.Agent)`.
- `[ ]` 112. Registrar o `BrowserAgent` no registro de agentes nativos.
- `[ ]` 113. Registrar o `SpawnAgent` no registro de agentes nativos.
- `[ ]` 114. No Bootstrap, empacotar os instanciados com o `AgentToolAdapter`.
- `[ ]` 115. Adicionar os adaptadores criados à lista de tools ativas do Manager.
- `[ ]` 116. Tratar gracefully se o agente pedido no JSON não estiver registrado.
- `[ ]` 117. Escrever testes validando o fluxo Manager -> Bootstrap -> Tool Registration.
- `[ ]` 118. Verificar integridade dos contextos de concorrência na criação dinâmica.
- `[ ]` 119. Adaptar injeção do Logger no fluxo dos Agentes e Adapter.
- `[ ]` 120. Validar e adaptar o carregamento do Provider para os agentes nativos (devem compartilhar a instância do Manager).

## Fase 8: Suporte inicial a Agentes Externos (MCP / Binários Externos)
- `[ ]` 121. Criar struct `ExternalAgent` em `internal/agents/specialists/external`.
- `[ ]` 122. Fazer `ExternalAgent` implementar `core.Agent`.
- `[ ]` 123. Configurar comando de execução sub-processo via `os/exec` ou cliente MCP.
- `[ ]` 124. Definir protocolo de comunicação IPC / JSON-RPC simplificado caso não seja MCP estrito.
- `[ ]` 125. Adicionar timeout configurável para resposta do external agent.
- `[ ]` 126. Ler `stdout` do subprocesso e fazer parsing do resultado e contextSummary.
- `[ ]` 127. Tratar falhas do subprocesso (`stderr` panic/exit codes não zero).
- `[ ]` 128. Criar script Python ou Node.js simples no repositório de testes (`tests/test_agent.py`) para validar o pipe externo.
- `[ ]` 129. Escrever testes de integração do ExternalAgent chamando o script bash/python.
- `[ ]` 130. Atualizar o `BootstrapAgents` para reconhecer `Type: "External"` e instanciar `ExternalAgent`.

## Fase 9: Criação do Comandos CLI
- `[ ]` 131. Adicionar arquivo `cmd/agent.go`.
- `[ ]` 132. Criar comando Cobra principal `agent`.
- `[ ]` 133. Criar subcomando `agent list` para listar especialistas carregados na topologia.
- `[ ]` 134. Adicionar output formatado (tabela) para listar Name, Type, Description.
- `[ ]` 135. Criar subcomando `agent add` para injetar uma linha dinamicamente no `crom_agents.json`.
- `[ ]` 136. Definir flag `--type` (native|mcp|external) no `agent add`.
- `[ ]` 137. Definir flag `--path` no `agent add`.
- `[ ]` 138. Criar testes para o comando CLI com Cobra testing utilities.
- `[ ]` 139. Criar subcomando `agent remove` para deletar registro do JSON.
- `[ ]` 140. Criar subcomando `agent validate` para verificar consistência do arquivo JSON local.

## Fase 10: Atualização de System Prompts
- `[ ]` 141. Revisar `internal/i18n/strings.json`.
- `[ ]` 142. Atualizar a instrução do Orquestrador principal sobre as novas Tools que representam subagentes.
- `[ ]` 143. Adicionar regras claras sobre delegação: "Quando a tarefa exigir pesquisa profunda, use o Pesquisador; para navegação, use o Browser".
- `[ ]` 144. Configurar instruções sobre o `ContextSummary` no prompt do Supervisor: "Você passará o estado atual conhecido para o especialista se for a segunda iteração".
- `[ ]` 145. Criar system prompt para o BrowserAgent nativo utilizando i18n (`system.agents.browser_prompt`).
- `[ ]` 146. Criar system prompt para o CoderAgent (placeholder futuro).
- `[ ]` 147. Atualizar system prompt de tradução EN/PT para prompts do agente.
- `[ ]` 148. Ajustar testes de strings para cobrir os novos nós no JSON.
- `[ ]` 149. Validar que nenhuma regressão afetou prompts de terminal existentes.
- `[ ]` 150. Limpar mensagens depreciadas do spawn_subagent antigo.

## Fase 11: Implementação Real do Browser Specialist via Arquitetura Nova
- `[ ]` 151. Abrir `internal/agents/specialists/browser/browser_subagent.go`.
- `[ ]` 152. Modificar struct para implementar `core.Agent` em vez de `tools.Tool`.
- `[ ]` 153. Refatorar método de entrada para aceitar o prompt e contexto resumido.
- `[ ]` 154. Refatorar método para retornar o tipo `AgentResult`.
- `[ ]` 155. Extrair a lógica do ReAct browser para instanciar seu próprio micro-orquestrador internamente.
- `[ ]` 156. Injetar provedor LLM configurado (via BaseAgent) no micro-orquestrador.
- `[ ]` 157. Ao final do loop de navegação web, empacotar toda a trajetória HTML no mecanismo de Summarizer (Fase 5).
- `[ ]` 158. Retornar a conclusão do BrowserAgent e o resumo.
- `[ ]` 159. Rodar testes ponta a ponta do Browser via chamadas diretas.
- `[ ]` 160. Corrigir vazamento de goroutines em contextos abortados do browser_agent.

## Fase 12: Implementação do Spawn Specialist
- `[ ]` 161. Abrir `internal/agents/specialists/spawn/spawn_subagent.go`.
- `[ ]` 162. Modificar struct para implementar `core.Agent`.
- `[ ]` 163. Refatorar injeção do ContextSummary no prompt isolado (esse agente foca em rodar scripts/bash num terminal fechado).
- `[ ]` 164. Retornar os stdout logs compactados via Summarizer em vez da string inteira gigantesca.
- `[ ]` 165. Verificar testes de injeção e falhas.
- `[ ]` 166. Garantir que o comando executado no sandbox fecha ao cancelar o contexto global.

## Fase 13: Validação de Ponta a Ponta
- `[ ]` 167. Configurar um workspace teste em diretório temporário.
- `[ ]` 168. Iniciar o orquestrador do crom-agente apontando para o binário novo.
- `[ ]` 169. Simular o usuário pedindo uma tarefa complexa de navegação + console.
- `[ ]` 170. Validar nos logs (com flag `--debug`) que o Orquestrador chamou a tool do adapter.
- `[ ]` 171. Validar que o Adapter invocou o core.Agent correto.
- `[ ]` 172. Validar o log do LLM para a chamada de compressão/summarização (Summarizer).
- `[ ]` 173. Validar se o Supervisor salvou a chave do summary na sessão.
- `[ ]` 174. Enviar mensagem de follow-up pedindo pro subagente continuar.
- `[ ]` 175. Checar via logs se o histórico resumido foi passado corretamente como contexto na segunda chamada.
- `[ ]` 176. Forçar crash manual no subagente e ver se o Manager pega o panic e restaura a sessão.
- `[ ]` 177. Revisar e otimizar uso de CPU/RAM após chamadas pesadas.

## Fase 14: Polimento, Segurança e Clean Code
- `[ ]` 178. Analisar pacotes não mais necessários no `internal/tools`.
- `[ ]` 179. Remover referências antigas de struct properties mortas.
- `[ ]` 180. Documentar cada Interface e método exportado usando docstrings Go padrão.
- `[ ]` 181. Documentar `crom_agents.json` schema no README ou docs.
- `[ ]` 182. Atualizar CHANGELOG.md do SDK e Core sobre a arquitetura.
- `[ ]` 183. Rodar scanner de segurança e vet nos novos pacotes usando `go vet`.
- `[ ]` 184. Padronizar tratamento de Context errors (`context.Canceled`, `context.DeadlineExceeded`).
- `[ ]` 185. Otimizar chamadas concorrentes no CLI usando error groups.
- `[ ]` 186. Verificar conformidade de chaves de licença caso haja agentes proprietários mapeados no json.
- `[ ]` 187. Checar a estrutura e imports cíclicos (ex: agents não deve importar Manager).
- `[ ]` 188. Validar interface de Logging Customizado e se os subagentes não bagunçam o terminal stdout do UI principal.
- `[ ]` 189. Ocultar verbosidade do log do Summarizer do usuário final, mantendo apenas logs de debug.
- `[ ]` 190. Fechar os últimos apontamentos do linter estático.

## Fase 15: Entregáveis e Documentação SDK
- `[ ]` 191. Criar um documento em Markdown (`docs/architecture/subagents.md`) detalhando como terceiros desenvolvem novos especialistas via binário ou MCP.
- `[ ]` 192. Fornecer um exemplo copy-paste de código Go em como registrar um Agente Nativo via `Registry`.
- `[ ]` 193. Fornecer exemplo de como estender o arquivo JSON com um `ExternalAgent` em Python.
- `[ ]` 194. Criar scripts de inicialização de template caso aplicável.
- `[ ]` 195. Gravar a saída da execução da topologia e validar que a listagem via CLI está bela e formatada.
- `[ ]` 196. Finalizar revisão final de code coverage (garantir > 80% nos pacotes `core` e `supervisor`).
- `[ ]` 197. Executar testes de compatibilidade retroativa - sessões antigas salvas (sem agent state) devem carregar graciosamente sem panicar.
- `[ ]` 198. Verificar a build final no macOS.
- `[ ]` 199. Verificar a build final em Linux.
- `[ ]` 200. Fazer push da feature e fechar o escopo arquitetural.
- `[ ]` 201. Realizar merge com a branch principal após a aprovação final.
