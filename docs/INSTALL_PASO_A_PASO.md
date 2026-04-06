# Guía de Instalación Paso a Paso — Ollama SaaS Gateway

> Manual para principiantes. Cada servicio se instala y verifica por separado.
> Tiempo estimado: 30-45 minutos (depende de tu conexión a internet).

---

## Índice

1. [Qué vamos a instalar](#1-qué-vamos-a-instalar)
2. [Requisitos previos](#2-requisitos-previos)
3. [Paso 1 — Clonar el proyecto](#3-paso-1--clonar-el-proyecto)
4. [Paso 2 — Instalar y levantar Ollama](#4-paso-2--instalar-y-levantar-ollama)
5. [Paso 3 — Instalar y levantar Qdrant](#5-paso-3--instalar-y-levantar-qdrant)
6. [Paso 4 — Instalar y levantar MongoDB](#6-paso-4--instalar-y-levantar-mongodb)
7. [Paso 5 — (Opcional) Instalar y levantar Redis](#7-paso-5--opcional-instalar-y-levantar-redis)
8. [Paso 6 — Configurar variables de entorno](#8-paso-6--configurar-variables-de-entorno)
9. [Paso 7 — Compilar y ejecutar la API](#9-paso-7--compilar-y-ejecutar-la-api)
10. [Paso 8 — Descargar un modelo en Ollama](#10-paso-8--descargar-un-modelo-en-ollama)
11. [Paso 9 — Verificar que todo funciona](#11-paso-9--verificar-que-todo-funciona)
12. [Paso 10 — (Opcional) Instalar la extensión VS Code](#12-paso-10--opcional-instalar-la-extensión-vs-code)
13. [Diagrama de arquitectura](#13-diagrama-de-arquitectura)
14. [Resolución de problemas frecuentes](#14-resolución-de-problemas-frecuentes)
15. [Referencia rápida de puertos](#15-referencia-rápida-de-puertos)
16. [Checklist de validación final](#16-checklist-de-validación-final)

---

## 1. Qué vamos a instalar

La aplicación tiene **5 componentes** que se instalan por separado:

| Componente | Qué hace | Puerto |
|---|---|---|
| **Ollama** | Motor de modelos de IA (LLM). Ejecuta los modelos de lenguaje | `11434` |
| **Qdrant** | Base de datos vectorial. Permite buscar código por similitud (RAG) | `6333` |
| **MongoDB** | Base de datos. Guarda perfiles, historial, sesiones | `27017` |
| **Redis** | Cache en memoria (opcional). Mejora velocidad si se activa | `6379` |
| **API Gateway** | El servidor Go que conecta todo y expone los endpoints | `8081` |

```
┌───────────┐     ┌───────────┐     ┌───────────┐
│  Ollama   │     │  Qdrant   │     │  MongoDB  │
│  :11434   │     │  :6333    │     │  :27017   │
└─────┬─────┘     └─────┬─────┘     └─────┬─────┘
      │                 │                 │
      └────────┬────────┴────────┬────────┘
               │                 │
         ┌─────┴─────────────────┴─────┐
         │      API Gateway (Go)       │
         │          :8081              │
         └─────────────┬───────────────┘
                       │
                 ┌─────┴─────┐
                 │  Tu app / │
                 │  VS Code  │
                 └───────────┘
```

---

## 2. Requisitos previos

Antes de empezar, necesitas instalar estas herramientas en tu computadora.

### 2.1 — Git

**macOS:**
```bash
# Si no lo tienes, se instala al ejecutar cualquier comando git por primera vez
git --version
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update && sudo apt install -y git
```

**Windows:**
Descargar de https://git-scm.com/download/win e instalar con las opciones por defecto.

### 2.2 — Docker y Docker Compose

Docker se usa para levantar Ollama, Qdrant, MongoDB y Redis sin instalar nada más.

**macOS:**
1. Descargar Docker Desktop desde https://www.docker.com/products/docker-desktop/
2. Abrir el `.dmg` descargado y arrastrar Docker a Aplicaciones.
3. Abrir Docker Desktop y esperar a que el ícono de la ballena deje de moverse.

**Linux (Ubuntu/Debian):**
```bash
# Instalar Docker
curl -fsSL https://get.docker.com | sudo sh

# Agregar tu usuario al grupo docker (para no usar sudo)
sudo usermod -aG docker $USER

# Cerrar sesión y volver a abrirla para que tome efecto
# Luego verificar:
docker --version
docker compose version
```

**Windows:**
1. Descargar Docker Desktop desde https://www.docker.com/products/docker-desktop/
2. Ejecutar el instalador y reiniciar si lo pide.
3. Abrir Docker Desktop y asegurarse de que está corriendo.

### 2.3 — Go 1.24 o superior

**macOS:**
```bash
brew install go
```

**Linux:**
```bash
# Descargar la última versión (ajustar número si hay más nueva)
wget https://go.dev/dl/go1.24.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.2.linux-amd64.tar.gz

# Agregar al PATH (poner en ~/.bashrc o ~/.zshrc para que persista)
export PATH=$PATH:/usr/local/go/bin
```

**Windows:**
Descargar el instalador `.msi` desde https://go.dev/dl/ y ejecutar.

### 2.4 — curl y jq (herramientas auxiliares)

```bash
# macOS
brew install curl jq

# Linux (Ubuntu/Debian)
sudo apt install -y curl jq

# Windows (con chocolatey)
choco install curl jq
```

### 2.5 — Verificar todo

Ejecuta estos comandos. Todos deben responder con una versión:

```bash
git --version        # git version 2.x.x
go version           # go version go1.24.x ...
docker --version     # Docker version 2x.x.x
docker compose version  # Docker Compose version v2.x.x
curl --version       # curl 8.x.x ...
jq --version         # jq-1.x
```

> **⚠️ Si alguno falla, resuélvelo antes de continuar.**

---

## 3. Paso 1 — Clonar el proyecto

```bash
# Ir al directorio donde quieres el proyecto
cd ~/proyectos   # o donde prefieras

# Clonar
git clone <URL-DE-TU-REPOSITORIO> ollama_saas_project

# Entrar al directorio
cd ollama_saas_project
```

Verifica que ves estos archivos:

```bash
ls -la
# Deberías ver: docker-compose.yml, Makefile, api/, vscode-extension/, docs/ ...
```

---

## 4. Paso 2 — Instalar y levantar Ollama

Ollama es el motor de IA. Ejecuta los modelos de lenguaje en tu máquina.

### 4.1 — Levantar con Docker

```bash
docker compose -f docker-compose.ollama.yml up -d
```

Esto descarga la imagen de Ollama y la de Open WebUI (interfaz web). La primera vez tardará unos minutos.

### 4.2 — Verificar que arrancó

Espera 15–20 segundos y ejecuta:

```bash
curl http://localhost:11434/
```

Deberías ver: `Ollama is running`

Si quieres ver la interfaz web de Ollama, abre en tu navegador: http://localhost:3000

### 4.3 — Verificar que el contenedor sigue vivo

```bash
docker compose -f docker-compose.ollama.yml ps
```

Deberías ver `ollama` con estado `Up` y `(healthy)`.

### 4.4 — Si algo falla

```bash
# Ver los logs
docker compose -f docker-compose.ollama.yml logs ollama

# Reiniciar
docker compose -f docker-compose.ollama.yml restart ollama
```

---

## 5. Paso 3 — Instalar y levantar Qdrant

Qdrant es la base de datos vectorial. Permite buscar código y documentos por similitud semántica.

### 5.1 — Levantar con Docker

```bash
docker compose -f docker-compose.qdrant.yml up -d
```

### 5.2 — Verificar

Espera 10 segundos:

```bash
curl http://localhost:6333/
```

Deberías ver un JSON con `"title":"qdrant"` y `"version":"..."`.

### 5.3 — Dashboard web de Qdrant

Abre en tu navegador: http://localhost:6333/dashboard

### 5.4 — Si algo falla

```bash
docker compose -f docker-compose.qdrant.yml logs qdrant
docker compose -f docker-compose.qdrant.yml restart qdrant
```

---

## 6. Paso 4 — Instalar y levantar MongoDB

MongoDB guarda perfiles de usuario, historial de conversaciones, flags de features y otros datos persistentes.

### 6.1 — Levantar con Docker

```bash
docker compose -f docker-compose.mongo.yml up -d
```

### 6.2 — Verificar

```bash
docker compose -f docker-compose.mongo.yml ps
```

Deberías ver `mongo` con estado `Up` y `(healthy)`.

Para probar la conexión directamente:

```bash
docker exec -it $(docker compose -f docker-compose.mongo.yml ps -q mongo) \
  mongosh --eval "db.adminCommand('ping')" \
  -u admin -p changeme --authenticationDatabase admin
```

Deberías ver: `{ ok: 1 }`

### 6.3 — Credenciales por defecto

| Campo | Valor |
|---|---|
| Usuario | `admin` |
| Contraseña | `changeme` |
| Puerto | `27017` (solo accesible desde localhost) |

> **⚠️ Cambia la contraseña en producción.** Se configura con la variable `MONGO_PASSWORD`.

### 6.4 — Si algo falla

```bash
docker compose -f docker-compose.mongo.yml logs mongo
docker compose -f docker-compose.mongo.yml restart mongo
```

---

## 7. Paso 5 — (Opcional) Instalar y levantar Redis

Redis se usa como cache. **Es opcional** — por defecto la API usa cache en memoria. Actívalo si quieres cache persistente o compartida.

### 7.1 — Levantar con Docker

```bash
docker run -d --name redis-gateway \
  -p 6379:6379 \
  --restart unless-stopped \
  redis:7-alpine
```

### 7.2 — Verificar

```bash
docker exec redis-gateway redis-cli ping
```

Deberías ver: `PONG`

### 7.3 — Conectar Redis a la API

Más adelante, en las variables de entorno (Paso 6), configura:

```bash
CACHE_BACKEND=redis
REDIS_URL=redis://localhost:6379/0
```

Si **no** instalas Redis, no hagas nada — la API usará cache en memoria automáticamente.

---

## 8. Paso 6 — Configurar variables de entorno

La API necesita saber dónde están los servicios. Crea un archivo `.env` con la configuración.

### 8.1 — Copiar el archivo de ejemplo

```bash
cp .env.example .env
```

### 8.2 — Editar `.env` para desarrollo local

Abre `.env` con tu editor favorito y asegúrate de que tiene estos valores:

```bash
# ---------- OBLIGATORIOS ----------

# Puerto de la API
PORT=8081

# Secreto para tokens JWT (genera uno único para ti)
# En macOS/Linux ejecuta: openssl rand -hex 32
JWT_SECRET=PEGA-AQUI-EL-RESULTADO-DE-openssl-rand-hex-32

# Dónde está Ollama
OLLAMA_URL=http://localhost:11434

# Dónde está Qdrant
QDRANT_URL=http://localhost:6333

# Dónde está MongoDB (usuario:contraseña@host:puerto)
MONGO_URI=mongodb://admin:changeme@localhost:27017

# Directorio del repositorio (déjalo en . si vas a correr la API desde api/)
REPO_ROOT=.

# ---------- OPCIONALES (puedes dejar los defaults) ----------

# Modelo de chat (se configurará en Paso 8)
CHAT_MODEL=qwen2.5-coder:7b

# Modelo de embeddings para RAG
EMBEDDING_MODEL=nomic-embed-text

# Cache: 'memory' (default) o 'redis' (si instalaste Redis)
CACHE_BACKEND=memory
# REDIS_URL=redis://localhost:6379/0   # descomentar si usas redis

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Health checks
HEALTH_CHECK_TIMEOUT_MS=2000
```

### 8.3 — Generar el JWT_SECRET

```bash
openssl rand -hex 32
```

Copia el resultado y pégalo como valor de `JWT_SECRET` en `.env`.

### 8.4 — Contraseña de MongoDB

Si cambiaste `MONGO_PASSWORD` en el docker-compose, actualiza también `MONGO_URI`:

```bash
MONGO_URI=mongodb://admin:TU-NUEVA-CONTRASEÑA@localhost:27017
```

---

## 9. Paso 7 — Compilar y ejecutar la API

### 9.1 — Entrar al directorio de la API

```bash
cd api
```

### 9.2 — Descargar dependencias

```bash
go mod download
go mod tidy
```

La primera vez descargará paquetes (~2-3 minutos dependiendo de tu conexión).

### 9.3 — Compilar

```bash
go build -o bin/server ./cmd/server
```

Si no hay errores, verás el binario en `api/bin/server`.

### 9.4 — Exportar variables de entorno

La API lee las variables del proceso. Cárgalas desde el `.env`:

```bash
# Desde el directorio raíz del proyecto (no api/)
cd ..

# Cargar variables (macOS / Linux)
set -a
source .env
set +a

# Volver al directorio de la API
cd api
```

> **En Windows (PowerShell):**
> ```powershell
> Get-Content ..\.env | ForEach-Object {
>   if ($_ -match '^([^#][^=]+)=(.*)$') {
>     [Environment]::SetEnvironmentVariable($matches[1], $matches[2])
>   }
> }
> ```

### 9.5 — Ejecutar la API

```bash
./bin/server
```

Deberías ver en la consola algo como:

```
{"level":"info","msg":"server listening","port":"8081"}
```

> **Alternativa rápida** (sin compilar, útil para desarrollo):
> ```bash
> go run ./cmd/server
> ```

### 9.6 — Verificar que la API responde

Abre **otra terminal** y ejecuta:

```bash
curl http://localhost:8081/health
```

Deberías ver respuesta con status `200` (vacío está bien, es el liveness probe).

Para ver el estado detallado de backends:

```bash
curl -s http://localhost:8081/health/readiness | jq .
```

Deberías ver:

```json
{
  "status": "healthy",
  "dependencies": {
    "mongo": { "status": "healthy", "latency_ms": 1, ... },
    "ollama": { "status": "healthy", "latency_ms": 5, ... },
    "qdrant": { "status": "healthy", "latency_ms": 2, ... }
  },
  "breakers": { ... }
}
```

> **Si ves `"status": "degraded"` o `"unhealthy"`,** revisa qué dependencia falló y vuelve atrás a verificar ese servicio.

---

## 10. Paso 8 — Descargar un modelo en Ollama

Sin un modelo, la API no puede generar respuestas. Necesitas al menos **un modelo de chat** y **un modelo de embeddings**.

### 10.1 — Descargar modelo de chat (recomendado para empezar)

```bash
# Modelo pequeño y rápido (3.8GB) — bueno para empezar
curl http://localhost:11434/api/pull -d '{"name": "qwen2.5-coder:7b"}'
```

Espera a que termine de descargar (puede tardar varios minutos según tu conexión).

> **Modelos alternativos:**
> - `llama3.2:3b` — más pequeño (2GB), menos capaz pero más rápido
> - `codellama:13b` — más capaz (7GB), necesita más RAM/VRAM
> - `deepseek-coder:6.7b` — bueno para código

### 10.2 — Descargar modelo de embeddings (para RAG)

```bash
curl http://localhost:11434/api/pull -d '{"name": "nomic-embed-text:latest"}'
```

### 10.3 — Verificar modelos instalados

```bash
curl -s http://localhost:11434/api/tags | jq '.models[].name'
```

Deberías ver los nombres de los modelos que descargaste.

### 10.4 — Verificar modelos desde la API

```bash
curl -s http://localhost:8081/api/models | jq .
```

---

## 11. Paso 9 — Verificar que todo funciona

### 11.1 — Health check completo

```bash
curl -s http://localhost:8081/health/readiness | jq .
```

Todo debería estar en `"healthy"`.

### 11.2 — Health check de un backend individual

```bash
# Verificar solo Ollama
curl -s http://localhost:8081/health/readiness/ollama | jq .

# Verificar solo MongoDB
curl -s http://localhost:8081/health/readiness/mongo | jq .
```

### 11.3 — Obtener un token JWT

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

echo "Tu token: $TOKEN"
```

> Guarda este token; lo necesitarás para endpoints protegidos.

### 11.4 — Hacer tu primera pregunta al chat

```bash
curl -s -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-coder:7b",
    "messages": [{"role": "user", "content": "Hola, explica qué es Docker en 2 frases"}],
    "stream": false
  }' | jq .choices[0].message.content
```

Si ves una respuesta del modelo, **¡felicidades, todo funciona!** 🎉

### 11.5 — Probar streaming (respuesta en tiempo real)

```bash
curl -N -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-coder:7b",
    "messages": [{"role": "user", "content": "Escribe un hola mundo en Go"}],
    "stream": true
  }'
```

Verás fragmentos de respuesta llegando uno a uno (Server-Sent Events).

### 11.6 — Indexar tu repositorio (RAG)

Para que la API pueda responder preguntas sobre tu código:

```bash
curl -s -X POST http://localhost:8081/internal/index/reindex \
  -H "Authorization: Bearer $TOKEN" | jq .
```

### 11.7 — Ver métricas

```bash
curl -s http://localhost:8081/metrics | jq .
```

---

## 12. Paso 10 — (Opcional) Instalar la extensión VS Code

La extensión te permite usar todo el poder del gateway directamente desde tu editor.

### 12.1 — Requisitos

- VS Code instalado
- Node.js 18+ instalado

```bash
node --version   # Debe ser v18 o superior
```

### 12.2 — Instalar dependencias de la extensión

```bash
cd vscode-extension
npm install
```

### 12.3 — Compilar

```bash
npm run compile
```

### 12.4 — Probar en modo desarrollo

1. Abre la carpeta `vscode-extension` en VS Code.
2. Presiona `F5` (esto abre una nueva ventana de VS Code de desarrollo).
3. En la nueva ventana, abre la configuración del workspace y configura:

```json
{
  "copilotLocal.apiUrl": "http://localhost:8081",
  "copilotLocal.model": "qwen2.5-coder:7b"
}
```

4. Presiona `Cmd+Shift+P` (macOS) o `Ctrl+Shift+P` (Linux/Windows).
5. Busca `Copilot Local: Open Chat Panel`.

### 12.5 — Comandos disponibles

| Comando | Qué hace |
|---|---|
| `Copilot Local: Open Chat Panel` | Abre el panel de chat |
| `Copilot Local: Send Selection` | Envía el código seleccionado al chat |
| `Copilot Local: Explain Selection` | Explica el código seleccionado |
| `Copilot Local: Add Tests` | Genera tests para la selección |
| `Copilot Local: Refactor Selection` | Sugiere refactorización |
| `Copilot Local: Fix Errors In Selection` | Corrige errores en el código |
| `Copilot Local: Translate Selection` | Traduce código a otro lenguaje |
| `Copilot Local: Debug Error` | Analiza un error/stacktrace |
| `Copilot Local: Reindex Repository` | Re-indexa el repo para RAG |
| `Copilot Local: Commit Message` | Genera mensaje de commit |
| `Copilot Local: Compare Models` | Compara respuestas de 2 modelos |

---

## 13. Diagrama de arquitectura

```
Tu computadora
├── Docker
│   ├── Ollama          (puerto 11434)  ← Motor de IA
│   │   └── WebUI       (puerto 3000)   ← Interfaz web opcional
│   ├── Qdrant          (puerto 6333)   ← Vector DB para RAG
│   ├── MongoDB         (puerto 27017)  ← Persistencia
│   └── Redis           (puerto 6379)   ← Cache (opcional)
│
├── API Gateway (Go)    (puerto 8081)   ← Servidor principal
│   ├── /health                         ← Estado de salud
│   ├── /health/readiness               ← Estado detallado
│   ├── /health/readiness/{name}        ← Check individual
│   ├── /openai/v1/chat/completions     ← Chat compatible OpenAI
│   ├── /api/v2/generate                ← Generación con RAG
│   ├── /api/v2/search                  ← Búsqueda semántica
│   ├── /metrics                        ← Métricas
│   └── /login                          ← Autenticación JWT
│
└── VS Code + Extensión                 ← Tu editor
    └── Copilot Local                   ← Chat, refactor, tests...
```

---

## 14. Resolución de problemas frecuentes

### "connection refused" al conectar a Ollama/Qdrant/Mongo

**Causa:** El servicio Docker no está corriendo.

```bash
# Ver qué contenedores están activos
docker ps

# Levantar el que falta (ejemplo: ollama)
docker compose -f docker-compose.ollama.yml up -d
```

### La API dice "unhealthy" en readiness

```bash
# Ver qué backend falla
curl -s http://localhost:8081/health/readiness | jq '.dependencies | to_entries[] | select(.value.status != "healthy")'
```

### "model not found" al hacer chat

No descargaste el modelo. Vuelve al [Paso 8](#10-paso-8--descargar-un-modelo-en-ollama).

### La API no arranca y dice "no se pudo inicializar runner de migraciones"

MongoDB no está accesible. Verifica:

```bash
curl -s http://localhost:8081/health/readiness/mongo | jq .
# o directamente:
docker compose -f docker-compose.mongo.yml ps
```

### Error "JWT_SECRET" o token inválido

Regenera el secreto y reinicia la API:

```bash
export JWT_SECRET=$(openssl rand -hex 32)
# reiniciar la API
```

### Docker dice "port already in use"

Otro proceso está usando ese puerto:

```bash
# Buscar qué está usando el puerto (ejemplo: 8081)
lsof -i :8081     # macOS/Linux
netstat -ano | findstr 8081   # Windows
```

### Ollama es muy lento o se queda sin memoria

- Usa un modelo más pequeño (`llama3.2:3b` en vez de `13b`).
- Cierra otras aplicaciones para liberar RAM.
- Si tienes GPU, Ollama la usará automáticamente.

### Quiero empezar de cero

```bash
# Parar y eliminar todo (datos incluidos)
docker compose -f docker-compose.ollama.yml down -v
docker compose -f docker-compose.qdrant.yml down -v
docker compose -f docker-compose.mongo.yml down -v
docker stop redis-gateway && docker rm redis-gateway

# Repetir desde el Paso 2
```

---

## 15. Referencia rápida de puertos

| Puerto | Servicio | URL de verificación |
|--------|----------|---------------------|
| `8081` | API Gateway | `curl http://localhost:8081/health` |
| `11434` | Ollama | `curl http://localhost:11434/` |
| `3000` | Open WebUI | Abrir en navegador |
| `6333` | Qdrant | `curl http://localhost:6333/` |
| `27017` | MongoDB | Conexión TCP (solo localhost) |
| `6379` | Redis | `docker exec redis-gateway redis-cli ping` |

---

## Resumen de comandos — Hoja de referencia rápida

```bash
# ===== LEVANTAR TODO =====
docker compose -f docker-compose.ollama.yml up -d
docker compose -f docker-compose.qdrant.yml up -d
docker compose -f docker-compose.mongo.yml up -d
# (opcional) docker run -d --name redis-gateway -p 6379:6379 redis:7-alpine

cd api
set -a && source ../.env && set +a
go run ./cmd/server

# ===== VERIFICAR TODO =====
curl http://localhost:11434/                         # Ollama
curl http://localhost:6333/                           # Qdrant
curl -s http://localhost:8081/health/readiness | jq . # API + todos los backends

# ===== DESCARGAR MODELOS =====
curl http://localhost:11434/api/pull -d '{"name":"qwen2.5-coder:7b"}'
curl http://localhost:11434/api/pull -d '{"name":"nomic-embed-text:latest"}'

# ===== USAR =====
TOKEN=$(curl -s -X POST http://localhost:8081/login -H "Content-Type: application/json" -d '{"username":"admin","password":"admin"}' | jq -r .token)
curl -s http://localhost:8081/openai/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"qwen2.5-coder:7b","messages":[{"role":"user","content":"Hola"}],"stream":false}' | jq .

# ===== PARAR TODO =====
# Ctrl+C en la terminal de la API
docker compose -f docker-compose.ollama.yml down
docker compose -f docker-compose.qdrant.yml down
docker compose -f docker-compose.mongo.yml down
```

---

## 16. Checklist de validación final

Copia y ejecuta este script completo en una terminal. Valida **todos los componentes** de una vez y muestra un resumen con ✅ o ❌ por cada check.

```bash
#!/usr/bin/env bash
# ============================================================
#  Checklist de validación — Ollama SaaS Gateway
#  Ejecutar desde la raíz del proyecto:
#    bash docs/checklist.sh
#  o copiar/pegar en una terminal
# ============================================================

PASS=0
FAIL=0
WARN=0
RESULTS=""

check() {
  local label="$1" cmd="$2" expected="$3" required="${4:-true}"
  result=$(eval "$cmd" 2>/dev/null)
  if echo "$result" | grep -qi "$expected"; then
    RESULTS+="  ✅  $label\n"
    ((PASS++))
  elif [ "$required" = "false" ]; then
    RESULTS+="  ⚠️  $label (opcional — no disponible)\n"
    ((WARN++))
  else
    RESULTS+="  ❌  $label\n"
    ((FAIL++))
  fi
}

echo ""
echo "══════════════════════════════════════════════════════════"
echo "  Validando instalación de Ollama SaaS Gateway..."
echo "══════════════════════════════════════════════════════════"
echo ""

# ---------- 1. Herramientas ----------
echo "→ Verificando herramientas..."
check "Git instalado"              "git --version"              "git version"
check "Go instalado (≥1.24)"       "go version"                 "go1.2"
check "Docker instalado"           "docker --version"           "Docker version"
check "Docker Compose instalado"   "docker compose version"     "Docker Compose"
check "curl instalado"             "curl --version"             "curl"
check "jq instalado"               "jq --version"               "jq"

# ---------- 2. Contenedores Docker ----------
echo "→ Verificando contenedores Docker..."
check "Contenedor Ollama corriendo"  "docker ps --format '{{.Names}}' | grep -c ollama"   "1"
check "Contenedor Qdrant corriendo"  "docker ps --format '{{.Names}}' | grep -c qdrant"   "1"
check "Contenedor MongoDB corriendo" "docker ps --format '{{.Names}}' | grep -c mongo"    "1"
check "Contenedor Redis corriendo"   "docker ps --format '{{.Names}}' | grep -c redis"    "1" "false"

# ---------- 3. Conectividad de backends ----------
echo "→ Verificando conectividad de backends..."
check "Ollama responde (HTTP :11434)"  \
  "curl -sf -o /dev/null -w '%{http_code}' http://localhost:11434/" "200"

check "Qdrant responde (HTTP :6333)"   \
  "curl -sf -o /dev/null -w '%{http_code}' http://localhost:6333/"  "200"

check "MongoDB acepta conexión (TCP :27017)" \
  "docker exec \$(docker ps -qf name=mongo) mongosh --quiet --eval 'db.adminCommand(\"ping\").ok' -u admin -p changeme --authenticationDatabase admin" "1"

check "Redis responde PONG"  \
  "docker exec \$(docker ps -qf name=redis) redis-cli ping" "PONG" "false"

# ---------- 4. API Gateway ----------
echo "→ Verificando API Gateway..."
check "API liveness (:8081/health)"   \
  "curl -sf -o /dev/null -w '%{http_code}' http://localhost:8081/health" "200"

check "API readiness — status healthy" \
  "curl -sf http://localhost:8081/health/readiness | jq -r .status" "healthy"

check "Ollama healthy (readiness)"     \
  "curl -sf http://localhost:8081/health/readiness | jq -r '.dependencies.ollama.status'" "healthy"

check "Qdrant healthy (readiness)"     \
  "curl -sf http://localhost:8081/health/readiness | jq -r '.dependencies.qdrant.status'" "healthy"

check "MongoDB healthy (readiness)"    \
  "curl -sf http://localhost:8081/health/readiness | jq -r '.dependencies.mongo.status'"  "healthy"

# ---------- 5. Modelos Ollama ----------
echo "→ Verificando modelos en Ollama..."
MODELS=$(curl -sf http://localhost:11434/api/tags 2>/dev/null | jq -r '.models[].name' 2>/dev/null)
MODEL_COUNT=$(echo "$MODELS" | grep -c '.' 2>/dev/null || echo 0)

if [ "$MODEL_COUNT" -gt 0 ]; then
  RESULTS+="  ✅  Modelos instalados ($MODEL_COUNT): $(echo $MODELS | tr '\n' ', ')\n"
  ((PASS++))
else
  RESULTS+="  ❌  No hay modelos instalados en Ollama\n"
  ((FAIL++))
fi

check "Modelo de embeddings (nomic-embed-text)" \
  "curl -sf http://localhost:11434/api/tags | jq -r '.models[].name' | grep -c nomic-embed" "1"

# ---------- 6. Autenticación ----------
echo "→ Verificando autenticación..."
TOKEN=$(curl -sf -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' 2>/dev/null | jq -r .token 2>/dev/null)

if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
  RESULTS+="  ✅  Login funciona — token JWT obtenido\n"
  ((PASS++))
else
  RESULTS+="  ❌  Login falló — no se obtuvo token\n"
  ((FAIL++))
fi

# ---------- 7. Chat (inferencia) ----------
echo "→ Verificando inferencia del modelo..."
CHAT_RESPONSE=$(curl -sf -m 30 -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen2.5-coder:7b","messages":[{"role":"user","content":"Responde solo: OK"}],"stream":false}' 2>/dev/null)

if echo "$CHAT_RESPONSE" | jq -e '.choices[0].message.content' >/dev/null 2>&1; then
  RESULTS+="  ✅  Inferencia funciona — el modelo respondió correctamente\n"
  ((PASS++))
else
  RESULTS+="  ❌  Inferencia falló — el modelo no respondió (¿está descargado?)\n"
  ((FAIL++))
fi

# ---------- 8. Endpoints clave ----------
echo "→ Verificando endpoints adicionales..."
check "Endpoint /api/models accesible" \
  "curl -sf -o /dev/null -w '%{http_code}' http://localhost:8081/api/models" "200"

check "Endpoint /metrics accesible" \
  "curl -sf -o /dev/null -w '%{http_code}' http://localhost:8081/metrics" "200"

# ---------- 9. Archivo .env ----------
echo "→ Verificando configuración..."
if [ -f ".env" ]; then
  RESULTS+="  ✅  Archivo .env existe\n"
  ((PASS++))

  if grep -q "JWT_SECRET" .env && ! grep -q "PEGA-AQUI" .env; then
    RESULTS+="  ✅  JWT_SECRET configurado\n"
    ((PASS++))
  else
    RESULTS+="  ❌  JWT_SECRET no configurado (ver Paso 6)\n"
    ((FAIL++))
  fi
else
  RESULTS+="  ❌  Archivo .env no encontrado (ver Paso 6)\n"
  ((FAIL++))
fi

# ---------- RESUMEN ----------
TOTAL=$((PASS + FAIL + WARN))
echo ""
echo "══════════════════════════════════════════════════════════"
echo "  RESULTADO DE LA VALIDACIÓN"
echo "══════════════════════════════════════════════════════════"
echo ""
printf "$RESULTS"
echo ""
echo "──────────────────────────────────────────────────────────"
echo "  Total: $TOTAL checks  |  ✅ $PASS ok  |  ❌ $FAIL fallos  |  ⚠️  $WARN opcionales"
echo "──────────────────────────────────────────────────────────"

if [ "$FAIL" -eq 0 ]; then
  echo ""
  echo "  🎉  ¡Todo correcto! La instalación está completa."
  echo ""
else
  echo ""
  echo "  ⚡  Hay $FAIL checks fallidos. Revisa los pasos correspondientes."
  echo ""
fi
```

### Cómo usarlo

**Opción A — Menú interactivo (recomendado):**

```bash
# Desde la raíz del proyecto
bash docs/checklist.sh
```

Verás un menú donde puedes elegir validar todo (opción 0) o una categoría individual (1-9).

**Opción B — Línea de comandos (sin menú):**

```bash
# Ejecutar todos los checks de una vez
bash docs/checklist.sh --all

# Ejecutar solo una categoría (ejemplo: backends)
bash docs/checklist.sh --category 3

# Ver categorías disponibles
bash docs/checklist.sh --list
```

**Opción B — Validación rápida sin script (5 comandos):**

Si prefieres validar manualmente, ejecuta estos 5 comandos uno por uno:

```bash
# 1. ¿Están todos los contenedores corriendo?
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -E "ollama|qdrant|mongo|redis"

# 2. ¿Los backends responden?
echo "Ollama:  $(curl -sf http://localhost:11434/ || echo 'NO RESPONDE')"
echo "Qdrant:  $(curl -sf http://localhost:6333/ | jq -r .title 2>/dev/null || echo 'NO RESPONDE')"

# 3. ¿La API y sus dependencias están sanas?
curl -sf http://localhost:8081/health/readiness | jq '{status, backends: [.dependencies | to_entries[] | {(.key): .value.status}]}'

# 4. ¿Hay modelos descargados?
curl -sf http://localhost:11434/api/tags | jq '[.models[].name]'

# 5. ¿La inferencia funciona?
curl -sf -m 30 http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen2.5-coder:7b","messages":[{"role":"user","content":"Responde OK"}],"stream":false}' \
  | jq -r '.choices[0].message.content' && echo "✅ Inferencia OK" || echo "❌ Inferencia FALLÓ"
```

### Resultado esperado (todo bien)

```
══════════════════════════════════════════════════════════
  RESULTADO DE LA VALIDACIÓN
══════════════════════════════════════════════════════════

  ✅  Git instalado
  ✅  Go instalado (≥1.24)
  ✅  Docker instalado
  ✅  Docker Compose instalado
  ✅  curl instalado
  ✅  jq instalado
  ✅  Contenedor Ollama corriendo
  ✅  Contenedor Qdrant corriendo
  ✅  Contenedor MongoDB corriendo
  ⚠️  Contenedor Redis corriendo (opcional — no disponible)
  ✅  Ollama responde (HTTP :11434)
  ✅  Qdrant responde (HTTP :6333)
  ✅  MongoDB acepta conexión (TCP :27017)
  ⚠️  Redis responde PONG (opcional — no disponible)
  ✅  API liveness (:8081/health)
  ✅  API readiness — status healthy
  ✅  Ollama healthy (readiness)
  ✅  Qdrant healthy (readiness)
  ✅  MongoDB healthy (readiness)
  ✅  Modelos instalados (2): qwen2.5-coder:7b, nomic-embed-text
  ✅  Modelo de embeddings (nomic-embed-text)
  ✅  Login funciona — token JWT obtenido
  ✅  Inferencia funciona — el modelo respondió correctamente
  ✅  Endpoint /api/models accesible
  ✅  Endpoint /metrics accesible
  ✅  Archivo .env existe
  ✅  JWT_SECRET configurado

──────────────────────────────────────────────────────────
  Total: 28 checks  |  ✅ 26 ok  |  ❌ 0 fallos  |  ⚠️  2 opcionales
──────────────────────────────────────────────────────────

  🎉  ¡Todo correcto! La instalación está completa.
```
