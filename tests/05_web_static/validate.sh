#!/bin/bash
# Validação automática do cenário 05_web_static (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 05_web_static..."

# 1. Verifica se index.html existe
if [[ -f "index.html" ]]; then
    echo "  ✅ index.html existe"
    
    # Verifica se contém estrutura HTML básica
    if grep -qi "<html" index.html && grep -qi "</html>" index.html; then
        echo "  ✅ index.html contém tags HTML válidas"
    else
        echo "  ❌ index.html não contém estrutura HTML válida"
        ERRORS=$((ERRORS + 1))
    fi
    
    # Verifica se contém header/nav
    if grep -qi "<header\|<nav" index.html; then
        echo "  ✅ Header/navegação encontrados"
    else
        echo "  ⚠ Nenhum header/nav encontrado"
    fi
    
    # Verifica se referencia CSS
    if grep -qi "stylesheet\|\.css\|<style" index.html; then
        echo "  ✅ Referência a CSS encontrada"
    else
        echo "  ⚠ Nenhuma referência a CSS"
    fi
    
    # Verifica se referencia JS
    if grep -qi "\.js\|<script" index.html; then
        echo "  ✅ Referência a JavaScript encontrada"
    else
        echo "  ⚠ Nenhuma referência a JavaScript"
    fi
else
    echo "  ❌ index.html não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 2. Verifica CSS
CSS_FILES=$(find . -name "*.css" -not -path "./.crom/*" 2>/dev/null | wc -l)
if [[ "$CSS_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $CSS_FILES arquivo(s) CSS"
else
    echo "  ⚠ Nenhum arquivo CSS separado encontrado"
fi

# 3. Verifica JS
JS_FILES=$(find . -name "*.js" -not -path "./.crom/*" -not -path "*/node_modules/*" 2>/dev/null | wc -l)
if [[ "$JS_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $JS_FILES arquivo(s) JavaScript"
    # Verifica sintaxe se node disponível
    if command -v node >/dev/null 2>&1; then
        for js in $(find . -name "*.js" -not -path "./.crom/*" -not -path "*/node_modules/*" 2>/dev/null); do
            if ! node --check "$js" 2>/dev/null; then
                echo "  ❌ Erro de sintaxe em $js"
                ERRORS=$((ERRORS + 1))
            fi
        done
    fi
else
    echo "  ⚠ Nenhum arquivo JavaScript separado encontrado"
fi

# 4. Verifica dark mode (tarefa específica)
if grep -rli "dark-mode\|dark_mode\|theme-toggle\|darkMode\|prefers-color-scheme" --include="*.js" --include="*.css" --include="*.html" . >/dev/null 2>&1; then
    echo "  ✅ Referências a dark mode encontradas"
else
    echo "  ⚠ Nenhuma referência a dark mode"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 05_web_static passou!"
