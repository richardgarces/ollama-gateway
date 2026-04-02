# Mejoras Técnicas

> Cada sección incluye un **prompt listo para Copilot** orientado a mejoras de arquitectura, rendimiento e infraestructura del proyecto.

---

## 1. Migrar a Interfaces Explícitas para Testabilidad

Extraer interfaces formales de todos los servicios para facilitar mocking en tests y desacoplar capas.

**Estado actual:** Los handlers usan interfaces anónimas inline (`interface{ Method(...) }`) en sus constructores.

```
Prompt Copilot:
Refactoriza para usar interfaces explícitas en internal/domain/interfaces.go:
1. Crea el archivo internal/domain/interfaces.go con interfaces nombradas:
   - OllamaClient{Generate, StreamGenerate, GetEmbedding}
   - VectorStore{UpsertPoint, Search}
   - RAGEngine{GenerateWithContext, StreamGenerateWithContext}
   - Indexer{IndexRepo, StartWatcher, StopWatcher, ClearState}
   - AgentRunner{Run}
2. Haz que cada Service en internal/services/ implemente su interfaz correspondiente
   (no cambies las firmas, solo verifica con var _ Interface = (*Service)(nil)).
3. Actualiza todos los handlers en internal/handlers/ para recibir las interfaces
   nombradas en vez de interfaces anónimas.
4. Actualiza internal/server/server.go para inyectar usando las interfaces.
No modifiques la lógica de negocio, solo los tipos de los parámetros.
```

---

## 2. Connection Pooling y Retry para Clientes HTTP

Configurar timeouts, connection pools y retry con backoff para las llamadas a Ollama y Qdrant.

**Estado actual:** Se usa `http.DefaultClient` o clientes sin configurar explícitamente.

```
Prompt Copilot:
Mejora la resiliencia de los clientes HTTP en internal/services/:
1. Crea pkg/httpclient/client.go con una función NewResilientClient(opts Options)
   *http.Client que configure:
   - Timeout global configurable (default 30s).
   - Transport con MaxIdleConns=100, MaxIdleConnsPerHost=10, IdleConnTimeout=90s.
   - TLS con TLSHandshakeTimeout=10s.
2. Crea pkg/httpclient/retry.go con un RoundTripper decorador que implemente
   retry con exponential backoff (max 3 intentos, solo para 5xx y connection errors).
   No reintentes POST si el body ya fue consumido.
3. Aplica NewResilientClient en:
   - internal/services/ollama.go (constructor NewOllamaService)
   - internal/services/qdrant.go (constructor NewQdrantService)
4. Agrega config vars HTTP_TIMEOUT_SECONDS y HTTP_MAX_RETRIES a config.go.
Usa slog o log estándar para loguear reintentos con el request_id del context.
```

---

## 3. Structured Logging con slog

Reemplazar `log.Printf` por structured logging usando `log/slog` (stdlib Go 1.21+).

**Estado actual:** Logs dispersos con `log.Printf` y `fmt.Printf` sin estructura.

```
Prompt Copilot:
Migra todo el logging del proyecto a log/slog:
1. Crea internal/config/logger.go con una función SetupLogger(level string)
   *slog.Logger que configure JSON output para producción y text para desarrollo.
   Añade LOG_LEVEL (default: "info") y LOG_FORMAT (default: "json") a config.go.
2. Inyecta *slog.Logger en todos los Services vía constructor (campo logger).
3. Reemplaza todos los log.Printf, log.Println, fmt.Printf de los archivos:
   - internal/services/*.go
   - internal/middleware/*.go
   - internal/server/server.go
   - cmd/server/main.go
   por llamadas slog con campos estructurados: slog.String("request_id", id),
   slog.String("service", "ollama"), slog.Duration("latency", d), etc.
4. En internal/middleware/middleware.go, loguea cada request con method, path,
   status, latency y request_id usando slog.Info.
No modifiques la lógica, solo reemplaza las llamadas de log.
```

---

## 4. Graceful Shutdown

Implementar cierre ordenado del servidor para drenar conexiones activas y detener goroutines.

**Estado actual:** `cmd/server/main.go` usa `http.ListenAndServe` sin manejo de señales.

