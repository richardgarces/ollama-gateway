# Características Nuevas Posibles

> Cada sección incluye un **prompt listo para Copilot** que puedes pegar directamente en el chat.

> Estado: este documento conserva solo features pendientes. Las ya implementadas fueron removidas.

---

## 1. Quota Policies por Tenant

Limites de consumo por requests/tokens y reglas por plan.

```
Prompt Copilot:
Implementa quota policies por tenant con enforcement en middleware, almacenamiento de consumo, burst control y endpoint de estado de cuota.
```

## 2. Workflows de Jobs (DAG)

Ejecucion de jobs dependientes con visibilidad de etapas.

```
Prompt Copilot:
Extiende el modulo jobs para soportar workflows DAG, validacion de ciclos, estado por nodo y cancelacion cascada.
```

## 3. APIDiff y Semver Guard

Guardia automatica para versionado de APIs antes de merge/release.

```
Prompt Copilot:
Crea APIDiff service y policy de semver guard que recomiende major/minor/patch segun cambios incompatibles detectados.
```

## 4. SLO Auto-Mitigation

Sugerir o ejecutar mitigaciones seguras ante degradacion.

```
Prompt Copilot:
Implementa evaluador SLO con acciones de mitigacion (fallback por feature flag, limitador temporal, rollback sugerido) y registro en auditoria.
```

## 5. Cost Explorer por Feature

Vista de costo estimado por feature/tenant/modelo.

```
Prompt Copilot:
Implementa cost explorer con agregacion por tenant, feature y modelo; endpoint de consulta filtrada y export CSV.
```
