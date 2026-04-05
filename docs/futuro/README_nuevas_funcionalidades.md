# Roadmap Futuro: Nuevas Funcionalidades del Gateway

Este documento propone nuevas capacidades para evolucionar el gateway de Ollama hacia una plataforma SaaS de desarrollo asistido por IA, basada en el analisis de la arquitectura actual, los casos de uso ya implementados y los modulos existentes.

## 1. Estado Actual Resumido

La aplicacion ya cuenta con una base solida:

- Arquitectura limpia (handlers, services, domain, middleware, config).
- Endpoints de negocio para review, docs, debug, traduccion, tests, SQL, CI/CD, seguridad, sesiones compartidas y commit messages.
- RAG con indexacion + embeddings + Qdrant.
- Integracion con extension VS Code.
- Dashboard interno + API Explorer embebido.
- Compatibilidad OpenAI para endpoints clave.

Esto habilita una fase de expansion enfocada en escalabilidad, gobernanza, seguridad enterprise y experiencia de producto.

---

## 2. Principios para las Nuevas Funcionalidades

- Seguridad por defecto: todo endpoint sensible debe ser autenticado, auditado y rate-limited.
- Productividad medible: cada feature debe impactar tiempo de entrega, calidad o costo.
- Observabilidad total: trazas, metricas y eventos de negocio por feature.
- Extensibilidad: capacidades nuevas deben integrarse al modelo actual de services + handlers.
- Compatibilidad incremental: agregar sin romper APIs existentes.

## 2.1 Estado Validado en el Repo (2026-04-04)

Resumen de verificacion en codigo real:

- Existe: 1
- Parcial: 11
- No existe: 2

Detalle por capacidad:

- Multi-tenant real: **Parcial**.
- Cola de trabajos asincronos con API de jobs: **Parcial**.
- Modo alta disponibilidad: **Parcial**.
- RBAC y scopes por endpoint: **No existe**.
- Secret scanning y policy engine: **Parcial**.
- Auditoria trazable global: **Parcial**.
- Refactor guiado por patrones: **Parcial**.
- Asistente de performance: **Existe**.
- Test intelligence por diff: **Parcial**.
- Copilot Local avanzado (4 comandos nuevos): **No existe**.
- Playbooks por incidente (API + VS Code): **Parcial**.
- API product externos (versionado + SDK): **Parcial**.
- Metricas de valor: **Parcial**.
- Trazas por feature con breakdown completo: **Parcial**.

## 2.2 Delta Prioritario Recomendado

Para alinear roadmap y estado actual, el siguiente paquete entrega el mayor impacto inmediato:

1. RBAC + scopes por endpoint (cubre riesgo operacional critico).
2. Auditoria global de acciones sensibles (quien, cuando, endpoint, resultado).
3. Job queue con endpoints de ciclo completo (crear, estado, cancelar, resultado).
4. Comandos VS Code faltantes: `architectReview` y `securityScanCurrentFile` como primer tramo.

Este delta deja lista la base de gobernanza y operacion para entrar en multi-tenant y policy engine en la siguiente fase.

---

## 3. Nuevas Funcionalidades Propuestas

## 3.1 Plataforma y Escalabilidad

### 3.1.1 Multi-tenant real

- Tenant isolation por:
  - colecciones vectoriales
  - claves de cache
  - perfiles
  - configuraciones por workspace
- Configuracion por tenant:
  - modelo por defecto
  - limites de consumo
  - politicas de seguridad

Valor: habilita uso SaaS empresarial con separacion estricta de datos.

Prompt sugerido:
"Disena e implementa multi-tenant real en el gateway Go siguiendo Clean Architecture. Incluye aislamiento por tenant en Qdrant, cache, perfiles y configuracion por workspace, propagacion de tenant_id en middleware, validaciones de seguridad, migraciones necesarias y tests unitarios + integracion para evitar fuga de datos entre tenants."

### 3.1.2 Cola de trabajos asincronos

- Job queue para procesos pesados:
  - indexacion masiva
  - scan de seguridad full repo
  - analisis de arquitectura
  - docgen/readme
- Endpoints de jobs:
  - crear job
  - consultar estado
  - cancelar job
  - obtener resultado

Valor: evita timeouts y mejora UX en operaciones largas.

