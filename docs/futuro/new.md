# Checklist de capacidades (estado actual)

Fecha: 2026-04-04

## Resumen

- Cumple: 17
- Parcial: 2
- No cumple: 1

## Matriz

| # | Capacidad | Estado | Evidencia técnica |
|---|---|---|---|
| 1 | Autocompletado inline | Cumple | `registerInlineCompletionItemProvider` y provider activo en `vscode-extension/extension.js` |
| 2 | Fill-in-the-Middle (FIM) | Cumple | Endpoint `POST /complete` y prompt `<PRE><SUF><MID>` en backend (`api/internal/server/server.go`, `api/internal/function/complete/service.go`) |
| 3 | Chat conversacional | Cumple | Webview de chat `copilotLocalChat` en `vscode-extension/extension.js` |
| 4 | Explicar código seleccionado | Cumple | Comando `copilot-local.explainSelection` en `vscode-extension/extension.js` |
| 5 | Refactorizar código seleccionado | Cumple | Comando `copilot-local.refactorSelection` en `vscode-extension/extension.js` |
| 6 | Generar tests unitarios | Cumple | Comando `copilot-local.addTests` + endpoints `/api/testgen` y `/api/testgen/file` |
| 7 | Generar docstrings/comentarios | Cumple | Slash `/docstring` en `resolveSlashChatPrompt` |
| 8 | Detectar bugs | Cumple | Comandos `copilot-local.fixErrors`, `copilot-local.debugError` + endpoint `/api/debug/error` |
| 9 | Traducir entre lenguajes | Cumple | Comando `copilot-local.translateSelection` + endpoint `/api/translate` |
| 10 | Búsqueda semántica en proyecto | Cumple | Comando `copilot-local.semanticSearch` en extensión con consulta a `/api/v2/search` y apertura de resultados |
| 11 | Historial de conversaciones | Cumple | Persistencia en `globalState/workspaceState`, comando `copilot-local.searchHistory` |
| 12 | Contexto multi-archivo | Parcial | Hay indexador/RAG en backend (`api/internal/function/indexer`, `api/internal/function/core/rag.go`), pero no selector explícito de archivos relacionados en UI |
| 13 | Comandos slash (/explain, /test, /refactor, /fix, /doc) | Cumple | `resolveSlashChatPrompt` soporta `/explain`, `/refactor`, `/test`, `/docstring`, `/fix` y `/doc` |
| 14 | Streaming de respuestas | Cumple | Flujo por WS/HTTP/CLI con chunks en `streamWS`, `streamHTTP`, `streamCLI` |
| 15 | Selección de modelo | Cumple | Selector de modelos en chat (`/api/models`) y setting `copilotLocal.model` |
| 16 | Soporte multi-lenguaje | Cumple | Flujo de traducción + endpoints para Go/Python/JS/TS/SQL/Bash según herramientas actuales |
| 17 | Insertar código generado | Cumple | Mensajes webview `apply` y `insert`, además inserción desde favoritos |
| 18 | Cancelar generación | Cumple | Botón `Stop` en chat + cancelación por `AbortController` para flujo WS/HTTP |
| 19 | Diff visual antes de aceptar refactor | No cumple | Hay explicación “diff-style” textual, pero no vista de diff integrada para aceptar/rechazar cambios |
| 20 | Modo offline completo + validar modelos locales | Parcial | Se añadió validación explícita (`copilot-local.validateLocalModels` + badge en chat); queda pendiente enforcement estricto de modo 100% local |

## Prioridad sugerida para cerrar brechas

1. Implementar diff visual interactivo (ideal con `vscode.diff` + apply parcial).
2. Añadir selector explícito de archivos relacionados en UI para contexto multi-archivo.
3. Endurecer modo offline para bloquear destinos no locales cuando el perfil lo requiera.

---

## Checklist no funcional (validacion tecnica)

Fecha: 2026-04-04

### Resumen

- Cumple: 12
- Parcial: 8
- No cumple: 0

### Matriz

