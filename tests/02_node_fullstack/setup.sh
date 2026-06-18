#!/bin/bash
# Setup: Projeto Node.js — Fullstack (Express + Frontend)
# Capacidades testadas: 1-7, 10-12, 16-17, 31-32
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# Cria package.json
cat > package.json << 'PKGJSON'
{
  "name": "crom-test-node-fullstack",
  "version": "1.0.0",
  "description": "Projeto Node.js para testes do crom-agente",
  "main": "src/server.js",
  "scripts": {
    "start": "node src/server.js",
    "dev": "node --watch src/server.js",
    "test": "node --test src/**/*.test.js"
  },
  "keywords": [],
  "license": "MIT"
}
PKGJSON

# Cria estrutura
mkdir -p src/routes src/middleware src/models public

echo "✓ Cenário 02_node_fullstack preparado."
