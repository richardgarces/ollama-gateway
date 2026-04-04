# Mejoras Técnicas Potenciales

> Cada sección incluye un **prompt listo para Copilot** para implementación técnica incremental.

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
