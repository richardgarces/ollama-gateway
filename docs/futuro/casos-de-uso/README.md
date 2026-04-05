# Casos de Uso Posibles

> Escenarios concretos de uso del gateway con el **prompt de Copilot** necesario para implementar cada uno.
>
> Estado: este documento conserva solo casos de uso pendientes. Los ya implementados fueron removidos.

---

## 1. Analisis de Breaking Changes en API

Detectar cambios incompatibles entre versiones de contratos OpenAPI/JSON.

```
Prompt Copilot:
Implementa un analizador de breaking changes de API:
1. Crea internal/function/apidiff/service.go con APIDiffService que compare dos especificaciones OpenAPI.
2. Detecta cambios incompatibles: eliminación de rutas, cambios de tipos, campos requeridos nuevos.
3. Crea internal/function/apidiff/transport/apidiff.go con endpoint POST /api/apidiff/compare.
4. Retorna un reporte con severity, endpoint afectado y sugerencia de migración.
5. Agrega soporte para modo "strict" y "compatibility".
```

---

## 2. Control de Cuota por Tenant

Aplicar limites de requests/tokens por tenant con alertas y bloqueo temporal.

```
Prompt Copilot:
Implementa control de cuota por tenant:
1. Crea internal/function/quota/service.go con QuotaService.
2. Enforce por middleware usando tenant_id del contexto.
3. Soporta limites por minuto/hora y burst.
4. Crea endpoint GET /api/quota/status.
5. Devuelve remaining, reset_at y accion recomendada.
```

---

## 3. Restore Operativo Guiado

Recuperar estado de jobs/configuracion tras una caida.

```
Prompt Copilot:
Implementa restore operativo:
1. Crea internal/function/recovery/service.go.
2. Define snapshot de estado minimo (jobs, runtime config, metadata).
3. Endpoint POST /api/recovery/snapshot y POST /api/recovery/restore.
4. Validar integridad y version de snapshot.
5. Reportar diff entre estado previo y restaurado.
```

---

## 4. Planeador de Jobs Dependientes

Ejecutar flujos de jobs con dependencias y estados por etapa.

```
Prompt Copilot:
Implementa orquestacion DAG de jobs:
1. Extiende modulo jobs con dependencias entre nodos.
2. Validar DAG aciclico al crear workflow.
3. Endpoint POST /api/jobs/workflows.
4. Endpoint GET /api/jobs/workflows/{id} con estado por nodo.
5. Cancelacion cascada y reintento selectivo por nodo.
```

---

## 5. Respuesta Automatica ante Degradacion SLO

Disparar acciones mitigadoras cuando se exceden umbrales de SLO.

```
Prompt Copilot:
Implementa respuesta por SLO:
1. Crea internal/function/slo/service.go con reglas y acciones.
2. Monitorea latencia/error rate por feature.
3. Endpoint POST /api/slo/evaluate para evaluacion manual.
4. Registrar recomendaciones/acciones en auditoria.
5. Soportar modo suggest y mode enforce.
```

---

## 6. Matriz de Compatibilidad de Modelos

Recomendar modelo por tarea, costo y restricciones de tenant.

```
Prompt Copilot:
Implementa matriz de compatibilidad de modelos:
1. Crea internal/function/modelmatrix/service.go.
2. Evaluar modelos por task_type, latencia esperada y token_budget.
3. Endpoint POST /api/models/matrix/recommend.
4. Explicar razones del ranking por modelo.
5. Permitir politicas por tenant para modelos permitidos.
```

