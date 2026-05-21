#!/bin/sh
set -eu

REPO="${AIMAN_REPO:-axsmak/aim}"
VERSION="${AIMAN_VERSION:-latest}"
INSTALL_DIR="${AIMAN_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="aiman"

usage() {
  cat <<EOF
Install AIM Loadout CLI for Linux.

Usage:
  sh install.sh
  AIMAN_VERSION=vX.Y.Z sh install.sh
  AIMAN_INSTALL_DIR=/usr/local/bin sh install.sh

Environment:
  AIMAN_REPO         GitHub repository, default: axsmak/aim
  AIMAN_VERSION      Release tag, default: latest
  AIMAN_INSTALL_DIR  Install directory, default: \$HOME/.local/bin
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

download() {
  url="$1"
  out="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    die "missing required command: curl or wget"
  fi
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    die "unknown argument: $1"
    ;;
esac

need uname
need tar
need mktemp
need chmod
need mkdir
need cp
need grep
need tr
need rm

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux)
    ;;
  *)
    die "unsupported OS: $os"
    ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64)
    arch="amd64"
    ;;
  aarch64|arm64)
    arch="arm64"
    ;;
  *)
    die "unsupported architecture: $machine"
    ;;
esac

archive="${BIN_NAME}_linux_${arch}.tar.gz"

if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

archive_url="${base_url}/${archive}"
checksums_url="${base_url}/checksums.txt"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

log "Downloading ${archive_url}"
download "$archive_url" "$tmp_dir/$archive"

log "Downloading ${checksums_url}"
download "$checksums_url" "$tmp_dir/checksums.txt"

log "Verifying checksum"
(
  cd "$tmp_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  ${archive}\$" checksums.txt | sha256sum -c -
  elif command -v shasum >/dev/null 2>&1; then
    grep "  ${archive}\$" checksums.txt | shasum -a 256 -c -
  else
    die "missing required command: sha256sum or shasum"
  fi
)

log "Extracting ${archive}"
tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"

[ -f "$tmp_dir/$BIN_NAME" ] || die "archive does not contain ${BIN_NAME}"

mkdir -p "$INSTALL_DIR"
cp "$tmp_dir/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod 755 "$INSTALL_DIR/$BIN_NAME"

log "Installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
"$INSTALL_DIR/$BIN_NAME" --version

case ":$PATH:" in
  *:"$INSTALL_DIR":*)
    ;;
  *)
    log ""
    log "Warning: ${INSTALL_DIR} is not in PATH."
    log "Add this to your shell profile:"
    log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac