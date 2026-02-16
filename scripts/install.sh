#!/usr/bin/env bash
set -euo pipefail

REPO="koganei/fora"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${FORA_VERSION:-latest}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd tar
need_cmd uname

os_raw="$(uname -s)"
arch_raw="$(uname -m)"

case "$os_raw" in
  Linux) OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)
    echo "error: unsupported OS: $os_raw" >&2
    exit 1
    ;;
esac

case "$arch_raw" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "error: unsupported architecture: $arch_raw" >&2
    exit 1
    ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "$VERSION" ]]; then
    echo "error: failed to determine latest release version" >&2
    exit 1
  fi
fi

arcfora="fora_${VERSION}_${OS}_${ARCH}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${arcfora}"

workdir="$(mktemp -d)"
cleanup() { rm -rf "$workdir"; }
trap cleanup EXIT

cd "$workdir"
echo "Downloading ${url}"
curl -fL -o "$arcfora" "$url"

tar -xzf "$arcfora"
extracted_dir="fora_${VERSION}_${OS}_${ARCH}"
if [[ ! -d "$extracted_dir" ]]; then
  echo "error: extracted arcfora directory missing: $extracted_dir" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR" 2>/dev/null || true

install_one() {
  local name="$1"
  local src="${extracted_dir}/${name}"
  if [[ ! -f "$src" ]]; then
    echo "warning: ${name} not found in arcfora, skipping" >&2
    return
  fi

  if [[ -w "$INSTALL_DIR" ]]; then
    install -m 0755 "$src" "${INSTALL_DIR}/${name}"
  else
    sudo install -m 0755 "$src" "${INSTALL_DIR}/${name}"
  fi
}

install_one fora
install_one fora-server
install_one fora-mcp

echo "Installed Fora binaries to ${INSTALL_DIR}:"
echo "- fora"
echo "- fora-server"
echo "- fora-mcp"

echo "Done."
