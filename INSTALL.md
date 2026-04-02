# Instalación y uso — Ollama SaaS Gateway

Este documento describe cómo instalar, configurar y probar localmente el proyecto "Ollama SaaS Gateway".

Contenido:
- Requisitos previos
- Configuración rápida (Qdrant, Ollama)
- Variables de entorno y `.env` ejemplo
- Compilar y ejecutar (modo desarrollo y con Docker)
- Indexar repositorio y probar endpoints (ejemplos)
- Integración con VS Code (extensión de desarrollo)
- Troubleshooting rápido

---

## 1. Requisitos previos

Asegúrate de tener instalados:

- Git
- Go (1.24 o superior)
- Docker y docker-compose (si vas a usar contenedores)
- Node.js (v16+) y VS Code (para la extensión dev)

Comprueba versiones:

```bash
git --version
go version
docker --version
docker-compose --version
node --version
code --version
```

---

## 2. Clonar el repositorio

```bash
git clone <tu-repo-url>
cd ollama_saas_project
```

---

## 3. Configurar dependencias principales

3.1 Qdrant (vector DB)

Opción A — Con `docker run` (rápido):

```bash
docker run -d --name qdrant -p 6333:6333 -v qdrant_data:/qdrant/storage qdrant/qdrant:latest
```

Opción B — Con `docker-compose` (recomendado si usas `docker-compose.yml` del repo):

```bash
docker-compose up -d qdrant
```

Verifica que Qdrant esté listo:

```bash
curl -sS http://localhost:6333/ | head
```

3.2 Ollama (modelo local)

Instalar Ollama depende de tu plataforma. Si usas la imagen oficial o instalación local, asegúrate de que el servicio responda en la URL configurada (por defecto `http://localhost:11434`). Si no tienes Ollama local, puedes configurar `OLLAMA_URL` apuntando a un servidor Ollama accesible.

Ejemplo con `docker` (si existe imagen pública):

```bash
# Revisa la documentación de Ollama para la instalación específica de la versión que uses.
```

---

## 4. Variables de entorno (archivo `.env`)

Crea un archivo `.env` en la raíz del proyecto o exporta variables en tu entorno. Ejemplo mínimo (`.env`):

```env
PORT=8081
REPO_ROOT=$(pwd)
OLLAMA_URL=http://localhost:11434
QDRANT_URL=http://localhost:6333
VECTOR_STORE_PATH=.vector_store.json
INDEXER_STATE_PATH=.indexer_state.json
JWT_SECRET=tu-secreto-jwt-en-hex
RATE_LIMIT_RPM=120
EMBEDDING_CACHE_TTL_SECONDS=3600
EMBEDDING_CACHE_MAX_ENTRIES=1000
VECTOR_STORE_PREFER_LOCAL=true
```

Nota: `REPO_ROOT` debe apuntar al directorio que deseas indexar (por ejemplo el mismo repo). Usa `openssl rand -hex 32` para generar un `JWT_SECRET` seguro.

---

## 5. Compilar y ejecutar (modo desarrollo)

1) Instala dependencias de Go y formatea:

```bash
cd api
go mod tidy
go fmt ./...
```

2) Compilar los binarios:

```bash
go build -o bin/server ./cmd/server
go build -o bin/copilot-cli ./cmd/copilot-cli
```

3) Ejecutar el servidor (usa el `.env` antes cargado):

```bash
# En macOS/Linux
export $(cat ../.env | xargs)
./bin/server

# O usar go run (modo rápido de desarrollo)
go run ./cmd/server
```

Si usas `docker-compose` en la raíz, puedes levantar todos los servicios (ollama/qdrant/api) con:

```bash
docker-compose up -d
```

---

## 6. Indexar el repositorio y controlar el indexer

El indexer tiene endpoints operator en `/internal/index/`.

Ejecutar reindex completo:

```bash
curl -X POST http://localhost:8081/internal/index/reindex -H "Authorization: Bearer <token-si-aplica>"
```

Iniciar watcher (indexación incremental):

```bash
curl -X POST http://localhost:8081/internal/index/start
```

Detener watcher:

```bash
curl -X POST http://localhost:8081/internal/index/stop
```

Resetear estado del indexer (elimina .indexer_state.json):

```bash
curl -X POST http://localhost:8081/internal/index/reset
```

---

## 7. Probar endpoints (ejemplos)

7.1 Embeddings (OpenAI-compatible)

```bash
curl -sS -X POST http://localhost:8081/openai/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{"input":"Hola mundo"}'
```

7.2 Chat completions (streaming SSE)

```bash
curl -N -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-rag","messages":[{"role":"user","content":"Explícame qué hace el patrón repository en 2 líneas."}],"stream":true}'
```

7.3 Usar `copilot-cli` (fallback CLI):

```bash
# ejemplo interactivo
./bin/copilot-cli --prompt "Resume este repo en 2 frases" --model local-rag
```

---

## 8. Extensión VS Code (modo desarrollo)

1. Abre VS Code en la carpeta `vscode-extension`:

```bash
code vscode-extension
```

2. Presiona `F5` para lanzar un Extension Host (nueva ventana).

3. En la ventana de desarrollo, abre el Command Palette (`Cmd/Ctrl+Shift+P`) y ejecuta `Copilot Local: Open Chat Panel`.

4. Configura settings (si es necesario): `copilotLocal.apiUrl`, `copilotLocal.model`, `copilotLocal.jwtToken`.

Nota: la extensión en modo dev usa `streamHTTP` para conectar al endpoint `/openai/v1/chat/completions` y mostrará respuesta parcial en tiempo real. Si el gateway no está disponible, la extensión intentará usar `copilot-cli` como fallback.

---

## 9. Ejemplo completo: indexar + preguntar al proyecto

1. Asegúrate de haber levantado Qdrant y el servidor.
2. Inicia watcher o reindex completo (ver sección 6).
3. Ejecuta una búsqueda RAG mediante el endpoint de chat o `copilot-cli`:

```bash
curl -N -X POST http://localhost:8081/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-rag","messages":[{"role":"user","content":"¿Cómo está organizada la carpeta internal/services?"}],"stream":true}'
```

Deberías ver la respuesta en streaming con contexto extraído del repositorio.

---

## 10. Troubleshooting rápido

- Si `Qdrant` no responde: revisa `docker ps` y logs con `docker logs qdrant`.
- Si `Ollama` falla: verifica `OLLAMA_URL` y que el servicio esté corriendo.
- Si ves `401 Unauthorized` en endpoints protegidos: asegúrate de tener `JWT_SECRET` y solicitar token via `/login`.
- Problemas con indexer: borra `.indexer_state.json` y reindexa.
- Logs del servidor: revisa la salida del binario `./bin/server` o `docker-compose logs api`.

---

Si quieres, puedo añadir scripts de `make` (por ejemplo `make dev`, `make up`) o un archivo `.env.example` en el repo. ¿Deseas que lo cree ahora?
