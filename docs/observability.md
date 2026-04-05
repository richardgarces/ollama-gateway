# Observabilidad con OpenTelemetry

Este documento describe como habilitar trazas distribuidas para el gateway y exportarlas via OTLP.

## Variables de entorno

Las siguientes variables controlan OpenTelemetry:

- `OTEL_ENABLED`: habilita o deshabilita trazas (`true` o `false`).
- `OTEL_SERVICE_NAME`: nombre del servicio reportado en las trazas.
- `OTEL_EXPORTER_OTLP_ENDPOINT`: endpoint OTLP gRPC (ejemplo: `localhost:4317`).
- `OTEL_EXPORTER_OTLP_INSECURE`: usa transporte sin TLS (`true` en local).
- `OTEL_SAMPLE_PERCENT`: porcentaje de muestreo de spans (`0` a `100`).

Defaults actuales:

- `OTEL_ENABLED=false`
- `OTEL_SERVICE_NAME=ollama-gateway`
- `OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317`
- `OTEL_EXPORTER_OTLP_INSECURE=true`
- `OTEL_SAMPLE_PERCENT=100`

## Que esta instrumentado

- Middleware HTTP global: crea spans de servidor por request y propaga contexto.
- Logging middleware: agrega `trace_id` en logs estructurados.
- Cliente HTTP interno (`pkg/httpclient`): crea spans de cliente y propaga contexto saliente.
- Servicio core de Ollama: spans alrededor de operaciones de generacion y embeddings.

## Habilitar en desarrollo local

1. Levanta un collector compatible con OTLP gRPC escuchando en `4317`.
2. Exporta variables de entorno:

```bash
export OTEL_ENABLED=true
export OTEL_SERVICE_NAME=ollama-gateway-local
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
export OTEL_SAMPLE_PERCENT=100
```

3. Inicia el servidor del API normalmente.
4. Verifica en tu backend de observabilidad (por ejemplo Jaeger/Tempo/Grafana) que existan spans del servicio.

## Produccion

- Usa `OTEL_EXPORTER_OTLP_INSECURE=false` cuando el collector requiera TLS.
- Ajusta `OTEL_SAMPLE_PERCENT` para balancear costo y visibilidad.
- Mantener `OTEL_SERVICE_NAME` estable facilita dashboards y alertas por servicio.
