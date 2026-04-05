# Mejoras Técnicas Potenciales

> Cada sección incluye un **prompt listo para Copilot** para implementación técnica incremental.
>
> Estado: este documento conserva solo mejoras técnicas pendientes. Las ya implementadas fueron removidas.

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
