#!/bin/bash
# ══════════════════════════════════════════════════════════════
# run_all.sh — Executa todos os cenários de teste E2E do crom-agente
#              com modelo LLM REAL (não mock)
# ══════════════════════════════════════════════════════════════
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/.results"
mkdir -p "$RESULTS_DIR"

# Carrega configuração compartilhada
source "$SCRIPT_DIR/config.sh"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  🧪 crom-agente — Suite de Testes E2E com Modelo Real${NC}"
echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
echo ""

# === Verificar se o binário existe ===
if [[ ! -x "$CROM_BIN" ]]; then
    echo -e "${YELLOW}  ⚠ Binário não encontrado em $CROM_BIN${NC}"
    echo -e "${YELLOW}  ⚠ Executando build_agent.sh primeiro...${NC}"
    bash "$SCRIPT_DIR/build_agent.sh"
    if [[ ! -x "$CROM_BIN" ]]; then
        echo -e "${RED}  ✗ Falha ao compilar o binário. Abortando.${NC}"
        exit 1
    fi
fi

echo -e "  ⚙️  Configuração:"
echo -e "     Binário:       $CROM_BIN"
echo -e "     Modo:          $CROM_PERMISSION_MODE"
echo -e "     Max Iterações: $CROM_MAX_ITERATIONS"
echo -e "     Timeout:       ${CROM_TIMEOUT}s"
echo -e "     Modelo Real:   $USE_REAL_MODEL"
if [[ -n "$CROM_PROVIDER" ]]; then
    echo -e "     Provider:      $CROM_PROVIDER"
fi
if [[ -n "$CROM_MODEL" ]]; then
    echo -e "     Modelo:        $CROM_MODEL"
fi
echo ""

TOTAL=0
PASSED=0
FAILED=0
SKIPPED=0

for scenario_dir in "$SCRIPT_DIR"/*/; do
    scenario_name=$(basename "$scenario_dir")
    
    # Ignora diretórios não-cenário
    [[ "$scenario_name" == ".results" ]] && continue
    [[ "$scenario_name" == ".bin" ]] && continue
    [[ ! -f "$scenario_dir/setup.sh" ]] && continue
    
    # Se SCENARIO_FILTER estiver definido, executa apenas o cenário correspondente
    [[ -n "${SCENARIO_FILTER:-}" && "$scenario_name" != "$SCENARIO_FILTER" ]] && continue
    
    TOTAL=$((TOTAL + 1))
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}  Cenário: ${scenario_name}${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    # ─── 1. Setup ───
    echo -e "  📦 [1/3] Executando setup..."
    if bash "$scenario_dir/setup.sh" > "$RESULTS_DIR/${scenario_name}_setup.log" 2>&1; then
        echo -e "  ${GREEN}✓ Setup concluído${NC}"
    else
        echo -e "  ${RED}✗ Setup falhou (ver $RESULTS_DIR/${scenario_name}_setup.log)${NC}"
        FAILED=$((FAILED + 1))
        echo ""
        continue
    fi
    
    # ─── 2. Executar agente com modelo real ───
    if [[ "$USE_REAL_MODEL" == "true" ]]; then
        # Extrair tarefa do tasks.md
        TASK=$(extract_task "$scenario_dir/tasks.md" | sed 's/^[ -]*//')
        TASK="$TASK"$'\n\n'"IMPORTANTE: Escreva o código completo e salve os arquivos correspondentes no workspace. NÃO remova, limpe ou delete os arquivos criados ou modificados ao final da tarefa."
        
        if [[ -z "$TASK" ]]; then
            echo -e "  ${YELLOW}⚠ Sem tarefa extraível em tasks.md. Pulando execução do agente.${NC}"
        else
            echo -e "  ${MAGENTA}🤖 [2/3] Executando agente com modelo real...${NC}"
            echo -e "  ${MAGENTA}   Tarefa: $(echo "$TASK" | head -1 | cut -c1-80)...${NC}"
            
            AGENT_LOG="$RESULTS_DIR/${scenario_name}_agent.log"
            echo -e "  📝 Log: $AGENT_LOG"
            
            # Constrói os argumentos em um array para evitar eval de bash -c
            AGENT_ARGS=()
            AGENT_ARGS+=("run" "$TASK")
            AGENT_ARGS+=("--workspace" "$scenario_dir")
            AGENT_ARGS+=("--permission-mode" "$CROM_PERMISSION_MODE")
            AGENT_ARGS+=("--max-iterations" "$CROM_MAX_ITERATIONS")
            AGENT_ARGS+=("--timeout" "$CROM_TIMEOUT")
            AGENT_ARGS+=("--max-history" "$CROM_MAX_HISTORY")
            AGENT_ARGS+=("--max-failures" "$CROM_MAX_FAILURES")
            
            if [[ -n "$CROM_PROVIDER" ]]; then
                AGENT_ARGS+=("--provider" "$CROM_PROVIDER")
            fi
            if [[ -n "$CROM_MODEL" ]]; then
                AGENT_ARGS+=("--model" "$CROM_MODEL")
            fi
            
            # Executar com timeout
            START_TIME=$(date +%s)
            if timeout "$SCENARIO_TIMEOUT" "$CROM_BIN" "${AGENT_ARGS[@]}" > "$AGENT_LOG" 2>&1; then
                END_TIME=$(date +%s)
                DURATION=$((END_TIME - START_TIME))
                echo -e "  ${GREEN}✓ Agente concluiu em ${DURATION}s${NC}"
            else
                EXIT_CODE=$?
                END_TIME=$(date +%s)
                DURATION=$((END_TIME - START_TIME))
                if [[ $EXIT_CODE -eq 124 ]]; then
                    echo -e "  ${RED}✗ Agente atingiu timeout (${SCENARIO_TIMEOUT}s)${NC}"
                else
                    echo -e "  ${RED}✗ Agente falhou (exit code: $EXIT_CODE, ${DURATION}s)${NC}"
                fi
                echo -e "  ${RED}  Ver log: $AGENT_LOG${NC}"
                FAILED=$((FAILED + 1))
                echo ""
                continue
            fi
        fi
    else
        echo -e "  ${YELLOW}⏭ [2/3] Pulando execução do agente (USE_REAL_MODEL=false)${NC}"
    fi
    
    # ─── 3. Validação ───
    if [[ -f "$scenario_dir/validate.sh" ]]; then
        echo -e "  🔍 [3/3] Executando validação..."
        if bash "$scenario_dir/validate.sh" > "$RESULTS_DIR/${scenario_name}_validate.log" 2>&1; then
            echo -e "  ${GREEN}✓ Validação passou${NC}"
            PASSED=$((PASSED + 1))
        else
            echo -e "  ${RED}✗ Validação falhou (ver $RESULTS_DIR/${scenario_name}_validate.log)${NC}"
            FAILED=$((FAILED + 1))
        fi
    else
        echo -e "  ${YELLOW}⚠ Sem script de validação (somente setup + agente)${NC}"
        SKIPPED=$((SKIPPED + 1))
    fi
    
    echo "  ⏳ Aguardando 5 segundos para evitar Rate Limits..."
    sleep 5
    echo ""
done

echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
echo -e "  📊 Resultados:"
echo -e "     ${GREEN}Passou:  ${PASSED}${NC}"
echo -e "     ${RED}Falhou:  ${FAILED}${NC}"
echo -e "     ${YELLOW}Pulados: ${SKIPPED}${NC}"
echo -e "     Total:   ${TOTAL}"
echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"

if [[ $FAILED -gt 0 ]]; then
    exit 1
fi
