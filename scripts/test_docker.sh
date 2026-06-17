#!/bin/bash
set -e

CWD="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CWD"

BINARY="bin/crom-agente-linux-amd64"
if [ ! -f "$BINARY" ]; then
    echo "🔨 Binario nao encontrado. Compilando..."
    bash scripts/build.sh --current-only
fi

echo "🐳 Testando compatibilidade do binario usando Docker..."

# Ubuntu
echo "👉 Testando no Ubuntu..."
docker run --rm -v "$CWD/bin:/app" ubuntu:latest /app/crom-agente-linux-amd64 version

# Alpine
echo "👉 Testando no Alpine (musl)..."
docker run --rm -v "$CWD/bin:/app" alpine:latest /app/crom-agente-linux-amd64 version

# Fedora
echo "👉 Testando no Fedora..."
docker run --rm -v "$CWD/bin:/app" fedora:latest /app/crom-agente-linux-amd64 version

echo "🎉 Compatibilidade do binario validada com sucesso em Ubuntu, Alpine e Fedora!"
