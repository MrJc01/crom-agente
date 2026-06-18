#!/bin/bash
# Validação automática do cenário 04_rust_calculator (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 04_rust_calculator..."

# 1. Verifica Cargo.toml
if [[ -f "Cargo.toml" ]]; then
    echo "  ✅ Cargo.toml existe"
    if grep -q "\[package\]" Cargo.toml; then
        echo "  ✅ Cargo.toml contém [package]"
    else
        echo "  ❌ Cargo.toml não contém [package]"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "  ❌ Cargo.toml não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 2. Verifica se existem arquivos .rs
RS_FILES=$(find . -name "*.rs" -not -path "./.crom/*" 2>/dev/null | wc -l)
if [[ "$RS_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $RS_FILES arquivo(s) .rs"
else
    echo "  ❌ Nenhum arquivo .rs encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica se src/lib.rs ou src/main.rs existem
if [[ -f "src/lib.rs" ]] || [[ -f "src/main.rs" ]]; then
    echo "  ✅ Arquivo principal Rust encontrado"
else
    echo "  ❌ Nenhum src/lib.rs ou src/main.rs"
    ERRORS=$((ERRORS + 1))
fi

# 4. Verifica se contém Calculator struct
if grep -rl "Calculator\|calculator" --include="*.rs" . >/dev/null 2>&1; then
    echo "  ✅ Referência a Calculator encontrada"
else
    echo "  ⚠ Nenhuma referência a Calculator nos arquivos .rs"
fi

# 5. Verifica se contém operações aritméticas
for op in "add\|sum" "sub\|subtract" "mul\|multiply" "div\|divide"; do
    if grep -rl "$op" --include="*.rs" . >/dev/null 2>&1; then
        echo "  ✅ Operação $op encontrada"
    else
        echo "  ⚠ Operação $op não encontrada"
    fi
done

# 6. Tenta compilar se cargo disponível
if command -v cargo >/dev/null 2>&1; then
    if cargo check 2>/dev/null; then
        echo "  ✅ cargo check passou sem erros"
    else
        echo "  ❌ cargo check falhou"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "  ⚠ cargo não disponível — pulando verificação de compilação"
fi

# 7. Verifica se existem testes (#[cfg(test)])
if grep -rl "#\[cfg(test)\]\|#\[test\]" --include="*.rs" . >/dev/null 2>&1; then
    echo "  ✅ Testes unitários encontrados"
else
    echo "  ⚠ Nenhum teste unitário encontrado"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 04_rust_calculator passou!"
