# Ollama SaaS Gateway — Documentación

Este repositorio contiene un Gateway SaaS en Go que expone endpoints que orquestan Ollama y bases de datos vectoriales (Qdrant) para implementar RAG, Agentes y perfiles.

Propósito:
- Proveer una API protegida por JWT para generar respuestas, chatear, ejecutar agentes y analizar repositorios.
- Integrar con Ollama (modelo LLM) y bases externas (Qdrant, MongoDB o similares) alojadas en máquinas separadas.

Contenido de esta carpeta:
- `INSTALL.md` — Guía de instalación y despliegue mínimo.
- `MAINTENANCE.md` — Operaciones de mantenimiento, stop/start, backups y verificación.
- `ARCHITECTURE.md` — Descripción de la arquitectura y diagramas (Mermaid).
- `ENV_VARS.md` — Variables de entorno requeridas y recomendadas.

Lectura rápida:
1. Preparar máquina A con Ollama (local o remota).
2. Preparar máquina B con servicios de datos (Qdrant, MongoDB, etc.).
3. En la máquina de la API: clonar este repo, configurar `.env`, y ejecutar `go run ./cmd/server` o usar binario.

Si quieres, puedo generar un `docker-compose.yml` de ejemplo para desplegar los servicios de datos en la máquina B.
