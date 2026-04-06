#!/usr/bin/env bash
# ============================================================
#  Checklist de validación — Ollama SaaS Gateway
#  Se puede ejecutar desde cualquier ubicación:
#    bash INSTALL/checklist.sh
#    bash INSTALL/checklist.sh --all         (ejecuta todo sin menú)
#    bash INSTALL/checklist.sh --category 3  (ejecuta categoría 3)
# ============================================================
set -uo pipefail

# ── Resolver raíz del proyecto ───────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# ── Colores ──────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # sin color

# ── Contadores globales ─────────────────────────────────────
PASS=0
FAIL=0
WARN=0

# ── Función de check individual ─────────────────────────────
# Uso: check "label" "comando" "texto esperado" "fix hint" [required=true]
check() {
  local label="$1" cmd="$2" expected="$3" hint="${4:-}" required="${5:-true}"
  printf "  %-55s" "$label"
  result=$(eval "$cmd" 2>/dev/null) || true
  if echo "$result" | grep -qi "$expected"; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
    return 0
  elif [ "$required" = "false" ]; then
    printf "${YELLOW}⚠️  Opcional — no disponible${NC}\n"
    if [ -n "$hint" ]; then
      echo -e "    ${DIM}💡 $hint${NC}"
    fi
    ((WARN++))
    return 0
  else
    printf "${RED}❌ FALLÓ${NC}\n"
    if [ -n "$hint" ]; then
      echo -e "    ${RED}💡 $hint${NC}"
    fi
    ((FAIL++))
    return 1
  fi
}

# ── Resumen de resultados ───────────────────────────────────
print_summary() {
  local total=$((PASS + FAIL + WARN))
  echo ""
  echo -e "${BOLD}──────────────────────────────────────────────────────────${NC}"
  printf "  Total: %d checks  |  ${GREEN}✅ %d ok${NC}  |  ${RED}❌ %d fallos${NC}  |  ${YELLOW}⚠️  %d opcionales${NC}\n" \
    "$total" "$PASS" "$FAIL" "$WARN"
  echo -e "${BOLD}──────────────────────────────────────────────────────────${NC}"
  if [ "$FAIL" -eq 0 ]; then
    echo -e "\n  ${GREEN}${BOLD}🎉 ¡Todo correcto!${NC}\n"
  else
    echo -e "\n  ${RED}${BOLD}⚡ Hay $FAIL checks fallidos. Revisa los pasos correspondientes.${NC}\n"
  fi
}

reset_counters() {
  PASS=0; FAIL=0; WARN=0
}

# ════════════════════════════════════════════════════════════
#  CATEGORÍAS DE VALIDACIÓN
# ════════════════════════════════════════════════════════════

cat_herramientas() {
  echo ""
  echo -e "${CYAN}${BOLD}[1] Herramientas del sistema${NC}"
  echo -e "${DIM}    Verifica que git, go, docker, curl y jq están instalados${NC}"
  echo ""
  check "Git instalado"                "git --version"            "git version"
  check "Go instalado (≥1.24)"         "go version"               "go1.2"
  check "Docker instalado"             "docker --version"         "Docker version"
  check "Docker Compose instalado"     "docker compose version"   "Docker Compose"
  check "curl instalado"               "curl --version"           "curl"
  check "jq instalado"                 "jq --version"             "jq"
}

cat_contenedores() {
  echo ""
  echo -e "${CYAN}${BOLD}[2] Contenedores Docker${NC}"
  echo -e "${DIM}    Verifica que los contenedores de los backends estén corriendo${NC}"
  echo -e "${DIM}    (Ollama corre de forma independiente, se valida en conectividad)${NC}"
  echo ""
  check "Contenedor Qdrant corriendo" \
    "docker ps --format '{{.Names}}' | grep -c qdrant" \
    "1" \
    "Levantar: docker compose -f docker-compose.qdrant.yml up -d"

  check "Contenedor MongoDB corriendo" \
    "docker ps --format '{{.Names}}' | grep -c mongo" \
    "1" \
    "Levantar: docker compose -f docker-compose.mongo.yml up -d"

  check "Contenedor Redis corriendo" \
    "docker ps --format '{{.Names}}' | grep -c redis" \
    "1" \
    "Levantar: docker run -d --name redis-gateway -p 6379:6379 redis:7-alpine" \
    "false"
}

