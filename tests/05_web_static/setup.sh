#!/bin/bash
# Setup: Projeto Web Estático (HTML/CSS/JS)
# Capacidades testadas: 1-3, 6-7, 31-32, Browser Tool
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

mkdir -p css js images

echo "✓ Cenário 05_web_static preparado."
