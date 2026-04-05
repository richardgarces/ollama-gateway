# Instalacion y Operacion Local - Ollama SaaS Gateway

Esta guia deja el gateway funcionando de forma reproducible en desarrollo, con dos topologias:

1. Stack local desacoplado por servicio: API, Ollama, Qdrant y Mongo con compose separados.
2. Topologia distribuida: API en una maquina y Ollama/Qdrant/Mongo en otras.

## 1. Requisitos

- Git
- Go 1.24+
- Docker + Docker Compose plugin (`docker compose`)
- Node.js 18+ y VS Code (si usas extension local)

Comprobacion rapida:

```bash
git --version
go version
docker --version
docker compose version
node --version
code --version
```

## 2. Clonar repositorio

```bash
git clone <tu-repo-url>
cd ollama_saas_project
```

## 3. Variables de entorno recomendadas

El servidor carga variables desde el entorno del proceso. Ejemplo minimo:

```bash
export PORT=8081
export REPO_ROOT="$PWD"
export OLLAMA_URL="http://localhost:11434"
export QDRANT_URL="http://localhost:6333"
export MONGO_URI="mongodb://admin:changeme@localhost:27017"
export JWT_SECRET="$(openssl rand -hex 32)"
```

Variables utiles de tuning:

```bash
export RATE_LIMIT_RPM=120
export RATE_LIMIT_USER_RPM=120
export HTTP_TIMEOUT_SECONDS=30
export HTTP_MAX_RETRIES=3
export EMBEDDING_CACHE_TTL_SECONDS=3600
export EMBEDDING_CACHE_MAX_ENTRIES=1000
export RAG_CACHE_TTL_SECONDS=1800
export RAG_CACHE_MAX_ENTRIES=500
```

Referencia completa en [api/docs/ENV_VARS.md](api/docs/ENV_VARS.md).

## 4. Levantar infraestructura con Docker (recomendado)

### 4.1 Levantar Ollama y WebUI (compose separado)

```bash
docker compose -f docker-compose.ollama.yml up -d
```

Verificacion:

```bash
curl -fsS http://localhost:11434/ >/dev/null && echo "ollama ok"
curl -fsS http://localhost:3000/ >/dev/null && echo "webui ok"
```

### 4.2 Levantar Qdrant

```bash
docker compose -f docker-compose.qdrant.yml up -d
```

### 4.3 Levantar Mongo

```bash
docker compose -f docker-compose.mongo.yml up -d
```

### 4.4 Levantar API

```bash
docker compose -f docker-compose.api.yml up -d
```

Verificacion:

```bash
curl -fsS http://localhost:8081/health | cat
curl -fsS http://localhost:6333/ | cat
```

Nota: en [docker-compose.yml](docker-compose.yml) la API usa por defecto `OLLAMA_URL=http://host.docker.internal:11434`. Si Ollama corre remoto, exporta `OLLAMA_URL` antes de levantar la API.

Compose separados disponibles:

- [docker-compose.api.yml](docker-compose.api.yml)
- [docker-compose.ollama.yml](docker-compose.ollama.yml)
- [docker-compose.qdrant.yml](docker-compose.qdrant.yml)
- [docker-compose.mongo.yml](docker-compose.mongo.yml)

## 5. Ejecutar API sin Docker (modo desarrollo)

```bash
cd api
go mod tidy
go build -o bin/server ./cmd/server
go build -o bin/copilot-cli ./cmd/copilot-cli
./bin/server
```

Alternativa rapida:

```bash
cd api
go run ./cmd/server
```

## 6. Autenticacion y smoke test

Obtener JWT:

```bash
TOKEN=$(curl -sS -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)
echo "$TOKEN"
```

Health y metricas:

```bash
curl -sS http://localhost:8081/health | cat
curl -sS http://localhost:8081/metrics | cat
curl -sS http://localhost:8081/metrics/prometheus | head
```

Chat OpenAI-compatible (stream SSE):

```bash
curl -N -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-rag","messages":[{"role":"user","content":"Resume este proyecto en 3 lineas"}],"stream":true}'
```

## 7. Indexer y contexto RAG

Reindex completo:

```bash
curl -sS -X POST http://localhost:8081/internal/index/reindex | cat
```

Iniciar watcher incremental:

```bash
curl -sS -X POST http://localhost:8081/internal/index/start | cat
```

Estado indexer:

```bash
curl -sS http://localhost:8081/internal/index/status | cat
```

Reset estado indexer:

```bash
curl -sS -X POST http://localhost:8081/internal/index/reset | cat
```

## 8. Endpoints internos de operacion

Solo localhost:

- `/dashboard`
- `/api-docs`
- `/internal/dashboard/status`
- `/internal/logs/stream` (SSE)
- `/internal/index/*`

## 9. Extension VS Code (desarrollo)

1. Abrir [vscode-extension](vscode-extension).
2. Presionar `F5` para ejecutar Extension Host.
3. Configurar settings:
   - `copilotLocal.apiUrl`
   - `copilotLocal.model`
   - `copilotLocal.jwtToken`
4. Probar comandos desde `Cmd+Shift+P`:
   - `Copilot Local: Open Chat Panel`
   - `Copilot Local: Join Shared Session`
   - `Copilot Local: Suggest Commit Message`
5. Atajo por defecto para abrir chat: `Cmd+Alt+K` (macOS).

## 10. Comandos Make utiles

Desde la raiz del repo:

```bash
make build
make test
make run
make docker-up
make docker-down
```

## 11. Troubleshooting

- 401/403 en rutas protegidas: generar token en `/login` y pasar header `Authorization: Bearer <token>`.
- API no conecta a Ollama: revisar `OLLAMA_URL` y acceso de red al puerto 11434.
- Qdrant sin resultados: reindexar y validar `/internal/index/status`.
- Mongo auth failed: revisar `MONGO_URI`, usuario y password en compose.
- Extension sin respuesta: revisar `copilotLocal.apiUrl`, `copilotLocal.jwtToken` y logs del servidor.

## 12. Despliegue distribuido (A/B)

- Maquina A: Ollama (puerto 11434).
- Maquina B: Qdrant + Mongo.
- Maquina API: gateway Go apuntando a `OLLAMA_URL`, `QDRANT_URL` y `MONGO_URI` remotos.

Consulta ejemplo para maquina B en [api/docs/docker-compose-machine-b.yml](api/docs/docker-compose-machine-b.yml).
