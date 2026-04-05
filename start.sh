#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -f "$SCRIPT_DIR/.env" ]; then
  set -a
  source "$SCRIPT_DIR/.env"
  set +a
else
  echo "⚠️  Archivo .env no encontrado en $SCRIPT_DIR" >&2
fi

cd "$SCRIPT_DIR/api" && go run ./cmd/server
