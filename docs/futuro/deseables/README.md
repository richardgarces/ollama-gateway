# Deseables (Nice-to-Have)

> Mejoras que no son críticas pero elevan significativamente la calidad del producto. Cada una incluye un **prompt para Copilot**.
>
> Estado: este documento conserva solo deseables pendientes. Los ya implementados fueron removidos.

---

## 1. Explainability Mode por Respuesta

Mostrar por que una respuesta incluyo ciertos fragmentos de contexto.

```
Prompt Copilot:
Agrega modo explainability que devuelva fuentes, score de relevancia y resumen de razonamiento por etapa sin exponer cadenas internas sensibles.
```

## 2. Plantillas de Politicas por Industria

Perfiles preconfigurados para fintech/healthcare/enterprise.

```
Prompt Copilot:
Implementa plantillas de policy packs por industria con defaults de seguridad, retencion y controles de apply.
```

## 3. Simulador de Impacto de Configuracion

Probar cambios de config antes de aplicarlos.

```
Prompt Copilot:
Crea simulador de impacto que compare configuracion actual vs propuesta y estime efectos en latencia, costo y riesgo operativo.
```

## 4. Assistant Persona Packs

Personas de asistente por rol (staff engineer, SRE, security reviewer).

```
Prompt Copilot:
Implementa persona packs configurables por tenant/workspace que ajusten tono, profundidad tecnica y criterios de salida.
```

## 5. Session Replay con Diff Visual

Reproducir decisiones de una sesion y ver cambios aplicados.

```
Prompt Copilot:
Agrega session replay con timeline de eventos y visualizacion diff de acciones apply para auditoria y aprendizaje interno.
```
