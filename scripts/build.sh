#!/bin/bash
set -e

# Define working dir
CWD="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CWD"

# Opcionalmente gera os arquivos gRPC se o protoc estiver disponivel
if command -v protoc &> /dev/null; then
    echo "⚙️ Gerando arquivos gRPC de daemon.proto..."
    protoc --go_out=. --go_opt=module=github.com/crom/crom-agente \
           --go-grpc_out=. --go-grpc_opt=module=github.com/crom/crom-agente \
           daemon.proto
else
    echo "⚠️ protoc nao encontrado. Ignorando geracao do gRPC (usando arquivos existentes)..."
fi

echo "🔨 Preparando compilacao cross-platform..."
mkdir -p bin

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
)

# Se o argumento --current-only for fornecido, compila apenas para a plataforma atual
if [ "$1" == "--current-only" ]; then
    CURRENT_OS=$(go env GOOS)
    CURRENT_ARCH=$(go env GOARCH)
    PLATFORMS=("${CURRENT_OS}/${CURRENT_ARCH}")
fi

MAX_SIZE_MB=30
MAX_SIZE_BYTES=$((MAX_SIZE_MB * 1024 * 1024))

for PLATFORM in "${PLATFORMS[@]}"; do
    IFS="/" read -r -a parts <<< "$PLATFORM"
    GOOS="${parts[0]}"
    GOARCH="${parts[1]}"
    
    OUTPUT_NAME="bin/crom-agente-${GOOS}-${GOARCH}"
    if [ "$GOOS" == "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    echo "🚀 Compilando para ${GOOS}/${GOARCH}..."
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags="-s -w" \
        -tags headless \
        -o "${OUTPUT_NAME}" \
        cmd/crom-agente/main.go
        
    # Validacao de tamanho
    FILE_SIZE=$(wc -c < "${OUTPUT_NAME}")
    FILE_SIZE_MB=$(echo "scale=2; ${FILE_SIZE}/1048576" | bc 2>/dev/null || expr "${FILE_SIZE}" / 1048576)
    
    echo "✓ Concluido: ${OUTPUT_NAME} (${FILE_SIZE_MB}MB)"
    
    if [ "${FILE_SIZE}" -gt "${MAX_SIZE_BYTES}" ]; then
        echo "❌ ERRO: O binario ${OUTPUT_NAME} excedeu o limite de ${MAX_SIZE_MB}MB! Tamanho: ${FILE_SIZE_MB}MB"
        exit 1
    fi
done

echo "🎉 Todas as compilacoes concluidas com sucesso e validadas abaixo de ${MAX_SIZE_MB}MB!"