cat_conectividad() {
  echo ""
  echo -e "${CYAN}${BOLD}[3] Conectividad de backends${NC}"
  echo -e "${DIM}    Verifica que cada backend responde en su puerto${NC}"
  echo ""
  check "Ollama corriendo (proceso independiente)" \
    "pgrep -x ollama >/dev/null 2>&1 && echo 'running' || curl -sf -m 2 http://localhost:11434/ >/dev/null 2>&1 && echo 'running'" "running"

  check "Ollama responde (HTTP :11434)" \
    "curl -sf -m 5 -o /dev/null -w '%{http_code}' http://localhost:11434/" "200"

  check "Qdrant responde (HTTP :6333)" \
    "curl -sf -m 5 -o /dev/null -w '%{http_code}' http://localhost:6333/"  "200"

  check "MongoDB acepta conexión (TCP :27017)" \
    "(docker exec \$(docker ps -qf name=mongo) mongosh --quiet --eval 'db.adminCommand(\"ping\").ok' -u admin -p changeme --authenticationDatabase admin 2>/dev/null || docker exec \$(docker ps -qf name=mongo) mongo --quiet --eval 'db.adminCommand(\"ping\").ok' -u admin -p changeme --authenticationDatabase admin 2>/dev/null || nc -z localhost 27017 2>/dev/null && echo 1)" \
    "1" \
    "Verificar: docker compose -f docker-compose.mongo.yml logs / ¿Puerto 27017 libre? (lsof -i :27017) / ¿Credenciales? (admin:changeme)"

  check "Redis responde PONG" \
    "docker exec \$(docker ps -qf name=redis) redis-cli ping" "PONG" \
    "Levantar: docker run -d --name redis-gateway -p 6379:6379 redis:7-alpine" \
    "false"
}

