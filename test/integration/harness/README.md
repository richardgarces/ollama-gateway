# Integration Harness

Harness reproducible para pruebas de integracion con servicios efimeros via Docker Compose.

## Servicios

- MongoDB en `127.0.0.1:${IT_MONGO_PORT:-27018}`
- Qdrant en `127.0.0.1:${IT_QDRANT_PORT:-6334}`
- Ollama en `127.0.0.1:${IT_OLLAMA_PORT:-11435}`

## Uso rapido

Desde la raiz del repositorio:

```bash
make integration-test
```

Esto realiza automaticamente:

1. Levantar servicios con `docker compose`
2. Ejecutar seed minimo en Mongo/Qdrant (y health check de Ollama)
3. Correr tests de integracion
4. Reportar resultados en `test/integration/harness/last-report.txt`
5. Limpiar recursos (`docker compose down -v --remove-orphans`)

## Variables utiles

- `IT_TEST_CMD`: comando de test a ejecutar
- `IT_KEEP_UP=1`: no hace cleanup para debug
- `IT_REPORT_FILE`: ruta custom para reporte
- `IT_OLLAMA_SEED_MODEL`: modelo opcional a hacer pull en seed
