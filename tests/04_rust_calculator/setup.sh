#!/bin/bash
# Setup: Projeto Rust — Calculator Library
# Capacidades testadas: 1-7, 10-12, 16-17, 35-37
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

# Cria Cargo.toml manualmente (sem precisar do cargo)
cat > Cargo.toml << 'CARGO'
[package]
name = "crom-test-calculator"
version = "0.1.0"
edition = "2021"
description = "Calculadora em Rust para testes do crom-agente"

[[bin]]
name = "calc"
path = "src/main.rs"
CARGO

mkdir -p src tests

echo "✓ Cenário 04_rust_calculator preparado."
