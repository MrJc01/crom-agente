#!/usr/bin/env bash
# Script para gravar um tutorial asciinema demonstrando o benchmark
# Requer: asciinema instalado (pip install asciinema)
#
# Uso:
#   ./scripts/record_tutorial.sh
#
# O arquivo gravado será salvo em docs/tutorial.cast
set -euo pipefail

OUTPUT="docs/tutorial.cast"

if ! command -v asciinema &>/dev/null; then
  echo "❌ asciinema não está instalado."
  echo "   Instale com: pip install asciinema"
  exit 1
fi

mkdir -p docs

echo "🎬 Iniciando gravação do tutorial..."
echo "   Execute os comandos manualmente durante a gravação."
echo "   Sugestão de roteiro:"
echo ""
echo "   1. crom-agente --version"
echo "   2. crom-agente config init"
echo "   3. cd /tmp && mkdir demo-project && cd demo-project && git init"
echo "   4. crom-agente run 'Crie um servidor HTTP simples em Go'"
echo "   5. cat main.go"
echo "   6. python3 benchmark/run_all.py --limit 1"
echo ""
echo "   Pressione Ctrl+D ou 'exit' para finalizar."
echo ""

asciinema rec \
  --title "crom-agente: Tutorial de Benchmark" \
  --cols 120 \
  --rows 35 \
  --idle-time-limit 3 \
  "${OUTPUT}"

echo ""
echo "✅ Gravação salva em: ${OUTPUT}"
echo "   Para reproduzir: asciinema play ${OUTPUT}"
echo "   Para publicar:   asciinema upload ${OUTPUT}"
