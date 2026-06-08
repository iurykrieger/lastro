#!/usr/bin/env bash
# harness-tools.sh — platform-aware launcher for the harness-tools binary.
# Usage: harness-tools.sh <subcommand> [args...]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$SCRIPT_DIR")"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
esac
case "$OS" in
  darwin|linux) BIN="${PLUGIN_DIR}/bin/${OS}-${ARCH}/lastro" ;;
  msys*|mingw*|cygwin*|windows*) BIN="${PLUGIN_DIR}/bin/windows-amd64/lastro.exe" ;;
  *)
    echo "harness-tools.sh: unsupported platform ${OS}-${ARCH} (no lastro binary)" >&2
    exit 1
    ;;
esac

if [[ ! -x "$BIN" ]]; then
  echo "harness-tools.sh: lastro binary not found at ${BIN}" >&2
  echo "Build with: make build-all  (from ${PLUGIN_DIR})" >&2
  exit 1
fi
exec "$BIN" "$@"