Prompt sugerido:
"Implementa una cola de trabajos asincronos para indexacion, security scan, analisis de arquitectura y docgen. Crea endpoints para crear job, consultar estado, cancelar job y obtener resultado. Usa workers con control de concurrencia, reintentos, timeout configurable, trazabilidad por request_id y tests para exito/error/cancelacion."

### 3.1.3 Modo alta disponibilidad

- Cache distribuida (Redis) para respuestas RAG y sesiones temporales.
- Coordinacion de workers para indexacion y scans.
- Health checks avanzados por dependencia (Ollama/Qdrant/Mongo/Redis).

Valor: operacion estable en escenarios de mayor carga.

Prompt sugerido:
"Fortalece el modo alta disponibilidad del gateway: integra cache distribuida Redis para RAG/sesiones, coordinacion de workers para evitar trabajo duplicado y health checks avanzados por dependencia (Ollama, Qdrant, Mongo, Redis). Incluye metricas Prometheus y pruebas de degradacion/fallback."

---

## 3.2 Seguridad y Gobernanza

### 3.2.1 RBAC y scopes por endpoint

- Roles sugeridos: admin, maintainer, developer, viewer.
- Scopes por feature:
  - security:scan
  - cicd:apply
  - docs:apply
  - patch:apply
  - indexer:control

Valor: reduce riesgo operacional y cumple requisitos enterprise.

Prompt sugerido:
"Agrega RBAC con roles admin/maintainer/developer/viewer y scopes por endpoint (security:scan, cicd:apply, docs:apply, patch:apply, indexer:control). Implementa middleware de autorizacion, errores HTTP consistentes, auditoria de denegaciones y tests de matriz rol x endpoint."

### 3.2.2 Secret scanning y policy engine

- Escaneo de secretos (tokens, keys, passwords hardcoded).
- Politicas configurables:
  - bloquear apply en cicd/sql/docs si hay findings criticos
  - exigir revisiones humanas en acciones de alto impacto

Valor: prevencion temprana de incidentes de seguridad.

Prompt sugerido:
"Implementa secret scanning y un policy engine configurable. Detecta secretos comunes en codigo/diffs, clasifica severidad y bloquea acciones apply de alto impacto cuando haya findings criticos. Permite reglas por tenant/workspace y agrega reportes claros + tests de politicas."

### 3.2.3 Auditoria trazable

- Bitacora de acciones sensibles:
  - quien
  - cuando
  - endpoint
  - payload resumido
  - resultado
- Exportacion para cumplimiento (JSON/CSV/SIEM).

Valor: cumplimiento y forensica de cambios.

Prompt sugerido:
"Implementa auditoria trazable de acciones sensibles registrando quien, cuando, endpoint, payload resumido y resultado. Define esquema de almacenamiento, endpoint de consulta filtrada y exportacion JSON/CSV. Asegura redaccion de datos sensibles y cobertura de pruebas sobre integridad del log."

---

## 3.3 Inteligencia de Desarrollo

### 3.3.1 Refactor guiado por patrones

- Deteccion de code smells recurrentes.
- Plantillas de refactor automatico:
  - extraer interfaz
  - separar responsabilidades
  - estandarizar manejo de errores
- Propuesta con diff + riesgo estimado.

Valor: mejora mantenibilidad y consistencia arquitectonica.

Prompt sugerido:
"Crea un modulo de refactor guiado por patrones que detecte code smells y proponga refactors estandar (extraer interfaz, separar responsabilidades, manejo uniforme de errores). Devuelve diff sugerido, riesgo estimado y justificacion tecnica. Incluye pruebas sobre casos reales del repositorio."

### 3.3.2 Asistente de performance

- Analisis de hotspots de codigo y consultas.
- Sugerencias de optimizacion:
  - concurrencia
  - memoria
  - estructuras de datos
  - query plans

Valor: mejora performance sin ensayo-error manual.

Prompt sugerido:
"Construye un asistente de performance que analice hotspots de codigo/consultas y entregue recomendaciones accionables sobre concurrencia, memoria, estructuras de datos y queries. Incluye scoring de impacto/ esfuerzo, evidencias y endpoints para ejecutar analisis por archivo o repo."

### 3.3.3 Test intelligence

- Priorizacion de tests por impacto de cambios.
- Generacion de suites de regresion por diff.
- Reporte de riesgo de merge.

Valor: feedback mas rapido con mayor confianza.

Prompt sugerido:
"Implementa test intelligence por diff: priorizacion de tests por impacto, generacion de suite de regresion y reporte de riesgo de merge. Usa analisis de dependencias/cambios, integra con endpoints existentes y produce salida consumible por VS Code y CI/CD."

