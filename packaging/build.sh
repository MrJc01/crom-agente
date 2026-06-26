#!/usr/bin/env bash
# Script de build para gerar pacotes .deb e .tar.gz para distribuição
set -euo pipefail

VERSION="${1:-dev}"
BINARY="crom-agente"
BUILD_DIR="dist"
PKG_DIR="${BUILD_DIR}/${BINARY}_${VERSION}"

echo "🔨 Building ${BINARY} v${VERSION}..."

# Build do binário Go
CGO_ENABLED=0 go build \
  -ldflags="-s -w -X github.com/crom/crom-agente/internal/cli.Version=${VERSION}" \
  -o "${BUILD_DIR}/${BINARY}" \
  ./cmd/crom-agente

echo "✅ Binário compilado: ${BUILD_DIR}/${BINARY}"

# Criar tarball
mkdir -p "${PKG_DIR}"
cp "${BUILD_DIR}/${BINARY}" "${PKG_DIR}/"
cp README.md LICENSE "${PKG_DIR}/" 2>/dev/null || true
tar czf "${BUILD_DIR}/${BINARY}-${VERSION}-linux-amd64.tar.gz" -C "${BUILD_DIR}" "${BINARY}_${VERSION}"
echo "📦 Tarball: ${BUILD_DIR}/${BINARY}-${VERSION}-linux-amd64.tar.gz"

# Gerar pacote .deb (se dpkg-deb estiver disponível)
if command -v dpkg-deb &>/dev/null; then
  DEB_DIR="${BUILD_DIR}/deb"
  mkdir -p "${DEB_DIR}/DEBIAN"
  mkdir -p "${DEB_DIR}/usr/local/bin"
  
  cp "${BUILD_DIR}/${BINARY}" "${DEB_DIR}/usr/local/bin/"
  
  cat > "${DEB_DIR}/DEBIAN/control" <<EOF
Package: crom-agente
Version: ${VERSION}
Section: devel
Priority: optional
Architecture: amd64
Maintainer: Crom Team <dev@crom.run>
Description: Agente autônomo de engenharia de software baseado em LLMs
EOF
  
  dpkg-deb --build "${DEB_DIR}" "${BUILD_DIR}/${BINARY}_${VERSION}_amd64.deb"
  echo "📦 Deb: ${BUILD_DIR}/${BINARY}_${VERSION}_amd64.deb"
fi

echo "🚀 Build completo!"
