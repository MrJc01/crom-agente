#!/bin/bash
# Validação automática do cenário 09_browser_automation (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 09_browser_automation..."

# 1. Verifica se as páginas HTML existem
for f in "pages/form.html" "pages/game.html" "pages/table.html" "pages/dashboard.html"; do
    if [[ -f "$f" ]]; then
        echo "  ✅ $f existe"
    else
        echo "  ❌ $f não encontrado"
        ERRORS=$((ERRORS + 1))
    fi
done

# 2. Verifica se o servidor pode ser iniciado
if [[ -f "server/serve.sh" ]]; then
    echo "  ✅ server/serve.sh existe"
else
    echo "  ❌ server/serve.sh não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica se o Python está disponível
if command -v python3 &>/dev/null; then
    echo "  ✅ python3 disponível"
else
    echo "  ⚠ python3 não encontrado (necessário para servir as páginas)"
fi

# 4. Valida que os HTMLs são bem formados
for f in pages/*.html; do
    if grep -q "</html>" "$f" 2>/dev/null; then
        echo "  ✅ $(basename $f) é HTML válido"
    else
        echo "  ❌ $(basename $f) não tem tag </html> de fechamento"
        ERRORS=$((ERRORS + 1))
    fi
done

# 5. Verifica que form.html tem campos de input
if grep -q 'id="name"' pages/form.html && grep -q 'id="email"' pages/form.html; then
    echo "  ✅ form.html tem campos name e email (testáveis)"
else
    echo "  ❌ form.html não tem os campos esperados"
    ERRORS=$((ERRORS + 1))
fi

# 6. Verifica que game.html tem score display
if grep -q 'id="scoreDisplay"' pages/game.html; then
    echo "  ✅ game.html tem score display (testável)"
else
    echo "  ❌ game.html não tem score display"
    ERRORS=$((ERRORS + 1))
fi

# 7. Verifica se o agente gerou screenshots (evidência de browser_action)
SCREENSHOTS=$(find . -name "screenshot*.png" -o -name "*.png" 2>/dev/null | grep -v ".crom" | wc -l)
if [[ "$SCREENSHOTS" -gt 0 ]]; then
    echo "  ✅ $SCREENSHOTS screenshot(s) gerado(s) pelo agente"
else
    echo "  ⚠ Nenhum screenshot encontrado (browser_action pode não ter rodado)"
fi

# 8. Verifica se foram gerados dados extraídos
EXTRACTED=$(find . -name "extracted*" -o -name "output*" -o -name "results*" 2>/dev/null | grep -v ".crom" | wc -l)
if [[ "$EXTRACTED" -gt 0 ]]; then
    echo "  ✅ Dados extraídos encontrados"
else
    echo "  ⚠ Nenhum arquivo de dados extraídos encontrado"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 09_browser_automation passou!"
