# Ollama SaaS Gateway

[![CI](https://github.com/Richard/ollama_saas_project/actions/workflows/ci.yml/badge.svg)](https://github.com/Richard/ollama_saas_project/actions/workflows/ci.yml)

Ollama SaaS Gateway es un servidor en Go diseñado para convertir una instalación local o remota de Ollama en un gateway privado y extensible. Su objetivo es facilitar casos de uso de asistente de desarrollo (Copilot local), RAG (búsqueda+LLM), agentes con herramientas y APIs compatibles con OpenAI para integraciones.

**Para un usuario final:**
- Provee endpoints compatibles con OpenAI (`/openai/v1/...`) para generar texto, chat y embeddings.
- Permite indexar repositorios locales y usar recuperación por vectores (Qdrant) para respuestas con contexto (RAG).
- Ofrece integración con editores (scaffold de extensión VS Code y `copilot-cli`) para flujo de trabajo local y privacidad.

## **Sección Técnica**

- Lenguaje: Go (módulos Go). Arquitectura limpia (cmd/, internal/, pkg/).
- Enrutador y middlewares: `net/http` (`http.ServeMux` con method patterns), middlewares para JWT, request-id, rate limiting y métricas.
- Integraciones principales:
	- Ollama: generación y embeddings (cliente con cache LRU+TTL).
	- Qdrant: vector DB para búsquedas RAG (con fallback a store en disco).
	- MongoDB (opcional): usado para persistencia de perfiles/historial en futuras mejoras.
- Streaming: SSE para chat/completions; extensión VS Code soporta consumo streaming y fallback a CLI.
- Observabilidad: métricas JSON y endpoint Prometheus (`/metrics/prometheus`).

**Estructura principal del repo:**
- `api/cmd/*` — puntos de entrada (`server`, `copilot-cli`).
- `api/internal/config` — carga y validación de variables de entorno.
- `api/internal/services` — lógica de negocio: Ollama, RAG, indexer, qdrant, agentes.
- `api/internal/handlers` — handlers HTTP y compatibilidad OpenAI.
- `api/pkg/httputil` — helpers HTTP (SSE, WriteJSON, WriteError).

## **Casos de Uso (10 ejemplos)**

1. Asistente de programación local: autocompletar, explicación y generación de código directamente desde tu editor, sin exponer código a la nube.
2. RAG para documentación de proyecto: hacer preguntas sobre el código con contexto relevante extraído del repositorio.
3. Refactorización asistida: aplicar sugerencias del LLM como parches o diffs al código del repositorio.
4. Generación de tests automáticos: producir pruebas unitarias basadas en el código existente.
5. Code review automatizado: analizar diffs de PR y producir comentarios estructurados.
6. Agente con herramientas seguras: ejecutar acciones controladas (leer archivos, aplicar patches, ejecutar pruebas) a través de agentes limitados.
7. Asistente de debugging: enviar stack traces o logs y recibir análisis de causa raíz y sugerencias de solución.
8. Traducción de código entre lenguajes: portear funciones o módulos manteniendo coherencia con el proyecto.
9. Panel de monitoreo interno: ver métricas, estado del indexer y logs en tiempo real para operaciones locales.
10. Integración CI/CD asistida: generar pipelines y workflows sugeridos basados en la estructura del repo.

## **Mejoras Deseables / Futuro**

- Plugins/Tools para agentes (cargar tools desde YAML y registrar dinámicamente).
- Historial de conversaciones persistente por usuario (MongoDB) para contexto multi-turno.
- Routing inteligente multi-modelo (select-model por embedding/semántica).
- WebSocket bidireccional para sesiones interactivas con cancelación y control.
- Caché de respuestas RAG (LRU+TTL) y backend distribuido (Redis) para escalado.
- Modo offline robusto y manejo de modelos locales en Ollama.
- Aplicar parches/diffs automáticamente con confirmación y control de seguridad.
- Integración VS Code enriquecida: ghost text, inline completions y snippets aplicables.
- Tests de integración automáticos con testcontainers (Qdrant, MongoDB).
- Dashboard web embebido para operaciones y trazabilidad.

## Desarrollo rápido

```bash
cd api
go build ./cmd/server
go test ./...
```

## Instalación

- Guía principal de instalación y operación: [INSTALL.md](INSTALL.md)
- Compose de API + Qdrant + Mongo: [docker-compose.yml](docker-compose.yml)
- Compose separado para Ollama + WebUI: [docker-compose.ollama.yml](docker-compose.ollama.yml)

## Endpoints principales (resumen)

- `GET /health` — estado básico
- `GET /metrics` — métricas JSON
- `GET /metrics/prometheus` — métricas Prometheus
- `POST /login` — autenticación (JWT)
- `POST /openai/v1/embeddings` — compatibilidad OpenAI embeddings
- `POST /openai/v1/chat/completions` — chat con streaming SSE
- `POST /api/generate` — endpoint RAG protegido (JWT)
- `POST /internal/index/reindex` — operator endpoint para reindexar

---

Para detalles de arquitectura, desarrollo y prompts listos para Copilot, revisa la carpeta `docs/futuro/`.
