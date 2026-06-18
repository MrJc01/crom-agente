#!/bin/bash
# Setup: Projeto Go — API REST simples
# Capacidades testadas: 1-7, 9-12, 15-20, 26-30, 36-40
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

# Inicializa módulo Go
cd "$DIR"
[ -f go.mod ] || go mod init github.com/crom-tests/go-api

# Cria estrutura de diretórios
mkdir -p cmd/server internal/handlers internal/models internal/middleware

echo "✓ Cenário 01_go_api preparado."