```
Prompt Copilot:
Implementa graceful shutdown en cmd/server/main.go:
1. Usa signal.NotifyContext con SIGINT y SIGTERM para capturar la señal de cierre.
2. Llama a server.Shutdown(ctx) con un timeout de 15 segundos para drenar
   conexiones HTTP activas.
3. Antes del shutdown HTTP, detén en orden:
   a. IndexerService.StopWatcher() — detener file watcher.
   b. Flush de métricas pendientes si aplica.
   c. Log de "shutting down..." con slog.
4. Después del shutdown, loguea "server stopped" y retorna con exit code 0.
5. Si el timeout expira, fuerza el cierre con log.Fatal.
Usa context.WithTimeout para controlar el deadline del cierre.
```

---

## 5. Dockerfile Multi-stage Optimizado

Reducir el tamaño de la imagen Docker usando multi-stage build y scratch/distroless.

**Estado actual:** `api/Dockerfile` existe pero puede no estar optimizado.

```
Prompt Copilot:
Optimiza api/Dockerfile con multi-stage build:
1. Stage "builder": usa golang:1.24-alpine, copia go.mod/go.sum primero
   (para cache de dependencias), luego copia el código y compila con
   CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server.
2. Stage "final": usa gcr.io/distroless/static-debian12 (o alpine:3.19 si
   necesitas shell para debugging).
   Copia solo el binario /server desde el builder.
   Expone puerto 8081, ejecuta con USER nonroot:nonroot.
3. Añade labels OCI estándar: org.opencontainers.image.source, version, description.
4. Añade un .dockerignore en api/ que excluya: .old*, bin/, *.test, .git, docs/.
5. Actualiza docker-compose.yml para usar el nuevo Dockerfile con build context ./api.
El resultado debe producir una imagen < 20MB.
```

---

## 6. Tests de Integración con Testcontainers

Añadir tests de integración que levanten Qdrant y MongoDB en contenedores temporales.

```
Prompt Copilot:
Implementa tests de integración usando testcontainers-go:
1. Agrega la dependencia github.com/testcontainers/testcontainers-go.
2. Crea api/integration_test.go (build tag //go:build integration) con:
   - setupQdrant(t) que levante qdrant/qdrant:latest y devuelva la URL.
   - setupMongo(t) que levante mongo:7 y devuelva la connection string.
3. Crea test TestFullRAGPipeline que:
   a. Levante Qdrant vía testcontainer.
   b. Instancie QdrantService, IndexerService y RAGService con la URL del container.
   c. Indexe un directorio de fixtures (crea api/testdata/sample.go con código Go).
   d. Haga una búsqueda y valide que retorna resultados relevantes.
4. Crea test TestOpenAIEndpointIntegration que:
   a. Levante el server completo con httptest.NewServer.
   b. Envíe un POST a /openai/v1/chat/completions.
   c. Valide el formato de la respuesta.
5. Añade un target en Makefile: test-integration que ejecute
   go test -tags=integration -v ./...
Los tests deben usar t.Parallel() donde sea posible.
```

---

## 7. Migrar de gorilla/mux a net/http (Go 1.22+)

Aprovechar el nuevo router de la stdlib de Go 1.22+ para eliminar la dependencia de gorilla/mux.

```
Prompt Copilot:
Migra el enrutamiento de gorilla/mux a net/http estándar (Go 1.22+):
1. En internal/server/server.go, reemplaza mux.NewRouter() por http.NewServeMux().
2. Convierte las rutas de r.HandleFunc("/path", handler).Methods("POST") a
   mux.HandleFunc("POST /path", handler) (nuevo pattern matching de Go 1.22).
3. Para path params (si hay alguno como {id}), usa r.PathValue("id").
4. Reemplaza r.Use(middleware) por el patrón de wrapping manual:
   handler = middleware(handler) aplicado en cadena.
5. Elimina gorilla/mux de go.mod con go mod tidy.
6. Actualiza la documentación en docs/ARCHITECTURE.md para reflejar
   que se usa el router estándar de Go.
7. Ejecuta go test ./... para verificar que no hay regresiones.
```

---

## 8. Caché Distribuido con Redis

Reemplazar los caches in-memory (embedding cache, response cache) con Redis para soportar múltiples instancias del gateway.

```
Prompt Copilot:
Implementa caché distribuido con Redis:
1. Agrega la dependencia github.com/redis/go-redis/v9.
2. Crea pkg/cache/cache.go con una interfaz Cache{Get(key)([]byte,error),
   Set(key string,val []byte,ttl time.Duration)error, Delete(key)error}.
3. Crea pkg/cache/memory.go implementando Cache con sync.Map + TTL (migra
   la lógica actual de EmbeddingCache aquí).
4. Crea pkg/cache/redis.go implementando Cache con go-redis.
5. Añade CACHE_BACKEND ("memory"|"redis") y REDIS_URL a config.go.
6. Modifica internal/services/ollama.go para usar la interfaz Cache en vez
   del EmbeddingCache propio.
7. En cmd/server/main.go, instancia el backend según CACHE_BACKEND y pásalo
   a OllamaService/RAGService.
Usa json.Marshal/Unmarshal para serializar embeddings a []byte.
```

