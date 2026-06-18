#!/bin/bash
# ══════════════════════════════════════════════════════════════
# build_agent.sh — Compila o binário crom-agente para testes E2E
# ══════════════════════════════════════════════════════════════
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/.bin"
OUTPUT_BIN="$OUTPUT_DIR/crom-agente"

CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  🔨 Compilando crom-agente para testes E2E${NC}"
echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"

# Criar diretório de output
mkdir -p "$OUTPUT_DIR"

# Compilar o binário
cd "$PROJECT_ROOT"

echo -e "  📦 Diretório: $PROJECT_ROOT"
echo -e "  📂 Output: $OUTPUT_BIN"
echo ""

if go build -tags headless -o "$OUTPUT_BIN" ./cmd/crom-agente/ 2>&1; then
    echo ""
    echo -e "  ${GREEN}✓ Compilação concluída com sucesso${NC}"
    
    # Verificar tamanho do binário
    SIZE=$(du -h "$OUTPUT_BIN" | cut -f1)
    echo -e "  📏 Tamanho do binário: $SIZE"
    
    # Verificar versão
    VERSION=$("$OUTPUT_BIN" version 2>&1 || echo "desconhecida")
    echo -e "  📌 Versão: $VERSION"
else
    echo -e "  ${RED}✗ Falha na compilação${NC}"
    exit 1
fi

echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
