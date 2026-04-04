# Características Nuevas Posibles

> Cada sección incluye un **prompt listo para Copilot** que puedes pegar directamente en el chat.

> Estado: este documento conserva solo features pendientes. Las ya implementadas fueron removidas.

---

## 1. Feature Flags por Tenant

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
