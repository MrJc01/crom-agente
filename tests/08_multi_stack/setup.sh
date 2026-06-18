#!/bin/bash
# Setup: Projeto Multi-Stack (Go backend + Node.js frontend)
# Capacidades testadas: 6-7, 10, 15, 35
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# --- Backend Go ---
mkdir -p backend
cd backend
[ -f go.mod ] || go mod init github.com/crom-tests/multi-stack-backend
mkdir -p cmd/api internal/handlers
cd "$DIR"

# --- Frontend Node ---
mkdir -p frontend/src frontend/public

cat > frontend/package.json << 'PKGJSON'
{
  "name": "crom-test-multi-frontend",
  "version": "1.0.0",
  "description": "Frontend para o teste multi-stack",
  "scripts": {
    "dev": "npx serve public -p 5173",
    "test": "echo 'no tests yet'"
  }
}
PKGJSON

echo "✓ Cenário 08_multi_stack preparado."
