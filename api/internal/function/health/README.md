# health

Modulo vertical en migracion.

Subcarpetas:
- domain
- repository
- service
- transport

## Configuracion de checks

El endpoint `GET /health/readiness` usa un registro de checks configurable y paralelo.

- Checks por defecto (si hay URL configurada): `ollama` (http), `qdrant` (http), `mongo` (tcp), `redis` (tcp).
- Checks extra configurables con `HEALTH_EXTRA_CHECKS_JSON`.
- Timeout por check con `HEALTH_CHECK_TIMEOUT_MS` y override opcional por check (`timeout_ms`).

Ejemplo:

```json
[
	{
		"name": "minio",
		"type": "http",
		"target": "http://minio:9000",
		"path": "/minio/health/live",
		"required": false,
		"timeout_ms": 1500
	}
]
```
