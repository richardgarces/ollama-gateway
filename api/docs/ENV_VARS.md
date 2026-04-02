# Variables de entorno

Este documento resume las variables soportadas actualmente por [api/internal/config/config.go](../internal/config/config.go).

## Core

- `PORT` (default: `8081`): puerto HTTP de la API.
- `JWT_SECRET` (sin default estable): secreto de firma JWT.
- `OLLAMA_URL` (default: `http://ollama:11434`): endpoint de Ollama.
- `QDRANT_URL` (default: `http://qdrant:6333`): endpoint de Qdrant.
- `MONGO_URI` (default: `mongodb://localhost:27017`): conexión Mongo.

## Repositorio y rutas

- `REPO_ROOT` (default: `.`): repo principal.
- `REPO_ROOTS` (default: usa `REPO_ROOT`): lista CSV de repos para modo multi-repo.
- `VECTOR_STORE_PATH` (default: `<REPO_ROOT>/.vector_store.json`): store vectorial local.
- `VECTOR_STORE_PREFER_LOCAL` (default: `false`): preferencia por store local en lecturas/escrituras.
- `INDEXER_STATE_PATH` (default: `<REPO_ROOT>/.indexer_state.json`): estado incremental indexer.
- `AGENT_TOOLS_DIR` (default: `<REPO_ROOT>/agent-tools`): directorio de tools de agentes.

## Rate limit

- `RATE_LIMIT_RPM` (default: `60`): límite global por minuto.
- `RATE_LIMIT_USER_RPM` (default: `60`): límite por usuario.
- `RATE_LIMIT_ENDPOINTS` (default: `{}`): JSON con límites por endpoint.

Ejemplo:

```bash
export RATE_LIMIT_ENDPOINTS='{"POST /openai/v1/chat/completions":30,"POST /api/generate":20}'
```

## HTTP resiliencia

- `HTTP_TIMEOUT_SECONDS` (default: `30`)
- `HTTP_MAX_RETRIES` (default: `3`)

## Cache

- `CACHE_BACKEND` (default: `memory`)
- `REDIS_URL` (default: `redis://localhost:6379/0`)
- `EMBEDDING_CACHE_TTL_SECONDS` (default: `3600`)
- `EMBEDDING_CACHE_MAX_ENTRIES` (default: `1000`)
- `RAG_CACHE_TTL_SECONDS` (default: `1800`)
- `RAG_CACHE_MAX_ENTRIES` (default: `500`)

## Logging

- `LOG_LEVEL` (default: `info`)
- `LOG_FORMAT` (default: `json`)

## Integraciones remotas opcionales

- `REMOTE_API_URL` (default: `""`)
- `REMOTE_API_KEY` (default: `""`)

## Recomendaciones de operación

- En producción, inyecta secretos desde gestor de secretos (no desde archivos en repo).
- Si Ollama está en otra máquina, restringe acceso al puerto 11434 por red privada.
- Mantén Qdrant/Mongo en red interna y publica solo la API.
- Si usas rutas locales (`/internal/*`, `/api-docs`), consúmelas desde localhost.
