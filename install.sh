#!/usr/bin/env bash
# Tunnd client installer
# Usage: curl -sSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh | bash

set -euo pipefail

REPO="elvonpiko/tunnd"
BINARY="tunnd"
INSTALL_DIR="${TUNND_INSTALL_DIR:-/usr/local/bin}"

# ── Detect platform ───────────────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "error: unsupported OS: $OS (use the Windows release from GitHub)" >&2
    exit 1
    ;;
esac

# ── Fetch latest version ──────────────────────────────────────────────────────

echo "Fetching latest Tunnd release…"
LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
VERSION="$(curl -fsSL "$LATEST_URL" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"

if [[ -z "$VERSION" ]]; then
  echo "error: could not determine latest version" >&2
  exit 1
fi

echo "Installing Tunnd ${VERSION} (${OS}/${ARCH})…"

# ── Download & verify ─────────────────────────────────────────────────────────

TARBALL="tunnd_${VERSION#v}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "${BASE_URL}/${TARBALL}" -o "${TMP_DIR}/${TARBALL}"
curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP_DIR}/checksums.txt"

cd "$TMP_DIR"
if command -v sha256sum &>/dev/null; then
  grep "$TARBALL" checksums.txt | sha256sum --check --status
elif command -v shasum &>/dev/null; then
  grep "$TARBALL" checksums.txt | shasum -a 256 --check --status
fi
echo "✔ Checksum verified"

tar -xzf "$TARBALL"

# ── Install ───────────────────────────────────────────────────────────────────

if [[ -w "$INSTALL_DIR" ]]; then
  mv "$BINARY" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (sudo required)…"
  sudo mv "$BINARY" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "✔ Tunnd ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "  Get started:"
echo "    tunnd setup"
echo ""
