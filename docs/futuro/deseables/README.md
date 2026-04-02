# Deseables (Nice-to-Have)

> Mejoras que no son críticas pero elevan significativamente la calidad del producto. Cada una incluye un **prompt para Copilot**.

---

## 1. Auto-completado de Código en Tiempo Real (Ghost Text)

Emular el comportamiento de Copilot: sugerir completions mientras el usuario escribe en VS Code.

```
Prompt Copilot:
Implementa auto-completado tipo "ghost text" en la extensión VS Code:
1. En vscode-extension/extension.js, registra un InlineCompletionItemProvider
   con vscode.languages.registerInlineCompletionItemProvider('*', provider).
2. El provider debe:
   a. Tomar las últimas 50 líneas antes del cursor como contexto.
   b. Hacer POST a {apiUrl}/openai/v1/completions con {prompt: contexto,
      stream: false, max_tokens: 100}.
   c. Devolver la respuesta como InlineCompletionItem.
3. Incluir debounce de 500ms para no saturar el servidor mientras el usuario
   escribe rápidamente.
4. Añadir setting copilotLocal.inlineCompletions (boolean, default: true)
   para activar/desactivar la feature.
5. Mostrar un indicador sutil en la status bar cuando esté buscando completions.
No envíes requests si el documento tiene menos de 3 líneas.
```

---

## 2. Temas y Personalización del Chat Panel

Permitir que el usuario personalice colores y fuentes del panel de chat webview.

```
Prompt Copilot:
Extiende el panel de chat webview en vscode-extension/extension.js:
1. Modifica getChatPanelHTML() para leer los colores del tema de VS Code
   usando variables CSS de VS Code: var(--vscode-editor-background),
   var(--vscode-editor-foreground), var(--vscode-button-background), etc.
2. Reemplaza los colores hardcoded del CSS (:root {...}) por estas variables.
3. Añade soporte para fuente monoespaciada en bloques de código: detecta
   contenido entre ``` y renderízalo con <pre><code> y la fuente
   var(--vscode-editor-font-family).
4. Añade un botón "Copy" en cada bloque de código que copie el contenido
   al clipboard usando navigator.clipboard.writeText.
5. Añade setting copilotLocal.chatFontSize (number, default: 13).
```

---

## 3. Export de Conversaciones

Permitir exportar el historial de chat a Markdown, JSON o texto plano.

```
Prompt Copilot:
Añade funcionalidad de export al panel de chat en vscode-extension/extension.js:
1. Agrega un botón "Export" en el header del chat webview.
2. Al hacer click, postMessage al host con {type:"export"}.
3. En el host (extension.js), captura el mensaje y muestra un QuickPick
   con opciones: "Markdown", "JSON", "Text".
4. Según la opción:
   - Markdown: genera un .md con ## User / ## Assistant y bloques de código.
   - JSON: genera un array de {role, content, timestamp}.
   - Text: texto plano con prefijos "You:" y "AI:".
5. Usa vscode.workspace.openTextDocument({content, language}) para abrir
   el resultado en un nuevo editor tab.
Para esto, el webview debe mantener un array de mensajes en JS y enviarlo
en el postMessage de export.
```

---

## 4. Notificaciones de Indexación

Mostrar progreso y notificaciones cuando el indexer está procesando archivos.

```
Prompt Copilot:
Añade notificaciones de progreso de indexación en la extensión VS Code:
1. En vscode-extension/extension.js, crea un comando copilot-local.reindex que:
   a. Haga POST a {apiUrl}/internal/index/reindex.
   b. Muestre un vscode.window.withProgress() con ProgressLocation.Notification
      y título "Indexing repository...".
   c. Periódicamente (cada 2s) haga GET a {apiUrl}/health para verificar
      que el servidor sigue activo, actualizando el mensaje de progreso.
   d. Al completar, muestre una notificación de éxito.
2. Registra el comando en package.json con título "Copilot Local: Reindex Repository".
3. Añade un StatusBarItem permanente que muestre el estado del indexer
   ("Indexed ✓" o "Indexing...") consultando el servidor al activar la extensión.
```

---

## 5. Syntax Highlighting en Respuestas del Chat

Renderizar código con highlighting en el panel webview usando una librería ligera.

```
Prompt Copilot:
Añade syntax highlighting al panel de chat en vscode-extension/extension.js:
1. Integra highlight.js (versión CDN minificada) en getChatPanelHTML():
   - Agrega un <link> al CSS de un tema oscuro de highlight.js (github-dark).
   - Agrega un <script> src a highlight.min.js con los lenguajes: go,
     javascript, typescript, python, bash, json, yaml, sql.
2. En el script del webview, después de añadir cada mensaje al DOM, busca
   bloques <pre><code> y llama hljs.highlightElement(el).
3. Parsea el texto markdown de la respuesta: detecta bloques ```lang\n...\n```
   y conviértelos a <pre><code class="language-{lang}">...</code></pre>.
   Escapa HTML del contenido dentro de los bloques.
4. Los bloques de código deben tener borde redondeado, padding y fondo
   ligeramente distinto al del chat.
