#!/bin/sh
set -eu

OWNER="whale9820"
REPO="au-cli"
BIN="au"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${AU_VERSION:-latest}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux) os="linux" ;;
  darwin) os="darwin" ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

asset="${BIN}-${os}-${arch}"
if [ "$os" = "windows" ]; then
  asset="${asset}.exe"
fi

if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${OWNER}/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${asset}"
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT INT TERM

echo "downloading ${asset} from ${VERSION}..."
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"

if [ -w "$INSTALL_DIR" ] || { [ ! -e "$INSTALL_DIR" ] && [ -w "$(dirname "$INSTALL_DIR")" ]; }; then
  mkdir -p "$INSTALL_DIR"
  mv "$tmp" "${INSTALL_DIR}/${BIN}"
else
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "$tmp" "${INSTALL_DIR}/${BIN}"
fi

trap - EXIT INT TERM

echo "installed ${BIN} to ${INSTALL_DIR}/${BIN}"
