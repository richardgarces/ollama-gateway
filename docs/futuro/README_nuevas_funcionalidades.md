# Roadmap Futuro: Nuevas Funcionalidades del Gateway

Este documento ahora conserva solo lo pendiente dentro de la carpeta /futuro.

## Estado Actual

- Existe: 13
- Parcial: 1
- No existe: 0

Pendiente real:

- Multi-tenant real: Parcial.

## Pendiente Prioritario

### Multi-tenant real

Objetivo:

- Aislamiento por tenant en storage y cache para evitar fuga de datos.
- Configuracion efectiva por tenant/workspace en toda la cadena de request.

Faltantes para marcar como Existe:

- Qdrant: colecciones/namespace por tenant.
- Cache: llaves con prefijo tenant obligatorio.
- Perfil y datos derivados: lectura/escritura tenant-aware en todos los servicios.
- Validacion de aislamiento por tests de integracion cross-tenant.

Prompt sugerido:

"Implementa multi-tenant real en el gateway Go siguiendo Clean Architecture. Incluye aislamiento por tenant en Qdrant, cache, perfiles y configuracion por workspace; propagacion de tenant_id desde middleware; validaciones de seguridad; y pruebas de integracion que garanticen no fuga de datos entre tenants."

## Nota

Las capacidades ya implementadas fueron removidas de este archivo para mantenerlo como backlog vivo de pendientes.

## Backlog Adicional (5 funcionalidades nuevas)

Estas funcionalidades se agregan como nuevo backlog y no alteran el conteo historico de la seccion Estado Actual.

1. APIDiff contractual versionado.
2. Cuotas por tenant con limites de costo/token.
3. Disaster recovery de estado operativo (snapshots + restore).
4. Orquestacion de jobs dependientes (DAG simple).
5. Auto-remediacion guiada por SLO para incidents repetitivos.

### 1) APIDiff contractual versionado

- Objetivo: detectar breaking changes entre versiones de OpenAPI antes de publicar.
- Valor: reduce regresiones en integraciones externas.

Prompt sugerido:

"Implementa APIDiff contractual: compara dos especificaciones OpenAPI, detecta breaking changes, clasifica severidad y expone endpoint POST /api/apidiff/compare con reporte JSON y modo strict/compatibility."

### 2) Cuotas por tenant con limites de costo/token

- Objetivo: controlar consumo por tenant y evitar abuso.
- Valor: habilita gobernanza economica en modo SaaS.

Prompt sugerido:

"Implementa quotas por tenant para requests y tokens por ventana temporal. Agrega middleware de enforcement, almacenamiento de consumo agregado y endpoint de consulta de cuota restante por tenant."

### 3) Disaster recovery de estado operativo

- Objetivo: poder recuperar estado de configuracion y colas tras fallo.
- Valor: mejora continuidad operativa.

Prompt sugerido:

"Implementa snapshots operativos (config/runtime/jobs/audit metadata) y restore seguro con validaciones. Expone endpoints de backup/restore y agrega pruebas de integridad post-restore."

### 4) Orquestacion de jobs dependientes

- Objetivo: permitir flujos de jobs con dependencias (DAG simple).
- Valor: automatiza pipelines complejos sin orquestador externo.

Prompt sugerido:

"Extiende job queue para soportar jobs dependientes (DAG aciclico), estados por nodo, cancelacion cascada y reporte de ejecucion por grafo."

### 5) Auto-remediacion guiada por SLO

- Objetivo: disparar acciones recomendadas cuando se degrade un SLO.
- Valor: reduce MTTR y estandariza respuesta.

Prompt sugerido:

"Implementa motor de auto-remediacion por reglas SLO: cuando latencia/error superen umbral, sugerir o ejecutar acciones seguras (rate-limit temporal, feature flag fallback, rollback asistido)."
