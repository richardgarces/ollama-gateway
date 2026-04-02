# Variables de entorno

Variables requeridas:
- `PORT` — puerto donde corre la API (por defecto `8081`).
- `OLLAMA_URL` — URL de Ollama, ejemplo `http://ollama-host:11434`.
- `QDRANT_URL` — URL de Qdrant (ej. `http://qdrant-host:6333`).
- `JWT_SECRET` — secreto para firmar JWT (hex 32 bytes recomendado).

Configuración opcional / tuning:
- `RATE_LIMIT_RPM` — peticiones por minuto por IP (default 60).
- `EMBEDDING_CACHE_TTL_SECONDS` — TTL en segundos para cache de embeddings (default 3600).
- `EMBEDDING_CACHE_MAX_ENTRIES` — límite máximo de entradas en cache (default 1000).
- `VECTOR_STORE_PATH` — ruta del store vectorial local persistente (default `.vector_store.json` dentro de `REPO_ROOT`).
- `VECTOR_STORE_PREFER_LOCAL` — si vale `true`, usa solo el store local para búsquedas y escrituras vectoriales.
- `INDEXER_STATE_PATH` — ruta del archivo de estado incremental del indexador (default `.indexer_state.json` dentro de `REPO_ROOT`).

Recomendaciones:
- Guardar `JWT_SECRET` en un secreto gestionado (Vault, AWS Secrets Manager) en producción.
- No exponer `OLLAMA_URL` directamente en redes públicas; usar VPN o reglas de firewall.
- Si se usa persistencia local, mantener `VECTOR_STORE_PATH` dentro de `REPO_ROOT` para evitar escritura fuera del directorio permitido.
