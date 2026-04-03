# Casos de Uso Posibles

> Escenarios concretos de uso del gateway con el **prompt de Copilot** necesario para implementar cada uno.

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

## 2. Asistente de Postmortem

Construir un timeline y causa raíz probable desde logs, métricas y cambios recientes.

```
Prompt Copilot:
Implementa un asistente de postmortem:
1. Crea internal/function/postmortem/service.go con PostmortemService.
2. Método AnalyzeIncident(input IncidentInput) (IncidentReport, error).
3. Input incluye logs, intervalo de tiempo, commit hash opcional y métricas.
4. Crea endpoint POST /api/postmortem/analyze.
5. Devuelve timeline, hipótesis de causa raíz, impacto y acciones preventivas.
```

---

## 3. Generador de Runbooks

Crear playbooks operativos reutilizables desde incidentes frecuentes.

```
Prompt Copilot:
Implementa generación de runbooks:
1. Crea internal/function/runbook/service.go con GenerateRunbook(incidentType string, context string).
2. Crea endpoint POST /api/runbooks/generate.
3. Genera pasos de diagnóstico, mitigación, rollback y validación post-fix.
4. Permite guardar resultado en docs/runbooks/{tipo}.md con apply=true.
5. Valida que la ruta destino esté dentro de REPO_ROOT.
```

---

## 4. Revisor de Migraciones SQL

Evaluar riesgo de migraciones antes de aplicarlas en producción.

```
Prompt Copilot:
Implementa un revisor de migraciones SQL:
1. Crea internal/function/sqlreview/service.go con ReviewMigration(sql string, dialect string).
2. Detecta operaciones peligrosas: locks largos, drop sin backup, alter costoso.
3. Crea endpoint POST /api/sql/review.
4. Devuelve findings, riesgo global y recomendaciones de rollout.
5. Incluye checks de rollback e idempotencia.
```

---

## 5. Asistente de Release Notes

Generar release notes desde commits, PRs y cambios de API.

```
Prompt Copilot:
Implementa generación de release notes:
1. Crea internal/function/release/service.go con BuildReleaseNotes(fromRef, toRef string).
2. Integra lectura de git log y convenciones Conventional Commits.
3. Crea endpoint POST /api/release/notes.
4. Secciones: Features, Fixes, Breaking Changes, Security.
5. Opcional apply=true para escribir CHANGELOG.md.
```

---

## 6. Asistente de Performance por Endpoint

Priorizar optimizaciones según p95/p99 y errores por ruta.

```
Prompt Copilot:
Implementa un asistente de performance de endpoints:
1. Crea internal/function/perf/service.go con AnalyzeEndpoints().
2. Consume métricas existentes y calcula ranking de endpoints críticos.
3. Crea endpoint GET /api/perf/endpoints.
4. Devuelve latencia p50/p95/p99, error rate y recomendaciones concretas.
5. Incluye score de impacto esperado por optimización sugerida.
```

---

## 7. Validador de Seguridad Pre-Deploy

Bloquear despliegues cuando existan findings críticos no resueltos.

```
Prompt Copilot:
Implementa un validador pre-deploy:
1. Crea internal/function/gate/service.go con CheckDeployGate().
2. Integra findings de security scan, cobertura mínima y estado de tests.
3. Crea endpoint GET /api/gate/deploy.
4. Respuesta: allow=true/false, razones y acciones requeridas.
5. Soporta perfiles strict y relaxed por entorno.
```

---

## 8. Asistente de Onboarding Tecnico

Generar guía de onboarding por rol usando contexto del repositorio.

```
Prompt Copilot:
Implementa onboarding automático por rol:
1. Crea internal/function/onboarding/service.go con GenerateGuide(role string).
2. Roles: backend, frontend, devops, qa.
3. Crea endpoint POST /api/onboarding/guide.
4. Incluye pasos de setup, comandos útiles y rutas clave del proyecto.
5. Soporta export a Markdown con apply=true.
```

---

## 9. Resumen Ejecutivo de PR

Crear resumen técnico y de riesgo para revisores no autores.

```
Prompt Copilot:
Implementa resumen ejecutivo de PR:
1. Crea internal/function/prsummary/service.go con SummarizeDiff(diff string).
2. Identifica alcance, riesgo, componentes afectados y pruebas sugeridas.
3. Crea endpoint POST /api/pr/summary.
4. Devuelve título sugerido, resumen y checklist de revisión.
5. Incluye semáforo de riesgo (low/medium/high).
```

---

## 10. Priorizador de Deuda Tecnica

Detectar y rankear deuda técnica por impacto y esfuerzo.

```
Prompt Copilot:
Implementa priorización de deuda técnica:
1. Crea internal/function/techdebt/service.go con AnalyzeTechDebt().
2. Usa señales: complejidad, churn, bugs históricos, cobertura baja.
3. Crea endpoint GET /api/techdebt/priorities.
4. Devuelve backlog ordenado con esfuerzo estimado y valor esperado.
5. Exporta reporte a docs/techdebt.md con apply=true.
```
