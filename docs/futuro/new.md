# Checklist de capacidades pendientes

Fecha: 2026-04-04

## Resumen

- Parcial: 2
- No cumple: 1

## Matriz

| # | Capacidad | Estado | Evidencia técnica |
|---|---|---|---|
| 12 | Contexto multi-archivo | Parcial | Hay indexador/RAG en backend (`api/internal/function/indexer`, `api/internal/function/core/rag.go`), pero no selector explícito de archivos relacionados en UI |
| 19 | Diff visual antes de aceptar refactor | No cumple | Hay explicación “diff-style” textual, pero no vista de diff integrada para aceptar/rechazar cambios |
| 20 | Modo offline completo + validar modelos locales | Parcial | Se añadió validación explícita (`copilot-local.validateLocalModels` + badge en chat); queda pendiente enforcement estricto de modo 100% local |

## Prioridad sugerida para cerrar brechas

1. Implementar diff visual interactivo (ideal con `vscode.diff` + apply parcial).
2. Añadir selector explícito de archivos relacionados en UI para contexto multi-archivo.
3. Endurecer modo offline para bloquear destinos no locales cuando el perfil lo requiera.

---

## Checklist no funcional pendiente

Fecha: 2026-04-04

### Resumen

- Parcial: 8
- No cumple: 0

### Matriz

| # | Criterio | Estado | Evidencia tecnica |
|---|---|---|---|
| 1 | Latencia menor a 500ms para primer token | Parcial | Hay metricas de tiempo de primer token y umbrales configurables, pero no SLA duro de <500ms garantizado |
| 2 | 100% local, sin telemetria de terceros | Parcial | Flujo por defecto local, pero endpoint configurable; no se observa enforcement estricto de solo local |
| 3 | Consumo RAM configurable | Parcial | Existen limites de pool/cache por env, pero no objetivo de RAM directo en MB/GB |
| 4 | Privacidad/seguridad sin enviar snippets a externos | Parcial | Rutas protegidas con JWT y enfoque local, pero API URL configurable permite destinos no locales |
| 11 | Compatible Linux/macOS/Windows | Parcial | Stack Go/Node es portable, pero no hay evidencia fuerte de matriz de build/test cross-OS en esta validacion |
| 12 | Arranque del servicio en menos de 2s | Parcial | No hay benchmark automatizado de tiempo de arranque en repo |
| 13 | Cobertura de tests unitarios >=70% | Parcial | Se implementó gate automatizado >=70% para paquetes críticos (`make test-coverage-gate`); cobertura global histórica sigue en 34.1% |
| 18 | Dependencias con licencia libre | Parcial | No se encontro auditoria automatizada de licencias que certifique cumplimiento total |

### Evidencia destacada

- Cobertura global backend: `total: (statements) 34.1%` (comando ejecutado: `go tool cover -func=coverage.out | tail -n 1` en `api/`).

## Actualizacion de implementacion (2026-04-04)

Durante la revision de README en `docs/futuro/` se cerraron brechas concretas en la extension VS Code:

- Comando dedicado de busqueda semantica: `copilot-local.semanticSearch` (consulta a `/api/v2/search`, QuickPick de resultados y apertura de archivo cuando hay ruta resoluble).
- Validacion explicita de modelos locales: `copilot-local.validateLocalModels` + estado visible en el chat (`models: <n>` o `models: unavailable`).
- Alias slash faltantes completados en chat: `/fix` y `/doc`.
- Cancelacion de chat en curso desde UI con boton `Stop` (AbortController y cancelacion en WS/HTTP).