---

## 9. Rate Limiting Granular por Usuario/Endpoint

Extender el rate limiter para aplicar límites diferentes por usuario autenticado y por endpoint.

**Estado actual:** Rate limit global basado en IP con un solo valor `RATE_LIMIT_RPM`.

```
Prompt Copilot:
Extiende internal/middleware/rate_limit.go y internal/observability/rate_limiter.go:
1. Modifica el rate limiter para soportar múltiples tiers:
   - Global: RATE_LIMIT_RPM (actual, mantener como default).
   - Por usuario: RATE_LIMIT_USER_RPM, keyed por userID del JWT claims.
   - Por endpoint: mapa configurable RATE_LIMIT_ENDPOINTS como JSON env var
     (ej: {"POST /openai/v1/chat/completions": 30, "POST /api/generate": 20}).
2. El middleware debe extraer el userID del context (puesto por auth middleware)
   y aplicar el límite más restrictivo (min de global, user, endpoint).
3. Cuando se exceda el límite, devuelve 429 con headers estándar:
   Retry-After, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset.
4. Registra los rate limits en las métricas de Prometheus con labels
   {user_id, endpoint, action="allowed|rejected"}.
```

---

## 10. Pipeline CI/CD con GitHub Actions

Configurar CI automatizado para lint, test, build y release.

```
Prompt Copilot:
Crea .github/workflows/ci.yml con GitHub Actions:
1. Trigger on push (main) y pull_request.
2. Jobs:
   a. lint: usa golangci-lint con la config por defecto.
   b. test: ejecuta go test -race -coverprofile=coverage.out ./...
      y sube el reporte de coverage como artifact.
   c. build: compila con go build -o server ./cmd/server y go build -o copilot-cli ./cmd/copilot-cli.
      Usa matrix para GOOS=[linux,darwin] y GOARCH=[amd64,arm64].
   d. docker: construye la imagen Docker y la pushea a ghcr.io (solo en push a main).
3. Cachea go modules con actions/cache.
4. Usa go 1.24 como versión de Go.
5. Añade un badge de CI al README.md principal.
El workflow debe fallar si hay errores de lint o tests fallidos.
```

---

## 11. Compresión y Paginación de la API

Añadir soporte para gzip y paginación en endpoints que devuelvan listas.

```
Prompt Copilot:
Añade compresión gzip y paginación a la API:
1. Crea internal/middleware/compress.go con un middleware que aplique
   gzip compression cuando el cliente envíe Accept-Encoding: gzip.
   Usa compress/gzip de la stdlib. Ignora respuestas SSE (text/event-stream).
2. Aplica el middleware globalmente en internal/server/server.go.
3. Crea pkg/httputil/pagination.go con helpers:
   - ParsePagination(r *http.Request) (page int, pageSize int) que lea
     query params ?page=1&page_size=20 con defaults.
   - WritePaginatedJSON(w, items, total, page, pageSize) que escriba la
     respuesta con formato {data:[], total:N, page:N, page_size:N, pages:N}.
4. Aplica paginación en los endpoints que devuelvan colecciones
   (por ahora, preparar el helper para uso futuro).
```

---

## 12. Health Check Detallado con Dependencias

Extender `/health` para reportar el estado de cada dependencia (Ollama, Qdrant, MongoDB).

```
Prompt Copilot:
Mejora internal/handlers/health.go:
1. Renombra el health check actual a /health/liveness (solo devuelve 200 OK).
2. Crea /health/readiness que verifique cada dependencia:
   - Ollama: GET {OLLAMA_URL}/ con timeout de 2s.
   - Qdrant: GET {QDRANT_URL}/ con timeout de 2s.
   - (futuro) MongoDB: ping con timeout de 2s.
3. Devuelve JSON: {status:"healthy"|"degraded"|"unhealthy",
   dependencies:{ollama:{status,latency_ms}, qdrant:{status,latency_ms}}}.
   Status es "healthy" si todas OK, "degraded" si algunas fallan, "unhealthy" si todas fallan.
4. /health sigue funcionando como alias de /health/liveness para retrocompatibilidad.
5. Inyecta las URLs de dependencias vía constructor NewHealthHandler(cfg).
```
