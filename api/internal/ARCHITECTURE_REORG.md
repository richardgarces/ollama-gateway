# Reorganizacion de `internal/` (vertical slices)

Se inicio la migracion hacia una estructura por modulo de dominio, agrupada bajo `internal/function/`, inspirada en:

- `internal/function/<modulo>/domain`
- `internal/function/<modulo>/repository`
- `internal/function/<modulo>/transport`
- `internal/function/<modulo>/*.go` (implementacion de negocio)

## Estado actual de migracion

### Migrados con compatibilidad

- `internal/function/profile/*`
- `internal/function/session/*`
- `internal/function/repo/*`
- `internal/function/security/*`
- `internal/function/search/*`
- `internal/function/auth/*`
- `internal/function/chat/*`
- `internal/function/openai/*`
- `internal/function/indexer/*`
- `internal/function/agent/*`
- `internal/function/generate/*`
- `internal/function/review/*`
- `internal/function/docgen/*`
- `internal/function/debug/*`
- `internal/function/translator/*`
- `internal/function/testgen/*`
- `internal/function/sqlgen/*`
- `internal/function/cicd/*`
- `internal/function/commitgen/*`
- `internal/function/architect/*`
- `internal/function/patch/*`
- `internal/function/metrics/*`
- `internal/function/dashboard/*`
- `internal/function/health/*`
- `internal/function/ws/*`
- `internal/function/api_explorer/*`
- `internal/function/core/*`

Estos modulos ya contienen implementacion movida desde la capa anterior, con imports actualizados para mantener compatibilidad.

### Estructura transversal

- `internal/config`
- `internal/middleware`
- `internal/utils`
- `internal/server`

### Legacy (a migrar por etapas)

- `internal/handlers`
- `internal/services`
- `internal/domain`

## Estado de wiring

El `server` ya instancia handlers mediante wrappers `transport` de los modulos verticales.

## Siguiente etapa recomendada

1. Consolidar carpetas documentales `internal/function/{assistant,devx,llm}` en modulos reales o eliminarlas si no aplican.
2. Eliminar `internal/handlers` e `internal/services` legacy cuando no haya dependencias residuales.