cat_api() {
  echo ""
  echo -e "${CYAN}${BOLD}[4] API Gateway${NC}"
  echo -e "${DIM}    Verifica liveness, readiness y estado de dependencias vía la API${NC}"
  echo ""
  check "API liveness (:8081/health)" \
    "curl -sf -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/health" "200"

  check "API readiness — responde" \
    "curl -s -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/health/readiness" "200\|503" \
    "¿API corriendo? Revisar detalle: curl -s http://localhost:8081/health/readiness | jq ."

  # Mostrar status actual
  local readiness_status
  readiness_status=$(curl -sf -m 5 http://localhost:8081/health/readiness 2>/dev/null | jq -r .status 2>/dev/null) || readiness_status=""
  if [ -n "$readiness_status" ]; then
    printf "  %-55s" "API readiness status"
    if [ "$readiness_status" = "healthy" ]; then
      printf "${GREEN}✅ $readiness_status${NC}\n"
      ((PASS++))
    else
      printf "${YELLOW}⚠️  $readiness_status${NC}\n"
      echo -e "    ${YELLOW}💡 Revisar: curl -s http://localhost:8081/health/readiness | jq .dependencies${NC}"
      ((WARN++))
    fi
  fi

  check "Ollama healthy (vía readiness)" \
    "curl -s -m 5 http://localhost:8081/health/readiness | jq -r '.dependencies.ollama.status'" "healthy" \
    "Ollama no responde. Verificar: ollama serve / ¿OLLAMA_URL correcto en .env?"

  check "Qdrant healthy (vía readiness)" \
    "curl -s -m 5 http://localhost:8081/health/readiness | jq -r '.dependencies.qdrant.status'" "healthy" \
    "Qdrant no responde. Verificar: docker compose -f docker-compose.qdrant.yml ps / ¿QDRANT_URL correcto en .env?"

  check "MongoDB healthy (vía readiness)" \
    "curl -s -m 5 http://localhost:8081/health/readiness | jq -r '.dependencies.mongo.status'"  "healthy" \
    "MongoDB no responde. Verificar: docker compose -f docker-compose.mongo.yml ps / ¿MONGO_URI correcto en .env?"
}

cat_modelos() {
  echo ""
  echo -e "${CYAN}${BOLD}[5] Modelos de Ollama${NC}"
  echo -e "${DIM}    Verifica que hay modelos descargados y listos para usar${NC}"
  echo ""

  local models model_count
  models=$(curl -sf -m 5 http://localhost:11434/api/tags 2>/dev/null | jq -r '.models[].name' 2>/dev/null) || models=""
  model_count=$(echo "$models" | grep -c '.' 2>/dev/null) || model_count=0

  printf "  %-55s" "Modelos instalados en Ollama"
  if [ "$model_count" -gt 0 ]; then
    printf "${GREEN}✅ $model_count encontrados${NC}\n"
    echo -e "    ${DIM}$(echo "$models" | tr '\n' ', ' | sed 's/,$//')${NC}"
    ((PASS++))
  else
    printf "${RED}❌ Ninguno${NC}\n"
    ((FAIL++))
  fi

  check "Modelo de embeddings (nomic-embed-text:latest)" \
    "curl -sf -m 5 http://localhost:11434/api/tags | jq -r '.models[].name' | grep -c nomic-embed-text:latest" "1"
}

cat_auth() {
  echo ""
  echo -e "${CYAN}${BOLD}[6] Autenticación (JWT)${NC}"
  echo -e "${DIM}    Verifica login y obtención de token${NC}"
  echo ""

  local token
  token=$(curl -sf -m 5 -X POST http://localhost:8081/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin"}' 2>/dev/null | jq -r .token 2>/dev/null) || token=""

  printf "  %-55s" "Login funciona — token JWT obtenido"
  if [ -n "$token" ] && [ "$token" != "null" ]; then
    printf "${GREEN}✅ OK${NC}\n"
    echo -e "    ${DIM}Token: ${token:0:20}...${NC}"
    ((PASS++))
  else
    printf "${RED}❌ FALLÓ${NC}\n"
    ((FAIL++))
  fi
}

cat_inferencia() {
  echo ""
  echo -e "${CYAN}${BOLD}[7] Inferencia (Chat)${NC}"
  echo -e "${DIM}    Envía un prompt al modelo y verifica que responde (puede tardar ~10-30s)${NC}"
  echo ""

  # Auto-detectar un modelo de chat (excluir embeddings)
  local chat_model
  chat_model=$(curl -sf -m 5 http://localhost:11434/api/tags 2>/dev/null \
    | jq -r '[.models[].name | select(test("embed|bge|e5-") | not)][0] // empty' 2>/dev/null) || chat_model=""

  if [ -z "$chat_model" ]; then
    printf "  %-55s" "Modelo de chat disponible"
    printf "${RED}❌ No hay modelo de chat${NC}\n"
    echo -e "    ${RED}💡 Descargar uno: curl http://localhost:11434/api/pull -d '{\"name\":\"phi3:latest\"}'${NC}"
    ((FAIL++))
    return
  fi

  printf "  %-55s" "Modelo de chat detectado"
  printf "${GREEN}✅ $chat_model${NC}\n"
  ((PASS++))

  printf "  %-55s" "Inferencia con $chat_model"
  local response
  response=$(curl -sf -m 180 -X POST http://localhost:8081/openai/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$chat_model\",\"messages\":[{\"role\":\"user\",\"content\":\"Responde solo: OK\"}],\"stream\":false}" 2>/dev/null) || response=""

  # DEBUG — descomentar si falla:
  echo "DBG response_len=${#response} response=${response:0:200}" >&2

  local content
  content=$(printf '%s\n' "$response" | jq -r '.choices[0].message.content // empty' 2>/dev/null) || content=""

  if [ -n "$content" ] && [ "$content" != "null" ]; then
    printf "${GREEN}✅ OK${NC}\n"
    echo -e "    ${DIM}Respuesta: ${content:0:80}${NC}"
    ((PASS++))
  else
    printf "${RED}❌ FALLÓ${NC}\n"
    echo -e "    ${RED}💡 ¿API corriendo? ¿Ollama respondiendo? Probar directo: curl http://localhost:11434/api/generate -d '{\"model\":\"$chat_model\",\"prompt\":\"hola\"}'${NC}"
    ((FAIL++))
  fi
}

cat_endpoints() {
  echo ""
  echo -e "${CYAN}${BOLD}[8] Endpoints adicionales${NC}"
  echo -e "${DIM}    Verifica que los endpoints principales de la API responden${NC}"
  echo ""
  check "GET  /api/models" \
    "curl -sf -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/api/models" "200"

  check "GET  /metrics" \
    "curl -sf -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/metrics" "200"

  check "GET  /health/readiness/ollama" \
    "curl -s -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/health/readiness/ollama" "200\|503" \
    "Ruta no encontrada (404). ¿Versión de la API actualizada? Recompilar: cd api && go build ./..."

  check "GET  /health/readiness/qdrant" \
    "curl -s -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/health/readiness/qdrant" "200\|503" \
    "Ruta no encontrada (404). ¿Versión de la API actualizada? Recompilar: cd api && go build ./..."

  check "GET  /health/readiness/mongo" \
    "curl -s -m 5 -o /dev/null -w '%{http_code}' http://localhost:8081/health/readiness/mongo"  "200\|503" \
    "Ruta no encontrada (404). ¿Versión de la API actualizada? Recompilar: cd api && go build ./..."
}

cat_config() {
  echo ""
  echo -e "${CYAN}${BOLD}[9] Configuración (.env)${NC}"
  echo -e "${DIM}    Verifica que el archivo .env existe y tiene valores clave${NC}"
  echo ""

  printf "  %-55s" "Archivo .env existe"
  if [ -f ".env" ]; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
  else
    printf "${RED}❌ No encontrado${NC}\n"
    echo -e "    ${RED}💡 Crear: cp .env.example .env  y editar los valores${NC}"
    ((FAIL++))
    return
  fi

  printf "  %-55s" "JWT_SECRET configurado"
  if grep -q "JWT_SECRET" .env && ! grep -q "PEGA-AQUI" .env && ! grep -q "change-this" .env; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
  else
    printf "${RED}❌ Falta o tiene placeholder${NC}\n"
    ((FAIL++))
  fi

  printf "  %-55s" "OLLAMA_URL configurado"
  if grep -q "OLLAMA_URL" .env; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
  else
    printf "${RED}❌ Falta${NC}\n"
    ((FAIL++))
  fi

  printf "  %-55s" "QDRANT_URL configurado"
  if grep -q "QDRANT_URL" .env; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
  else
    printf "${RED}❌ Falta${NC}\n"
    ((FAIL++))
  fi

  printf "  %-55s" "MONGO_URI configurado"
  if grep -q "MONGO_URI" .env; then
    printf "${GREEN}✅ OK${NC}\n"
    ((PASS++))
  else
    printf "${RED}❌ Falta${NC}\n"
    ((FAIL++))
  fi
}

# ════════════════════════════════════════════════════════════
#  EJECUTAR TODO
# ════════════════════════════════════════════════════════════

run_all() {
  reset_counters
  echo ""
  echo -e "${BOLD}══════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}  Validación completa — Ollama SaaS Gateway${NC}"
  echo -e "${BOLD}══════════════════════════════════════════════════════════${NC}"

  cat_herramientas
  cat_contenedores
  cat_conectividad
  cat_api
  cat_modelos
  cat_auth
  cat_inferencia
  cat_endpoints
  cat_config

  print_summary
}

run_category() {
  reset_counters
  case "$1" in
    1) cat_herramientas ;;
    2) cat_contenedores ;;
    3) cat_conectividad ;;
    4) cat_api ;;
    5) cat_modelos ;;
    6) cat_auth ;;
    7) cat_inferencia ;;
    8) cat_endpoints ;;
    9) cat_config ;;
    *) echo -e "${RED}Categoría inválida: $1${NC}"; return 1 ;;
  esac
  print_summary
}

