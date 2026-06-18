#!/bin/bash
# Setup: Projeto Python — CLI Tool
# Capacidades testadas: 1-7, 10-12, 16-17
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# Cria requirements.txt
cat > requirements.txt << 'REQ'
click>=8.0
rich>=13.0
pytest>=7.0
REQ

# Cria pyproject.toml
cat > pyproject.toml << 'TOML'
[project]
name = "crom-test-python-cli"
version = "0.1.0"
description = "CLI Tool para testes do crom-agente"
requires-python = ">=3.9"

[tool.pytest.ini_options]
testpaths = ["tests"]
TOML

mkdir -p src/commands tests

echo "✓ Cenário 03_python_cli preparado."
