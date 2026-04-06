#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Buscar .env: primero en INSTALL/, luego en la raíz del proyecto
if [ -f "$SCRIPT_DIR/.env" ]; then
  set -a
  source "$SCRIPT_DIR/.env"
  set +a
elif [ -f "$PROJECT_ROOT/.env" ]; then
  set -a
  source "$PROJECT_ROOT/.env"
  set +a
else
  echo "⚠️  Archivo .env no encontrado en $SCRIPT_DIR ni en $PROJECT_ROOT" >&2
fi

cd "$PROJECT_ROOT/api" && go run ./cmd/server
