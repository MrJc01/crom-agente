#!/bin/bash
# Validação automática do cenário 11_comprehensive_tests
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# Carrega configuração compartilhada para isolar HOME e desativar daemon do usuário
source ../config.sh

echo "🔍 Validando cenário 11_comprehensive_tests..."

# Configurar OLLAMA_HOST no arquivo .env temporário de testes para apontar para o mock local (porta 9099)
mkdir -p "$HOME/.crom"
touch "$HOME/.crom/.env"
if grep -q "^OLLAMA_HOST=" "$HOME/.crom/.env"; then
    sed -i '/^OLLAMA_HOST=/d' "$HOME/.crom/.env"
fi
echo "OLLAMA_HOST=http://localhost:9099" >> "$HOME/.crom/.env"

# Garantir permissão de execução para o run_tests.py
chmod +x run_tests.py

# Se test_results.json já existe (por exemplo, porque o agente com modelo real executou),
# podemos validar diretamente. Se não existe ou se queremos garantir uma execução limpa,
# rodamos o script de testes de integração E2E mockado.
if [[ ! -f "test_results.json" ]]; then
    echo "  ⚙️ Executando run_tests.py..."
    python3 run_tests.py
else
    echo "  📄 test_results.json já encontrado. Validando resultados existentes..."
fi

# Verifica se o arquivo test_results.json foi gerado com sucesso
if [[ ! -f "test_results.json" ]]; then
    echo "  ❌ Falha: arquivo test_results.json não foi gerado."
    exit 1
fi

# Analisa o test_results.json para garantir que todos os 15 testes passaram
TOTAL_TESTS=$(jq '. | length' test_results.json 2>/dev/null || python3 -c "import json; print(len(json.load(open('test_results.json'))))")
PASSED_TESTS=$(jq '[.[] | select(.status == "PASS")] | length' test_results.json 2>/dev/null || python3 -c "import json; print(sum(1 for x in json.load(open('test_results.json')) if x.get('status') == 'PASS'))")
FAILED_TESTS=$(jq '[.[] | select(.status == "FAIL")] | length' test_results.json 2>/dev/null || python3 -c "import json; print(sum(1 for x in json.load(open('test_results.json')) if x.get('status') == 'FAIL'))")

echo "  📊 Resultados da Validação:"
echo "     Total de Testes: $TOTAL_TESTS"
echo "     Passaram:        $PASSED_TESTS"
echo "     Falharam:        $FAILED_TESTS"

if [[ "$TOTAL_TESTS" -ne 15 ]]; then
    echo "  ❌ Falha: O número de testes executados foi $TOTAL_TESTS, esperava-se 15."
    exit 1
fi

if [[ "$FAILED_TESTS" -gt 0 ]]; then
    echo "  ❌ Falha: $FAILED_TESTS teste(s) falharam."
    exit 1
fi

echo ""
echo "✅ Validação do cenário 11_comprehensive_tests passou com sucesso!"
exit 0
