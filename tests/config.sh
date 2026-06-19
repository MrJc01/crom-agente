#!/bin/bash
# ══════════════════════════════════════════════════════════════
# config.sh — Configuração compartilhada dos testes E2E
# ══════════════════════════════════════════════════════════════

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"

# Caminho do binário compilado
CROM_BIN="$TESTS_DIR/.bin/crom-agente"

# Redireciona o HOME para uma pasta local de testes para isolar as execuções.
# Isso força o agente a executar em modo standalone (sem usar o daemon ativo do usuário)
# e evita efeitos colaterais no HOME real do usuário.
REAL_HOME="${REAL_HOME:-$HOME}"
export HOME="$TESTS_DIR/.home"
mkdir -p "$HOME/.crom"

if [[ -f "$REAL_HOME/.crom/.env" && "$(realpath "$REAL_HOME/.crom/.env")" != "$(realpath "$HOME/.crom/.env")" ]]; then
    cp "$REAL_HOME/.crom/.env" "$HOME/.crom/.env"
fi
if [[ -f "$REAL_HOME/.crom/global.json" && "$(realpath "$REAL_HOME/.crom/global.json")" != "$(realpath "$HOME/.crom/global.json")" ]]; then
    cp "$REAL_HOME/.crom/global.json" "$HOME/.crom/global.json"
fi

# Remove sockets órfãos no HOME temporário para garantir que sempre rode standalone
rm -f "$HOME/.crom/crom-agente.sock" "$HOME/.crom/crom-agente.pid"

# Compartilha caches/pacotes do usuário real para evitar downloads redundantes e erros de ambiente
export PYTHONUSERBASE="$REAL_HOME/.local"
export CARGO_HOME="$REAL_HOME/.cargo"
export GOPATH="${GOPATH:-$REAL_HOME/go}"
export GOCACHE="${GOCACHE:-$REAL_HOME/.cache/go-build}"
export npm_config_cache="$REAL_HOME/.npm"

# === Configurações do Agente para Testes ===
# Pode ser sobrescrito via variáveis de ambiente antes de chamar run_all.sh

# Modo de permissão: total_access para execução não-interativa
CROM_PERMISSION_MODE="${CROM_PERMISSION_MODE:-total_access}"

# Máximo de iterações do loop ReAct por cenário
CROM_MAX_ITERATIONS="${CROM_MAX_ITERATIONS:-15}"

# Timeout por ferramenta (segundos)
CROM_TIMEOUT="${CROM_TIMEOUT:-30}"

# Limite de histórico de mensagens
CROM_MAX_HISTORY="${CROM_MAX_HISTORY:-50}"

# Máximo de falhas consecutivas
CROM_MAX_FAILURES="${CROM_MAX_FAILURES:-3}"

# Provider/Model override (vazio = usa global.json)
CROM_PROVIDER="${CROM_PROVIDER:-}"
CROM_MODEL="${CROM_MODEL:-}"

# Flag para habilitar/desabilitar execução com modelo real
# Se false, apenas setup + validate são executados (sem agente)
USE_REAL_MODEL="${USE_REAL_MODEL:-true}"

# Timeout total por cenário (em segundos) — mata o agente se ultrapassar
SCENARIO_TIMEOUT="${SCENARIO_TIMEOUT:-300}"

# === Helper: Extrair tarefa principal do tasks.md ===
# Extrai a primeira "Tarefa 1" de cada tasks.md como prompt para o agente
extract_task() {
    local tasks_file="$1"
    if [[ ! -f "$tasks_file" ]]; then
        echo ""
        return
    fi

    # Busca o conteúdo da "Tarefa 1" — do heading ### Tarefa 1 até o próximo ### 
    local task_text
    task_text=$(awk '
        /^### Tarefa 1/{found=1; next}
        /^### Tarefa [2-9]/{if(found) exit}
        found{print}
    ' "$tasks_file" | sed '/^$/d' | head -10)

    echo "$task_text"
}

# === Helper: Construir comando do agente ===
build_agent_cmd() {
    local workspace_dir="$1"
    local task="$2"

    local cmd="$CROM_BIN run"
    cmd="$cmd \"$task\""
    cmd="$cmd --workspace \"$workspace_dir\""
    cmd="$cmd --permission-mode $CROM_PERMISSION_MODE"
    cmd="$cmd --max-iterations $CROM_MAX_ITERATIONS"
    cmd="$cmd --timeout $CROM_TIMEOUT"
    cmd="$cmd --max-history $CROM_MAX_HISTORY"
    cmd="$cmd --max-failures $CROM_MAX_FAILURES"

    if [[ -n "$CROM_PROVIDER" ]]; then
        cmd="$cmd --provider $CROM_PROVIDER"
    fi
    if [[ -n "$CROM_MODEL" ]]; then
        cmd="$cmd --model $CROM_MODEL"
    fi

    echo "$cmd"
}
