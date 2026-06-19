#!/bin/bash
# Setup: Cenário de Testes E2E Abrangentes (15 Testes)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# Limpa e recria diretório de workspace temporário e resultados anteriores
rm -rf temp_workspace test_results.json
mkdir -p temp_workspace

echo "✓ Cenário 11_comprehensive_tests preparado."