# ════════════════════════════════════════════════════════════
#  MENÚ INTERACTIVO
# ════════════════════════════════════════════════════════════

show_menu() {
  echo ""
  echo -e "${BOLD}══════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}  Checklist de Validación — Ollama SaaS Gateway${NC}"
  echo -e "${BOLD}══════════════════════════════════════════════════════════${NC}"
  echo ""
  echo -e "  ${CYAN}[0]${NC}  Ejecutar TODOS los checks"
  echo ""
  echo -e "  ${CYAN}[1]${NC}  Herramientas del sistema    ${DIM}(git, go, docker, curl, jq)${NC}"
  echo -e "  ${CYAN}[2]${NC}  Contenedores Docker         ${DIM}(ollama, qdrant, mongo, redis)${NC}"
  echo -e "  ${CYAN}[3]${NC}  Conectividad de backends    ${DIM}(HTTP/TCP directo a cada uno)${NC}"
  echo -e "  ${CYAN}[4]${NC}  API Gateway                 ${DIM}(liveness, readiness, deps)${NC}"
  echo -e "  ${CYAN}[5]${NC}  Modelos de Ollama           ${DIM}(modelos descargados)${NC}"
  echo -e "  ${CYAN}[6]${NC}  Autenticación (JWT)         ${DIM}(login y token)${NC}"
  echo -e "  ${CYAN}[7]${NC}  Inferencia (Chat)           ${DIM}(enviar prompt, recibir respuesta)${NC}"
  echo -e "  ${CYAN}[8]${NC}  Endpoints adicionales       ${DIM}(models, metrics, readiness/*)${NC}"
  echo -e "  ${CYAN}[9]${NC}  Configuración (.env)        ${DIM}(variables de entorno)${NC}"
  echo ""
  echo -e "  ${DIM}[q]  Salir${NC}"
  echo ""
}

