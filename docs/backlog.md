# Backlog de Funcionalidades - Crom Agent

Este documento registra as ideias, melhorias e requisitos propostos pelo usuário para implementação futura no ecossistema **Crom Agent**.

---

## 🚀 Funcionalidades Prioritárias e Futuras

### 1. Formatação de Resultados em Markdown & Visualização de Grafos
* **Descrição**: Formatar a saída de planos e execuções no chat usando Markdown enriquecido.
* **Recursos**:
  * Renderizar grafos de dependência das tarefas (com Mermaid ou bibliotecas similares de grafos).
  * Exibição clara de blocos de código e scripts de terminal fáceis de copiar.
  * Botão de cópia rápida para comandos de terminal.
  * Integração de scripts executados com referência direta (`@`) no chat.

### 2. Seleção de Texto Interativa no Chat (Floating "@" Context Tooltip)
* **Descrição**: Permitir a seleção livre de qualquer trecho de texto nas mensagens do chat.
* **Recursos**:
  * Ao selecionar um texto, exibir um tooltip flutuante próximo à seleção com a opção **"Mandar como @ context"**.
  * Clicar no botão insere o texto selecionado diretamente como anexo de contexto específico no campo de entrada do chat.
  * Permitir que qualquer trecho da conversa atual seja referenciado diretamente sem precisar reescrever.

### 3. Customização da Personalidade do Modelo (System Prompt Creator/Editor)
* **Descrição**: Interface visual para gerenciar o Prompt de Sistema do agente.
* **Recursos**:
  * Botão na lista do menu "+" que abre um dropdown para selecionar personalidades existentes.
  * Opção de criar ou editar personalidades através de um popup com formulário.
  * Armazenar as personalidades no arquivo de configuração do workspace ou de forma global para reuso do agente.

---

## 🛠️ Modificações Realizadas na Sessão Atual

* **Aprovação Automática (HITL)**: Criado o botão "+" no chat input que abre o dropdown rápido permitindo alternar a aprovação automática de comandos e ferramentas locais (HITL), enviando o estado `auto_approve` via WebSocket para o daemon em tempo real.
* **Sincronização Automática do Explorador de Arquivos**: Adicionado um trigger automático de recarga da árvore de arquivos sempre que o agente executa e retorna o resultado de uma ferramenta (`tool_result` com sucesso) ou finaliza uma tarefa (`finished`), além de adicionar um botão de atualização manual (`RefreshCw`) no topo do painel do Explorer.
* **Persistência de Sessões**: Correção do bug onde o recarregamento da página (reload) perdia o histórico da conversa. As sessões de chat e a sessão ativa são sincronizadas e carregadas do `localStorage` no startup.
