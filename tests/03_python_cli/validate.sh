#!/bin/bash
# Validação automática do cenário 03_python_cli (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 03_python_cli..."

# 1. Verifica requirements.txt
if [[ -f "requirements.txt" ]]; then
    echo "  ✅ requirements.txt existe"
else
    echo "  ⚠ requirements.txt não encontrado"
fi

# 2. Verifica se existe cli.py ou main.py
if [[ -f "src/cli.py" ]] || [[ -f "cli.py" ]] || [[ -f "src/main.py" ]] || [[ -f "main.py" ]]; then
    echo "  ✅ Arquivo principal Python encontrado"
else
    echo "  ❌ Nenhum arquivo Python principal (cli.py/main.py) encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica sintaxe de todos os .py
PY_FILES=$(find . -name "*.py" -not -path "./.crom/*" -not -path "*/venv/*" -not -path "*/__pycache__/*" 2>/dev/null)
PY_ERRORS=0
for py in $PY_FILES; do
    if ! python3 -m py_compile "$py" 2>/dev/null; then
        echo "  ❌ Erro de sintaxe em $py"
        PY_ERRORS=$((PY_ERRORS + 1))
    fi
done
if [[ $PY_ERRORS -eq 0 ]] && [[ -n "$PY_FILES" ]]; then
    PY_COUNT=$(echo "$PY_FILES" | wc -l)
    echo "  ✅ $PY_COUNT arquivo(s) Python passaram na verificação de sintaxe"
elif [[ -z "$PY_FILES" ]]; then
    echo "  ❌ Nenhum arquivo .py encontrado"
    ERRORS=$((ERRORS + 1))
fi
ERRORS=$((ERRORS + PY_ERRORS))

# 4. Verifica se contém click (CLI framework)
if grep -rl "click\|argparse\|typer" --include="*.py" . >/dev/null 2>&1; then
    echo "  ✅ Framework CLI detectado (click/argparse/typer)"
else
    echo "  ⚠ Nenhum framework CLI detectado"
fi

# 5. Verifica se existem testes
TEST_FILES=$(find . -name "test_*.py" -o -name "*_test.py" 2>/dev/null | grep -v __pycache__ | wc -l)
if [[ "$TEST_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $TEST_FILES arquivo(s) de teste Python"
else
    echo "  ⚠ Nenhum arquivo de teste Python encontrado"
fi

# 6. Verifica models
if [[ -f "src/models.py" ]] || [[ -f "models.py" ]]; then
    echo "  ✅ Arquivo de modelos encontrado"
else
    echo "  ⚠ models.py não encontrado"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 03_python_cli passou!"
