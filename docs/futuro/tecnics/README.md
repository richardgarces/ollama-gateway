# Mejoras Técnicas Potenciales

> Cada sección incluye un **prompt listo para Copilot** para implementación técnica incremental.
>
> Estado: este documento conserva solo mejoras técnicas pendientes. Las ya implementadas fueron removidas.

---

## 1. Data Access Tenant-Aware End-to-End

Propagar tenant_id hasta storage con validaciones centralizadas.

```
Prompt Copilot:
Implementa capa tenant-aware para repositorios y servicios de datos, con validacion obligatoria de tenant_id y tests cross-tenant.
```

## 2. Distributed Locking para Jobs Criticos

Evitar ejecucion concurrente duplicada en despliegues multi-instancia.

```
Prompt Copilot:
Implementa locking distribuido para jobs criticos usando Redis, con expiracion, renovacion y fallback seguro.
```

## 3. Contract Testing Automatizado de API

Verificar compatibilidad de endpoints versionados en CI.

```
Prompt Copilot:
Agrega contract tests para /api/v1 y /api/v2 con snapshots de schema y verificacion de backward compatibility.
```

## 4. Event Sourcing Ligero para Auditoria Operativa

Normalizar eventos clave en un stream inmutable consultable.

```
Prompt Copilot:
Implementa event log append-only para acciones sensibles, con query por rango temporal y correlacion por request_id.
```

## 5. Chaos Testing Basico en Dependencias

Simular fallos de Ollama/Qdrant/Mongo/Redis para validar resiliencia.

```
Prompt Copilot:
Implementa suite de chaos tests que inyecte latencia, timeouts y errores de red en dependencias para validar degradacion controlada.
```
