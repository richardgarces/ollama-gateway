# Características Nuevas Posibles

> Cada sección incluye un **prompt listo para Copilot** que puedes pegar directamente en el chat.

---

## 1. Memoria Semántica de Largo Plazo

Persistir contexto relevante por proyecto para mejorar respuestas multi-sesión.

```
Prompt Copilot:
Implementa memoria semántica persistente:
1. Crea internal/function/memory/service.go con SaveContext/GetRelevantContext.
2. Guarda eventos resumidos en Mongo + embedding en Qdrant.
3. Integra en RAG para inyectar contexto histórico antes de generar.
4. Crea endpoints /api/memory/save y /api/memory/query.
5. Añade política de TTL y pruning por prioridad.
```

---

## 2. Planificador Multi-Step para Agentes

Permitir que un agente ejecute planes con checkpoints y reintentos.

```
Prompt Copilot:
Implementa planner multi-step para agentes:
1. Crea internal/function/planner/service.go con ExecutePlan(steps []Step).
2. Cada step registra estado: pending/running/done/failed.
3. Soporta retry por step con backoff y límite configurable.
4. Crea endpoint POST /api/agent/plan.
5. Devuelve timeline completo de ejecución y errores por paso.
```

---

## 3. Sandbox de Aplicación de Parches

Aplicar patches en entorno temporal antes de tocar archivos reales.

```
Prompt Copilot:
Implementa sandbox para patch apply:
1. Crea internal/function/sandbox/service.go.
2. Copia archivos objetivo a un directorio temporal aislado.
3. Aplica patch y valida compilación mínima en sandbox.
4. Solo si valida, permite apply real al repo.
5. Expón endpoints /api/patch/sandbox/preview y /api/patch/sandbox/apply.
```

---

## 4. Feedback Loop de Calidad

Recolectar feedback explícito para ajustar prompts y routing.

```
Prompt Copilot:
Implementa feedback loop:
1. Crea internal/function/feedback/service.go con SaveFeedback.
2. Registra rating, comentario, request_id y metadata de modelo.
3. Crea endpoint POST /api/feedback.
4. Agrega endpoint GET /api/feedback/summary para métricas agregadas.
5. Integra feedback score en estrategia de routing de modelos.
```

---

## 5. Recomendador Inteligente de Modelo

Elegir modelo por balance de latencia, costo y calidad esperada.

```
Prompt Copilot:
Implementa model recommender:
1. Crea internal/function/model_recommender/service.go.
2. Entrada: tipo de tarea, SLA de latencia, presupuesto de tokens.
3. Salida: modelo recomendado y explicación de decisión.
4. Crea endpoint POST /api/models/recommend.
5. Integrar con RouterService como hint opcional.
```

---

## 6. Context Resolver por Dependencias

Seleccionar automáticamente archivos más relevantes para contexto RAG.

```
Prompt Copilot:
Implementa resolver de contexto por grafo:
1. Usa imports AST para construir grafo de dependencias.
2. Dado un archivo/prompt, rankea archivos vecinos por relevancia.
3. Crea internal/function/context/service.go con ResolveContextFiles.
4. Integra en RAG para reducir ruido de contexto.
5. Expón endpoint POST /api/context/resolve.
```

---

## 7. Guardrails de Seguridad para Apply

Bloquear cambios potencialmente peligrosos antes de aplicar.

```
Prompt Copilot:
Implementa guardrails de apply:
1. Crea internal/function/guardrails/service.go.
2. Reglas: paths sensibles, secretos detectados, comandos peligrosos.
3. Antes de patch apply, ejecuta evaluación guardrails.
4. Si falla, devuelve rechazo con razones y remediación.
5. Añade endpoint GET /api/guardrails/rules para inspección.
```

---

## 8. Feature Flags por Tenant

Activar funcionalidades gradualmente por cliente o entorno.

```
Prompt Copilot:
Implementa feature flags:
1. Crea internal/function/flags/service.go con IsEnabled(tenant, feature).
2. Persistencia en Mongo con cache local TTL.
3. Middleware que evalúe flags para endpoints marcados.
4. Endpoints CRUD /api/flags.
5. Soporta rollout por porcentaje y fechas.
```

---

## 9. Evaluador Automático de Prompts

Medir calidad de prompts del sistema usando benchmarks repetibles.

```
Prompt Copilot:
Implementa prompt evaluator:
1. Crea internal/function/eval/service.go con RunBenchmark(suite string).
2. Carga casos de prueba desde fixtures versionados.
3. Evalúa exactitud, latencia y consistencia de salida.
4. Endpoint POST /api/eval/run y GET /api/eval/results/{id}.
5. Exporta resultados en JSON y Markdown.
```

---

## 10. Sesiones Colaborativas con Roles

Extender sesiones compartidas para roles y permisos de participante.

```
Prompt Copilot:
Implementa roles en sesiones compartidas:
1. Extiende ChatSession con participant_roles.
2. Roles: owner/editor/viewer/moderator.
3. Aplica permisos en join/chat/history endpoints.
4. Crea endpoint PATCH /api/sessions/{id}/participants/{user}/role.
5. Añade auditoría de cambios de rol con timestamp y actor.
```
