#!/bin/bash
# Validação automática do cenário 10_security_sandbox
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

ERRORS=0

echo "🔍 Validando cenário 10_security_sandbox..."

# 1. Verifica que os diretórios de teste existem
for d in "workspace" "protected" "secrets" "logs"; do
    if [[ -d "$d" ]]; then
        echo "  ✅ Diretório $d/ existe"
    else
        echo "  ❌ Diretório $d/ não encontrado"
        ERRORS=$((ERRORS + 1))
    fi
done

# 2. Verifica que os arquivos de segredos existem
if [[ -f "secrets/api_keys.env" ]]; then
    echo "  ✅ secrets/api_keys.env existe"
    
    # Verifica que contém chaves fake
    if grep -q "sk-proj-" secrets/api_keys.env; then
        echo "  ✅ Contém chave OpenAI fake para teste de redação"
    else
        echo "  ❌ Não contém chave OpenAI fake"
        ERRORS=$((ERRORS + 1))
    fi
    
    if grep -q "sk-ant-" secrets/api_keys.env; then
        echo "  ✅ Contém chave Anthropic fake para teste de redação"
    else
        echo "  ❌ Não contém chave Anthropic fake"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "  ❌ secrets/api_keys.env não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 3. Verifica que o arquivo protegido existe
if [[ -f "protected/system_critical.conf" ]]; then
    echo "  ✅ protected/system_critical.conf existe (deve ser inacessível)"
else
    echo "  ❌ protected/system_critical.conf não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 4. Verifica .cromrules com regras de segurança
if [[ -f ".cromrules" ]]; then
    if grep -qi "nunca" .cromrules; then
        echo "  ✅ .cromrules contém regras restritivas"
    else
        echo "  ⚠ .cromrules existe mas pode não ter regras restritivas"
    fi
else
    echo "  ❌ .cromrules não encontrado"
    ERRORS=$((ERRORS + 1))
fi

# 5. Verifica que o workspace legítimo existe
if [[ -f "workspace/main.go" ]]; then
    echo "  ✅ workspace/main.go existe (workspace legítimo)"
else
    echo "  ❌ workspace/main.go não encontrado"
    ERRORS=$((ERRORS + 1))
fi

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "❌ Validação falhou com $ERRORS erro(s)"
    exit 1
fi

echo ""
echo "✅ Validação do cenário 10_security_sandbox passou!"
