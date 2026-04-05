#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Uso:
#   VSCE_PAT="<token>" ./scripts/publish-marketplace.sh [patch|minor|major|x.y.z]
# Ejemplos:
#   VSCE_PAT="***" ./scripts/publish-marketplace.sh patch
#   VSCE_PAT="***" ./scripts/publish-marketplace.sh 0.1.1

BUMP_OR_VERSION="${1:-patch}"

if [[ ! "$BUMP_OR_VERSION" =~ ^(patch|minor|major|[0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "Error: argumento invalido '$BUMP_OR_VERSION'."
  echo "Usa: patch | minor | major | x.y.z"
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "Error: node no esta disponible en PATH."
  exit 1
fi

PUBLISHER="$(node -p "(function(){try{return require('./package.json').publisher||''}catch(e){return ''}})()")"
EXT_NAME="$(node -p "(function(){try{return require('./package.json').name||''}catch(e){return ''}})()")"

if [[ -z "$PUBLISHER" ]]; then
  echo "Error: falta el campo 'publisher' en package.json."
  echo "Agrega, por ejemplo: \"publisher\": \"tu-publisher\""
  exit 1
fi

if [[ -z "$EXT_NAME" ]]; then
  echo "Error: falta el campo 'name' en package.json."
  exit 1
fi

if [[ -z "${VSCE_PAT:-}" ]]; then
  echo "Error: falta VSCE_PAT en el entorno."
  echo "Obtiene el token en Azure DevOps (Marketplace publisher) y ejecuta:"
  echo "VSCE_PAT=\"<token>\" ./scripts/publish-marketplace.sh $BUMP_OR_VERSION"
  exit 1
fi

if command -v vsce >/dev/null 2>&1; then
  VSCE_BIN="vsce"
elif [[ -x "./node_modules/.bin/vsce" ]]; then
  VSCE_BIN="./node_modules/.bin/vsce"
else
  echo "Error: no se encontro vsce."
  echo "Instala con: npm install -g @vscode/vsce"
  exit 1
fi

echo "[1/2] Compilando extension..."
npm run build

echo "[2/2] Publicando en Marketplace: $PUBLISHER.$EXT_NAME ($BUMP_OR_VERSION)"
"$VSCE_BIN" publish "$BUMP_OR_VERSION" --pat "$VSCE_PAT" --allow-missing-repository --skip-license

echo "Publicacion completada."
