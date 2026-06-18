#!/bin/bash
# Validação automática do cenário 02_node_fullstack (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 02_node_fullstack..."

# 1. Verifica package.json
if [[ -f "package.json" ]]; then
    echo "  ✅ package.json existe"
    if node -e "JSON.parse(require('fs').readFileSync('package.json'))" 2>/dev/null; then
        echo "  ✅ package.json é JSON válido"
    else
        echo "  ❌ package.json contém JSON inválido"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "  ❌ package.json não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 2. Verifica se existe server.js ou app.js
if [[ -f "src/server.js" ]] || [[ -f "server.js" ]] || [[ -f "src/app.js" ]] || [[ -f "app.js" ]] || [[ -f "src/index.js" ]]; then
    echo "  ✅ Arquivo de servidor encontrado"
else
    echo "  ❌ Nenhum arquivo de servidor (server.js/app.js/index.js) encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica se o código contém referência a express
if grep -rl "express\|http" --include="*.js" . >/dev/null 2>&1; then
    echo "  ✅ Código contém referências a express/http"
else
    echo "  ⚠ Nenhuma referência a express/http nos arquivos JS"
fi

# 4. Verifica se existe frontend (index.html)
if [[ -f "public/index.html" ]] || [[ -f "index.html" ]]; then
    echo "  ✅ Arquivo HTML de frontend encontrado"
else
    echo "  ⚠ Nenhum index.html encontrado"
fi

# 5. Verifica sintaxe dos .js com Node
JS_FILES=$(find . -name "*.js" -not -path "*/node_modules/*" -not -path "./.crom/*" 2>/dev/null)
JS_ERRORS=0
for js in $JS_FILES; do
    if ! node --check "$js" 2>/dev/null; then
        echo "  ❌ Erro de sintaxe em $js"
        JS_ERRORS=$((JS_ERRORS + 1))
    fi
done
if [[ $JS_ERRORS -eq 0 ]] && [[ -n "$JS_FILES" ]]; then
    echo "  ✅ Todos os arquivos JS passaram na verificação de sintaxe"
elif [[ -z "$JS_FILES" ]]; then
    echo "  ⚠ Nenhum arquivo .js para verificar"
fi
ERRORS=$((ERRORS + JS_ERRORS))

# 6. Verifica se existem rotas /api/notes
if grep -rl "notes\|/api" --include="*.js" . >/dev/null 2>&1; then
    echo "  ✅ Referências a rotas de API encontradas"
else
    echo "  ⚠ Nenhuma rota /api/notes encontrada"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 02_node_fullstack passou!"