---

## 3.4 Experiencia de Producto (VS Code + API)

### 3.4.1 Copilot Local avanzado en VS Code

- Comandos nuevos:
  - copilot-local.architectReview
  - copilot-local.securityScanCurrentFile
  - copilot-local.generatePipelineAndOpen
  - copilot-local.createPRSummary
- Integracion con Source Control:
  - sugerencia de PR title/body
  - checklists automaticos por tipo de cambio

Valor: flujo end-to-end sin salir del editor.

Prompt sugerido:
"Extiende la extension de VS Code con comandos architectReview, securityScanCurrentFile, generatePipelineAndOpen y createPRSummary. Conecta cada comando al backend, maneja errores con UX clara, agrega telemetria local de latencia y tests de comandos criticos."

### 3.4.2 Playbooks por tipo de incidente

- Plantillas para debugging y respuesta:
  - build roto
  - test flaky
  - degradacion de latencia
  - findings de seguridad

Valor: estandariza respuestas operativas y reduce MTTR.

Prompt sugerido:
"Implementa playbooks ejecutables para incidentes (build roto, test flaky, degradacion de latencia, findings de seguridad). Cada playbook debe tener pasos, comandos sugeridos, criterios de salida y evidencia requerida. Exponerlos por API y UI de VS Code."

### 3.4.3 API products externos

- Versionado formal de API (v1/v2).
- SDK ligero (JS/TS y Go).
- Documentacion publica con ejemplos por caso de uso.

Valor: facilita adopcion por equipos y terceros.

Prompt sugerido:
"Productiza la API externa: consolida versionado formal v1/v2, genera SDK minimo para JS/TS y Go, y publica documentacion con ejemplos por caso de uso. Mantiene compatibilidad retroactiva con deprecations claras y pruebas contractuales."

---

## 3.5 Observabilidad de Negocio

### 3.5.1 Metricas de valor

- KPIs sugeridos:
  - tiempo promedio para resolver tasks de codigo
  - porcentaje de respuestas reutilizadas por cache
  - ratio de findings criticos detectados antes de merge
  - tasa de aceptacion de sugerencias de commit/tests/docs

Valor: decisiones de roadmap basadas en impacto real.

Prompt sugerido:
"Implementa metricas de valor de negocio en el gateway y dashboard: tiempo promedio de resolucion, ratio de cache reutilizada, findings criticos pre-merge y tasa de aceptacion de sugerencias. Define eventos, agregaciones, endpoint de consulta y visualizaciones basicas."

### 3.5.2 Trazas por feature

- Correlacion request -> servicio -> LLM -> storage.
- Breakdown de latencia por etapa (RAG, embedding, vector search, generation).

Valor: diagnostico rapido y optimizacion dirigida.

Prompt sugerido:
"Agrega trazas por feature con correlacion request -> servicio -> LLM -> storage y breakdown de latencia por etapa (RAG, embedding, vector search, generation). Expone datos via Prometheus/logs estructurados y asegura trazabilidad por request_id."

---

## 4. Roadmap Sugerido (90 dias)

## Fase 1 (0-30 dias)

- RBAC basico + scopes.
- Auditoria de endpoints sensibles.
- Job queue para indexacion y security scan.
- Comandos VS Code: securityScanCurrentFile y architectReview.

## Fase 2 (31-60 dias)

- Multi-tenant basico.
- Redis para cache distribuida.
- Test intelligence por diff.
- PR summary generator.

## Fase 3 (61-90 dias)

- Policy engine de seguridad.
- API productization (versionado + SDK).
- Metricas de negocio en dashboard.
- Hardening de HA y resiliencia.

---

## 5. Criterios de Exito

- Reduccion de 25% en tiempo de tareas repetitivas de desarrollo.
- Reduccion de 30% en incidentes de seguridad por errores evitables.
- Mejora del 20% en lead time de PR a merge.
- Disponibilidad > 99.5% en ambiente objetivo.

---

## 6. Proxima Accion Recomendada

Comenzar por un paquete de alto impacto y bajo riesgo:

1. RBAC + auditoria.
2. Job queue para operaciones largas.
3. Extension VS Code con comandos de seguridad y arquitectura.

Este paquete habilita gobernanza, escalabilidad operacional y mejora inmediata de experiencia de equipo.
