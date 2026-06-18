#!/bin/bash
# Validação automática do cenário 07_broken_code (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 07_broken_code..."

# 1. Verifica se go.mod existe
if [[ ! -f "go.mod" ]]; then
    echo "  ❌ go.mod não encontrado"
    ERRORS=$((ERRORS + 1))
else
    echo "  ✅ go.mod existe"
fi

# 2. Verifica se os arquivos existem
for f in "internal/calculator/math.go" "internal/calculator/concurrent.go" "internal/parser/csv.go" "internal/parser/csv_test.go" "internal/formatter/format.go"; do
    if [[ -f "$f" ]]; then
        echo "  ✅ $f existe"
    else
        echo "  ❌ $f não encontrado"
        ERRORS=$((ERRORS + 1))
    fi
done

# 3. Verifica que o projeto COMPILA (o agente deveria ter corrigido os erros)
echo "  🔍 Verificando compilação (bugs devem estar corrigidos)..."
if go build ./... 2>/dev/null; then
    echo "  ✅ Projeto compila sem erros (bugs de compilação corrigidos)"
else
    echo "  ❌ Projeto ainda não compila (bugs não foram corrigidos)"
    ERRORS=$((ERRORS + 1))
fi

# 4. Verifica go vet
if go vet ./... 2>/dev/null; then
    echo "  ✅ go vet passou sem avisos"
else
    echo "  ⚠ go vet reportou avisos"
fi

# 5. Verifica que testes do parser passam (bugs de lógica corrigidos)
echo "  🔍 Verificando testes do parser..."
if go test ./internal/parser/ 2>/dev/null; then
    echo "  ✅ Testes do parser passam (bugs de lógica corrigidos)"
else
    echo "  ❌ Testes do parser ainda falham"
    ERRORS=$((ERRORS + 1))
fi

# 6. Verifica complexidade refatorada do formatter
if [[ -f "internal/formatter/format.go" ]]; then
    # Conta o número de funções (indicador de refatoração)
    FUNC_COUNT=$(grep -c "^func " internal/formatter/format.go 2>/dev/null || echo "0")
    if [[ $FUNC_COUNT -gt 3 ]]; then
        echo "  ✅ format.go contém $FUNC_COUNT funções (provavelmente refatorado)"
    else
        echo "  ⚠ format.go contém apenas $FUNC_COUNT funções (pode não ter sido refatorado)"
    fi
fi

# 7. Verifica race condition fix
echo "  🔍 Verificando race conditions..."
if go test -race ./internal/calculator/ -timeout 15s 2>/dev/null; then
    echo "  ✅ Testes passam com -race (race condition corrigida)"
else
    echo "  ⚠ Testes com -race falharam ou timeout"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 07_broken_code passou!"
