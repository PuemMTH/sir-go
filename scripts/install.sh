#!/usr/bin/env bash
# curl -fsSL https://raw.githubusercontent.com/PuemMTH/sir-go/main/install.sh | bash
set -euo pipefail

REPO="PuemMTH/sir-go"
BIN_NAME="sir"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# ── Detect OS ─────────────────────────────────────────────────────────────────

case "$(uname -s)" in
  Linux*)  OS=linux  ;;
  Darwin*) OS=darwin ;;
  *)
    echo "error: unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

# ── Detect arch ───────────────────────────────────────────────────────────────

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *)
    echo "error: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

ASSET="${BIN_NAME}_${OS}_${ARCH}"

# ── Resolve latest tag ────────────────────────────────────────────────────────

echo "  Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | head -1 \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$TAG" ]; then
  echo "error: could not determine latest release tag" >&2
  exit 1
fi

echo "  Latest version: ${TAG}"

# ── Download ──────────────────────────────────────────────────────────────────

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "  Downloading ${ASSET}..."
curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${BIN_NAME}"

# ── Verify checksum ───────────────────────────────────────────────────────────

echo "  Verifying checksum..."
curl -fsSL "$CHECKSUM_URL" -o "${TMP_DIR}/checksums.txt"

EXPECTED=$(grep "$ASSET" "${TMP_DIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "error: no checksum entry found for ${ASSET}" >&2
  exit 1
fi

if [ "$OS" = "darwin" ]; then
  ACTUAL=$(shasum -a 256 "${TMP_DIR}/${BIN_NAME}" | awk '{print $1}')
else
  ACTUAL=$(sha256sum "${TMP_DIR}/${BIN_NAME}" | awk '{print $1}')
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "error: checksum verification failed" >&2
  exit 1
fi

# ── Install ───────────────────────────────────────────────────────────────────

chmod +x "${TMP_DIR}/${BIN_NAME}"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
else
  echo "  Installing to ${INSTALL_DIR} (sudo required)..."
  sudo mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
fi

echo ""
echo "  ✓ Installed ${BIN_NAME} ${TAG} → ${INSTALL_DIR}/${BIN_NAME}"
echo ""
echo "  Run: sir --help"
