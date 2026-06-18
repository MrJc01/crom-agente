#!/bin/bash
# Validação automática do cenário 01_go_api (pós-agente real)
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 01_go_api..."

# 1. Verifica se go.mod existe
if [[ ! -f "go.mod" ]]; then
    echo "  ❌ go.mod não encontrado"
    ERRORS=$((ERRORS + 1))
else
    echo "  ✅ go.mod existe"
fi

# 2. Verifica stack detection (go.mod deve conter module)
if grep -q "module" go.mod 2>/dev/null; then
    echo "  ✅ Stack Go detectável (go.mod contém module)"
else
    echo "  ❌ go.mod não contém declaração de módulo"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica se existe pelo menos um arquivo .go com handlers HTTP
GO_FILES=$(find . -name "*.go" -not -path "./.crom/*" | wc -l)
if [[ "$GO_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $GO_FILES arquivo(s) .go"
else
    echo "  ❌ Nenhum arquivo .go encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 4. Verifica se algum .go contém referências a net/http (API REST)
if grep -rl "net/http" --include="*.go" . >/dev/null 2>&1; then
    echo "  ✅ Código contém import net/http (API REST)"
else
    echo "  ❌ Nenhum arquivo .go importa net/http"
    ERRORS=$((ERRORS + 1))
fi

# 5. Verifica se os endpoints da tarefa foram criados
for endpoint in "/api/tasks" "GET" "POST"; do
    if grep -rl "$endpoint" --include="*.go" . >/dev/null 2>&1; then
        echo "  ✅ Referência a '$endpoint' encontrada no código"
    else
        echo "  ⚠ Referência a '$endpoint' não encontrada (pode estar implementado de outra forma)"
    fi
done

# 6. Verifica se o projeto compila
if go build ./... 2>/dev/null; then
    echo "  ✅ Projeto compila sem erros"
else
    echo "  ❌ Projeto não compila"
    ERRORS=$((ERRORS + 1))
fi

# 7. Verifica go vet
if go vet ./... 2>/dev/null; then
    echo "  ✅ go vet passou sem avisos"
else
    echo "  ⚠ go vet reportou avisos"
fi

# 8. Verifica se existem testes
TEST_FILES=$(find . -name "*_test.go" -not -path "./.crom/*" | wc -l)
if [[ "$TEST_FILES" -gt 0 ]]; then
    echo "  ✅ Encontrados $TEST_FILES arquivo(s) de teste"
else
    echo "  ⚠ Nenhum arquivo de teste encontrado"
fi

# 9. .cromrules (opcional)
if [[ -f ".cromrules" ]]; then
    echo "  ✅ .cromrules existe (regras locais carregáveis)"
else
    echo "  ⚠ .cromrules não encontrado (opcional)"
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 01_go_api passou!"
