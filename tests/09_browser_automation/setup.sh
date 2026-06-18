#!/bin/bash
# Setup: Cenário de Automação de Browser
# Capacidades testadas: Browser Tool, Computer Control, 31-32
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

mkdir -p pages server

echo "✓ Cenário 09_browser_automation preparado."
