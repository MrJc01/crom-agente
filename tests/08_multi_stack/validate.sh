#!/bin/bash
# Validação automática do cenário 08_multi_stack (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 08_multi_stack..."

# 1. Verifica Go Backend
if [[ -d "backend" ]]; then
    echo "  ✅ Diretório backend/ existe"
    
    if [[ -f "backend/go.mod" ]]; then
        echo "  ✅ backend/go.mod existe"
    else
        echo "  ⚠ backend/go.mod não encontrado"
    fi
    
    # Verifica se existe código Go
    GO_FILES=$(find backend -name "*.go" 2>/dev/null | wc -l)
    if [[ "$GO_FILES" -gt 0 ]]; then
        echo "  ✅ $GO_FILES arquivo(s) Go no backend"
    else
        echo "  ❌ Nenhum arquivo Go no backend"
        ERRORS=$((ERRORS + 1))
    fi
    
    # Tenta compilar backend
    if [[ -f "backend/go.mod" ]]; then
        cd backend
        if go build ./... 2>/dev/null; then
            echo "  ✅ Backend Go compila"
        else
            echo "  ❌ Backend Go não compila"
            ERRORS=$((ERRORS + 1))
        fi
        cd "$DIR"
    fi
else
    echo "  ❌ Diretório backend/ não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 2. Verifica Node Frontend
if [[ -d "frontend" ]]; then
    echo "  ✅ Diretório frontend/ existe"
    
    if [[ -f "frontend/package.json" ]]; then
        echo "  ✅ frontend/package.json existe"
    else
        echo "  ⚠ frontend/package.json não encontrado"
    fi
    
    # Verifica HTML
    HTML_FILES=$(find frontend -name "*.html" 2>/dev/null | wc -l)
    if [[ "$HTML_FILES" -gt 0 ]]; then
        echo "  ✅ $HTML_FILES arquivo(s) HTML no frontend"
    else
        echo "  ⚠ Nenhum arquivo HTML no frontend"
    fi
    
    # Verifica JS
    JS_FILES=$(find frontend -name "*.js" -not -path "*/node_modules/*" 2>/dev/null | wc -l)
    if [[ "$JS_FILES" -gt 0 ]]; then
        echo "  ✅ $JS_FILES arquivo(s) JS no frontend"
    fi
else
    echo "  ❌ Diretório frontend/ não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica referências HTTP (API REST)
if grep -rl "net/http\|ListenAndServe\|http.Handle" --include="*.go" backend/ >/dev/null 2>&1; then
    echo "  ✅ Backend contém servidor HTTP"
else
    echo "  ⚠ Nenhuma referência a servidor HTTP no backend"
fi

# 4. Verifica Makefile
if [[ -f "Makefile" ]]; then
    echo "  ✅ Makefile existe"
else
    echo "  ⚠ Makefile não encontrado"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 08_multi_stack passou!"