interactive_menu() {
  while true; do
    show_menu
    printf "  Elige una opción: "
    read -r choice

    case "$choice" in
      0)
        run_all
        ;;
      [1-9])
        run_category "$choice"
        ;;
      q|Q)
        echo -e "\n  ${DIM}¡Hasta luego!${NC}\n"
        exit 0
        ;;
      *)
        echo -e "\n  ${RED}Opción inválida. Usa 0-9 o q para salir.${NC}"
        ;;
    esac

    echo ""
    printf "  ${DIM}Presiona Enter para volver al menú...${NC}"
    read -r
  done
}

# ════════════════════════════════════════════════════════════
#  PUNTO DE ENTRADA
# ════════════════════════════════════════════════════════════

usage() {
  echo "Uso: bash docs/checklist.sh [opciones]"
  echo ""
  echo "Sin opciones:   Muestra menú interactivo"
  echo ""
  echo "Opciones:"
  echo "  --all              Ejecutar todos los checks (sin menú)"
  echo "  --category N       Ejecutar solo la categoría N (1-9)"
  echo "  --list             Listar categorías disponibles"
  echo "  -h, --help         Mostrar esta ayuda"
}

# Parsear argumentos de línea de comandos
if [ $# -eq 0 ]; then
  interactive_menu
else
  case "$1" in
    --all)
      run_all
      ;;
    --category)
      if [ -z "${2:-}" ]; then
        echo "Error: --category requiere un número (1-9)"
        exit 1
      fi
      run_category "$2"
      ;;
    --list)
      echo ""
      echo "Categorías disponibles:"
      echo "  1  Herramientas del sistema"
      echo "  2  Contenedores Docker"
      echo "  3  Conectividad de backends"
      echo "  4  API Gateway"
      echo "  5  Modelos de Ollama"
      echo "  6  Autenticación (JWT)"
      echo "  7  Inferencia (Chat)"
      echo "  8  Endpoints adicionales"
      echo "  9  Configuración (.env)"
      echo ""
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "Opción desconocida: $1"
      usage
      exit 1
      ;;
  esac
fi
