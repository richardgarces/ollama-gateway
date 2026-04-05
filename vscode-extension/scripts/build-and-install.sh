#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "[1/3] Compilando extension..."
npm run build

echo "[2/3] Empaquetando VSIX..."
if command -v vsce >/dev/null 2>&1; then
  VSCE_BIN="vsce"
elif [[ -x "./node_modules/.bin/vsce" ]]; then
  VSCE_BIN="./node_modules/.bin/vsce"
else
  echo "Error: no se encontro vsce."
  echo "Instala con: npm install -g @vscode/vsce"
  exit 1
fi

"$VSCE_BIN" package --allow-missing-repository --skip-license

VSIX_FILE="$(ls -t ./*.vsix | head -n 1)"
echo "VSIX generado: $VSIX_FILE"

echo "[3/3] Instalando extension en VS Code..."
if command -v code >/dev/null 2>&1; then
  code --install-extension "$VSIX_FILE" --force
  echo "Instalacion completa."
  echo "Si no aparece de inmediato, ejecuta Reload Window en VS Code."
else
  echo "Aviso: comando 'code' no disponible en PATH."
  echo "Instala manualmente el VSIX desde: $VSIX_FILE"
fi