Usa una función sanitizeHTML() para escapar < > & " en el texto fuera
de bloques de código para prevenir XSS.
```

---

## 6. Modo Offline con Modelos Pre-descargados

Detectar automáticamente si Ollama está disponible y trabajar con modelos ya descargados.

```
Prompt Copilot:
Implementa detección de modo offline en internal/services/ollama.go:
1. Agrega un método Ping() bool que haga GET a {OllamaURL}/ con timeout de 2s.
2. Agrega un método ListModels() ([]string, error) que haga GET a
   {OllamaURL}/api/tags y retorne los nombres de modelos disponibles.
3. Al inicializar OllamaService (constructor), llama Ping(). Si falla,
   loguea warning "Ollama offline — generation disabled" pero no hagas
   panic (permite que el servidor arranque en modo degradado).
4. En Generate y StreamGenerate, si Ollama no responde, devuelve un error
   descriptivo: "Ollama is not available. Check that the service is running."
5. Crea un endpoint GET /api/models en internal/handlers/models.go que
   devuelva la lista de modelos disponibles (o vacío si offline).
6. En la extensión VS Code, al abrir el chat panel, consulta /api/models
   y muestra los modelos disponibles en un <select> del UI.
```

---

## 7. Atajos de Teclado Contextuales

Añadir atajos de teclado que envíen acciones específicas al gateway según el contexto del editor.

```
Prompt Copilot:
Añade comandos contextuales a la extensión VS Code:
1. Registra estos nuevos comandos en package.json y extension.js:
   - copilot-local.explainSelection: envía "Explain this code:\n{selection}"
   - copilot-local.refactorSelection: envía "Refactor this code for clarity:\n{selection}"
   - copilot-local.addTests: envía "Write unit tests for this code:\n{selection}"
   - copilot-local.fixErrors: envía "Fix the errors in this code:\n{selection}"
2. Cada comando abre el chat panel (si no está abierto) y envía el prompt
   pre-construido automáticamente.
3. Añade entries al menú contextual del editor (contributes.menus.editor/context)
   con un submenú "Copilot Local >" que liste estas acciones.
4. Asigna keybindings:
   - Explain: Cmd+Shift+E
   - Refactor: Cmd+Shift+R
   - Tests: Cmd+Shift+T
   - Fix: Cmd+Shift+F
Todos los comandos deben usar streamHTTP con fallback a CLI.
```

---

## 8. Snippets Interactivos (Apply Code)

Permitir aplicar bloques de código de la respuesta del chat directamente al editor activo.

```
Prompt Copilot:
Implementa "Apply Code" desde el chat panel:
1. En getChatPanelHTML(), junto al botón "Copy" de cada bloque de código,
   añade un botón "Apply" que envíe postMessage({type:"apply", code, lang}).
2. En extension.js, al recibir el mensaje "apply":
   a. Si hay un editor activo, inserta el código en la posición del cursor
      usando editor.edit(editBuilder => editBuilder.insert(pos, code)).
   b. Si el código es un diff (detectar si empieza con --- o @@),
      muestra un vscode.workspace.applyEdit con los cambios calculados.
   c. Si no hay editor activo, abre un nuevo documento con el contenido.
3. Antes de aplicar, muestra un diálogo de confirmación:
   vscode.window.showInformationMessage("Apply code to editor?", "Yes", "No").
4. Después de aplicar, formatea el documento con
   vscode.commands.executeCommand('editor.action.formatDocument').
```

---

## 9. Métricas de Uso en la Extensión

Trackear uso local (sin telemetría externa) para mostrar estadísticas al usuario.

```
Prompt Copilot:
Implementa métricas de uso locales en la extensión VS Code:
1. Crea un archivo vscode-extension/metrics.js con una clase LocalMetrics que:
   - Almacene datos en context.globalState (persistente entre sesiones).
   - Trackee: total_requests, total_tokens_approx (chars/4),
     requests_per_day (mapa fecha→count), avg_response_time_ms, errors_count.
2. Instrumenta streamHTTP: mide tiempo desde request hasta primer chunk
   (time_to_first_token) y hasta done (total_time). Cuenta chars recibidos.
3. Registra un comando copilot-local.showStats que abra un webview panel
   mostrando:
   - Requests hoy / esta semana / total.
   - Tiempo promedio de respuesta.
   - Tokens aproximados consumidos.
   - Gráfico ASCII simple de requests/día (últimos 7 días).
4. Registra el comando en package.json.
No envíes datos a ningún servidor externo — todo queda local.
```

---

## 10. Soporte Multi-idioma en Prompts del Sistema

Permitir que el system prompt del RAG esté en el idioma preferido del usuario.

```
Prompt Copilot:
Añade soporte multi-idioma para los system prompts:
1. Crea internal/services/prompts/ con archivos: en.go, es.go, pt.go.
   Cada archivo exporta un map[string]string con claves como:
   "rag_system", "code_explain", "code_refactor", "code_test".
2. Crea internal/services/prompts/prompts.go con una función
   Get(lang, key string) string que busque en el mapa del idioma solicitado,
   con fallback a "en" si el idioma no existe.
3. Añade PROMPT_LANG (default: "en") a config.go.
4. Modifica internal/services/rag.go buildPrompt() para usar
   prompts.Get(cfg.PromptLang, "rag_system") en vez del system prompt hardcoded.
5. Permite override por request: si el body de ChatCompletions incluye un
   campo "lang", úsalo en vez del default.
```