| # | Criterio | Estado | Evidencia tecnica |
|---|---|---|---|
| 1 | Latencia menor a 500ms para primer token | Parcial | Hay metricas de tiempo de primer token y umbrales configurables, pero no SLA duro de <500ms garantizado |
| 2 | 100% local, sin telemetria de terceros | Parcial | Flujo por defecto local, pero endpoint configurable; no se observa enforcement estricto de solo local |
| 3 | Consumo RAM configurable | Parcial | Existen limites de pool/cache por env, pero no objetivo de RAM directo en MB/GB |
| 4 | Privacidad/seguridad sin enviar snippets a externos | Parcial | Rutas protegidas con JWT y enfoque local, pero API URL configurable permite destinos no locales |
| 5 | Concurrencia alta carga sin bloquear hilo principal | Cumple | Uso extendido de goroutines, WaitGroup y semaforos por canales en servicios criticos |
| 6 | Timeouts configurables (request, DB, IA) | Cumple | Timeouts por env y por cliente HTTP/context en backend |
| 7 | Logs estructurados JSON a stdout | Cumple | Logger con slog JSON/Text y niveles configurables |
| 8 | Configuracion por archivo ademas de env vars | Cumple | Soporte `CONFIG_FILE` (JSON tipo env) combinado con env vars y recarga runtime |
| 9 | Extension pesa menos de 5MB instalada | Cumple | Se agregó verificación automatizada `size:check`; footprint de artefactos runtime: 0.10MB |
| 10 | API versionada y estable | Cumple | Endpoints v1/v2 y rutas legacy con deprecation headers |
| 11 | Compatible Linux/macOS/Windows | Parcial | Stack Go/Node es portable, pero no hay evidencia fuerte de matriz de build/test cross-OS en esta validacion |
| 12 | Arranque del servicio en menos de 2s | Parcial | No hay benchmark automatizado de tiempo de arranque en repo |
| 13 | Cobertura de tests unitarios >=70% | Parcial | Se implementó gate automatizado >=70% para paquetes críticos (`make test-coverage-gate`); cobertura global histórica sigue en 34.1% |
| 14 | Documentacion de API actualizada | Cumple | API explorer local y rutas de documentacion disponibles |
| 15 | Errores HTTP con codigos correctos | Cumple | Uso consistente de helper de error y codigos 400/401/404/429/500 segun caso |
| 16 | Recarga de configuracion sin reiniciar y sin downtime | Cumple | Nuevo endpoint protegido `POST /api/admin/config/reload` aplica recarga en caliente sin reinicio del proceso |
| 17 | Despliegue en docker y binario unico | Cumple | Existe despliegue Docker y binario de servidor en flujo de build |
| 18 | Dependencias con licencia libre | Parcial | No se encontro auditoria automatizada de licencias que certifique cumplimiento total |
| 19 | Bajo acoplamiento (Clean Architecture) | Cumple | Estructura por capas clara en internal/function, config, middleware, server |
| 20 | Observabilidad integrada con Prometheus | Cumple | Endpoint /metrics/prometheus con handler Prometheus |

### Evidencia destacada

- Cobertura global backend: `total: (statements) 34.1%` (comando ejecutado: `go tool cover -func=coverage.out | tail -n 1` en `api/`).
- Coverage gate crítico (>=70%) implementado y validado: `make test-coverage-gate`.
- Tamaño de artefactos runtime de extensión validado: `node scripts/check-size.js` -> `Package footprint 0.10MB within 5MB`.

## Actualizacion de implementacion (2026-04-04)

Durante la revision de README en `docs/futuro/` se cerraron brechas concretas en la extension VS Code:

- Comando dedicado de busqueda semantica: `copilot-local.semanticSearch` (consulta a `/api/v2/search`, QuickPick de resultados y apertura de archivo cuando hay ruta resoluble).
- Validacion explicita de modelos locales: `copilot-local.validateLocalModels` + estado visible en el chat (`models: <n>` o `models: unavailable`).
- Alias slash faltantes completados en chat: `/fix` y `/doc`.
- Cancelacion de chat en curso desde UI con boton `Stop` (AbortController y cancelacion en WS/HTTP).
