# Deseables (Nice-to-Have)

> Mejoras que no son críticas pero elevan significativamente la calidad del producto. Cada una incluye un **prompt para Copilot**.

---

## 1. Modo Dictado por Voz en Chat

Permitir entrada por voz en el panel de chat de la extensión.

```
Prompt Copilot:
Añade modo dictado al chat panel:
1. En vscode-extension/extension.js agrega botón "Mic" en el header.
2. Usa Web Speech API (cuando esté disponible) para capturar voz.
3. Inserta el texto transcrito en el input del chat.
4. Muestra estado visual: escuchando / detenido.
5. Agrega setting copilotLocal.voiceInputEnabled (default: false).
```

---

## 2. Plantillas de Prompt Rápidas

Acciones rápidas para insertar prompts predefinidos por contexto.

```
Prompt Copilot:
Implementa prompt templates rápidos:
1. Crea comando copilot-local.quickPrompt.
2. Muestra QuickPick con plantillas: explain, optimize, secure, test.
3. Inserta la plantilla en el chat panel usando selección activa del editor.
4. Permite definir templates custom por setting JSON.
5. Guarda último template usado por workspace.
```

---

## 3. Historial Buscable de Chat

Buscar conversaciones previas desde la extensión.

```
Prompt Copilot:
Añade historial buscable del chat:
1. Persiste mensajes en globalState por workspace.
2. Crea comando copilot-local.searchHistory.
3. Muestra QuickPick con resultados por coincidencia textual.
4. Al seleccionar resultado, abre chat panel y resalta el mensaje.
5. Agrega botón "Clear History" con confirmación.
```

---

## 4. Comparador de Respuestas de Modelos

Comparar salidas de dos modelos para el mismo prompt.

```
Prompt Copilot:
Implementa modo compare de modelos:
1. En chat panel agrega botón "Compare".
2. Permite elegir dos modelos disponibles de /api/models.
3. Envía el mismo prompt a ambos modelos y muestra columnas lado a lado.
4. Incluye tiempo de respuesta y longitud de salida por modelo.
5. Agrega comando copilot-local.compareModels para abrir esta vista.
```

---

## 5. Favoritos de Respuestas

Guardar respuestas útiles para reutilización futura.

```
Prompt Copilot:
Implementa favoritos en chat:
1. Agrega botón "Star" en cada mensaje assistant.
2. Guarda favoritos en globalState con {title, content, timestamp}.
3. Crea comando copilot-local.openFavorites.
4. Permite copiar o aplicar directamente un favorito al editor.
5. Soporta eliminar favoritos desde la misma vista.
```

---

## 6. Modo Focus para Bloques de Código

Filtrar solo snippets de código en una respuesta larga.

```
Prompt Copilot:
Añade modo focus de código:
1. En el chat panel agrega toggle "Code only".
2. Cuando esté activo, renderiza solo bloques fenced code.
3. Muestra contador de bloques extraídos y lenguaje detectado.
4. Mantén botones Copy/Apply por bloque.
5. Permite volver a vista completa sin perder scroll.
```

---

## 7. Alertas de Calidad de Respuesta

Notificar cuando se degrade la experiencia del usuario.

```
Prompt Copilot:
Implementa alertas de calidad local:
1. Usa métricas locales para detectar: latencia alta, errores consecutivos.
2. Define umbrales configurables por setting.
3. Muestra warning no intrusivo en status bar cuando se excedan.
4. Agrega comando copilot-local.resetQualityAlerts.
5. Registra eventos de alerta en output channel de la extensión.
```

---

## 8. Perfiles de Uso por Workspace

Configurar presets por proyecto (modelo, temperatura, idioma).

```
Prompt Copilot:
Implementa perfiles por workspace:
1. Crea settings scoped por workspace para model/lang/temperature.
2. Añade comando copilot-local.switchProfile.
3. Define perfiles default: fast, balanced, deep-analysis.
4. Al cambiar perfil, actualiza chat panel y comandos automáticamente.
5. Muestra perfil activo en status bar.
```

---

## 9. Restauración de Sesión al Reiniciar

Recuperar el estado del chat al volver a abrir VS Code.

```
Prompt Copilot:
Añade restore de sesión de chat:
1. Persiste estado del panel (mensajes, modelo seleccionado, sesión activa).
2. Al activar la extensión, restaura estado si existe.
3. Muestra banner "session restored" en el panel.
4. Agrega comando copilot-local.clearSessionState.
5. Maneja compatibilidad de versión de esquema de estado.
```

---

## 10. Explicador de Fallos de Tests

Generar diagnóstico rápido a partir de salida de tests.

```
Prompt Copilot:
Implementa comando explain test failure:
1. Crea comando copilot-local.explainTestFailure.
2. Captura salida reciente del terminal de tests o selección activa.
3. Construye prompt con contexto de archivo y error.
4. Envía al chat panel y muestra hipótesis + plan de corrección.
5. Ofrece botón para generar test de regresión sugerido.
```
