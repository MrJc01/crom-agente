#!/bin/bash
# Setup: Projeto Go com bugs intencionais
# Capacidades testadas: 1-3, 7, 16, 19, 37, 39
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

[ -f go.mod ] || go mod init github.com/crom-tests/broken-code

mkdir -p internal/calculator internal/parser internal/formatter

echo "✓ Cenário 07_broken_code preparado."
echo "  ⚠ O projeto contém bugs intencionais para o agente corrigir."
