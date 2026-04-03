# Mejoras Técnicas Potenciales

> Cada sección incluye un **prompt listo para Copilot** para implementación técnica incremental.

---

## 1. Bus de Eventos Interno

Desacoplar módulos con eventos de dominio en lugar de llamadas directas.

```
Prompt Copilot:
Implementa un bus de eventos interno:
1. Crea internal/function/events/bus.go con Publish/Subscribe.
2. Define eventos: RequestCompleted, SessionCreated, FileIndexed.
3. Usa canales y workers con shutdown seguro por contexto.
4. Refactoriza 2 flujos actuales para emitir/consumir eventos.
5. Agrega pruebas unitarias de orden y entrega.
```

---

## 2. Cache Distribuido para RAG

Reducir latencia y costo reutilizando resultados frecuentes.

```
Prompt Copilot:
Implementa cache distribuido para RAG:
1. Añade Redis como dependencia opcional en config.
2. Crea internal/function/cache/service.go con Get/Set y TTL.
3. Cachea retrieval+respuesta por hash de prompt+contexto.
4. Soporta invalidación por reindex de proyecto.
5. Expón métricas hit_rate/miss_rate en /metrics.
```

---

## 3. Circuit Breaker por Proveedor

Evitar cascadas de fallo cuando Ollama/Qdrant no responde.

```
Prompt Copilot:
Implementa circuit breaker:
1. Crea internal/function/resilience/circuit_breaker.go.
2. Estados: closed/open/half-open por proveedor externo.
3. Configura thresholds y timeout por env vars.
4. Integra en clientes Ollama y Qdrant.
5. Publica estado del breaker en endpoint de health detallado.
```

---

## 4. Retries con Backoff y Jitter

Uniformar estrategia de reintentos en llamadas externas.

```
Prompt Copilot:
Implementa retry policy reusable:
1. Crea internal/function/resilience/retry.go.
2. Expon función Do(ctx, op, policy) con exponential backoff + jitter.
3. Clasifica errores retriables/no retriables.
4. Reemplaza retries ad-hoc en servicios externos.
5. Incluye tests de timing y límites de intentos.
```

---

## 5. Outbox Pattern para Consistencia

Garantizar publicación de eventos sin perder consistencia en BD.

```
Prompt Copilot:
Implementa outbox pattern:
1. Crea colección outbox_events en Mongo.
2. Al escribir entidades críticas, guarda evento en outbox en misma operación lógica.
3. Worker dedicado consume outbox y publica en bus interno.
4. Marca eventos procesados con retries y dead-letter simple.
5. Añade endpoint admin para reintento manual de outbox.
```

---

## 6. Migraciones Versionadas de Config/Schema

Evitar drift de estructura entre entornos.

```
Prompt Copilot:
Implementa sistema de migraciones:
1. Crea internal/function/migrations/runner.go.
2. Define migraciones versionadas con Up/Down.
3. Guarda versión aplicada en Mongo.
4. Ejecuta migraciones al iniciar servidor con lock distribuido.
5. Añade comando CLI para listar/aplicar/revertir migraciones.
```

---

## 7. OpenTelemetry End-to-End

Trazabilidad completa de request desde handler hasta proveedores externos.

```
Prompt Copilot:
Implementa trazas con OpenTelemetry:
1. Inicializa tracer provider configurable.
2. Instrumenta handlers, servicios y clientes HTTP.
3. Propaga trace_id en logs estructurados.
4. Exporta a OTLP endpoint configurable.
5. Documenta setup en docs/observability.md.
```

---

## 8. Pooling y Límites de Conexión

Controlar presión de recursos bajo alta concurrencia.

```
Prompt Copilot:
Implementa connection/resource pooling:
1. Revisa clientes de Mongo/Qdrant/Ollama para límites de conexiones.
2. Expón parámetros max_open, max_idle, timeout en config.
3. Añade semáforos por operación costosa (embedding/retrieval).
4. Publica métricas de saturación por pool.
5. Agrega tests de carga básicos para validar estabilidad.
```

---

## 9. Test Harness de Integración Aislado

Asegurar pruebas reproducibles con dependencias efímeras.

```
Prompt Copilot:
Implementa harness de integración:
1. Crea carpeta test/integration/harness.
2. Levanta Mongo/Qdrant/Ollama con docker compose para tests.
3. Ejecuta seed mínimo de datos de prueba.
4. Añade make target integration-test.
5. Reporta resultados y limpieza automática de recursos.
```

---

## 10. Política de Versionado de API

Preparar evolución de contratos sin romper clientes.

```
Prompt Copilot:
Implementa versionado de API:
1. Introduce prefijos /api/v1 y /api/v2 en router.
2. Mantén compatibilidad backward en handlers actuales.
3. Añade capa de traducción para campos deprecados.
4. Publica headers de deprecación y fechas objetivo.
5. Actualiza docs con guía de migración de clientes.
```
