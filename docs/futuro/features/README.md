# Características Nuevas Posibles

> Cada sección incluye un **prompt listo para Copilot** que puedes pegar directamente en el chat.

---

## 1. Sistema de Plugins / Tools para Agentes

Permitir que los agentes registren herramientas (tools) dinámicamente desde archivos YAML/JSON, en vez de tenerlas hardcoded en `agent.go`.

**Estado actual:** `AgentService.Run()` tiene `get_time` y `read_file` fijos en el código.

```
Prompt Copilot:
Refactoriza internal/services/agent.go para que las herramientas (tools) del agente
se carguen dinámicamente desde archivos YAML en un directorio configurable
(AGENT_TOOLS_DIR). Cada archivo YAML define: name, description, type (shell|http|function)
y parameters. Crea un ToolRegistry en internal/services/tool_registry.go que cargue
los archivos al iniciar, implemente la interfaz Tool{Name()string, Run(args map[string]string)(string,error)},
y se inyecte en AgentService. Mantén get_time y read_file como tools built-in.
Sigue Clean Architecture: no lógica de negocio en handlers.
```

---

## 2. Historial de Conversaciones Persistente

Guardar el historial de chat por usuario/sesión en MongoDB para contexto multi-turno.

**Estado actual:** Cada request a `/openai/v1/chat/completions` es stateless.

```
Prompt Copilot:
Implementa persistencia de historial de conversaciones en MongoDB. Crea:
1. internal/domain/conversation.go con structs Conversation{ID, UserID, Messages[], CreatedAt, UpdatedAt}
2. internal/services/conversation.go con ConversationService que tenga métodos
   Create, Append, GetByID, ListByUser (usa el driver go.mongodb.org/mongo-driver)
3. Modifica internal/handlers/openai.go ChatCompletions para aceptar un campo opcional
   "conversation_id" en el body. Si viene, carga mensajes previos y los antepone al prompt.
   Después de generar la respuesta, guarda el turno user+assistant en la conversación.
4. Agrega MONGO_URI a internal/config/config.go (default: mongodb://localhost:27017).
Inyecta dependencias vía constructor, no variables globales.
```

---

## 3. Multi-modelo con Routing Inteligente

Extender el `RouterService` para seleccionar modelo según análisis semántico del prompt (no solo keywords), con soporte para modelos remotos (OpenAI, Anthropic) como fallback.

**Estado actual:** `router.go` usa heurísticas simples (`strings.Contains`).

```
Prompt Copilot:
Mejora internal/services/router.go para implementar routing inteligente de modelos:
1. Reemplaza la heurística de strings.Contains por un clasificador basado en embeddings:
   genera embedding del prompt con OllamaService.GetEmbedding, compara con embeddings
   pre-calculados de categorías (code, creative, analysis, chat) usando cosine similarity.
2. Añade soporte para modelos remotos como fallback: agrega a Config los campos
   REMOTE_API_KEY y REMOTE_API_URL. Si el modelo local falla (timeout/error),
   reintenta con el remoto vía HTTP POST compatible con OpenAI API.
3. Registra la decisión de routing en los logs con el request_id del middleware.
Mantén la interfaz actual SelectModel(prompt string) string.
```

---

## 4. Code Actions: Aplicar Parches desde Respuestas del LLM

Extraer bloques de código (diffs/patches) de las respuestas del agente y aplicarlos al workspace.

**Estado actual:** Las respuestas se devuelven como texto plano, sin acción sobre archivos.

```
Prompt Copilot:
Implementa un sistema de code actions que parsee respuestas del LLM:
1. Crea internal/services/patch.go con PatchService que tenga:
   - ExtractCodeBlocks(response string) []CodeBlock (parsea bloques ```lang\n...```)
   - ExtractDiff(response string) []UnifiedDiff (parsea formato unified diff)
   - ApplyPatch(repoRoot string, diff UnifiedDiff) error (aplica cambios al archivo)
2. Crea internal/handlers/patch.go con endpoint POST /api/patch que reciba
   {response: string, apply: bool}. Si apply=true, aplica los diffs al REPO_ROOT.
3. Usa filepath.Abs() y valida que todos los paths estén dentro de REPO_ROOT
   para prevenir path traversal (regla de seguridad del proyecto).
4. Añade un endpoint GET /api/patch/preview que devuelva los diffs parseados sin aplicar.
```

---

## 5. Sistema de Perfiles de Usuario

Cada usuario puede tener preferencias almacenadas (modelo preferido, temperatura, system prompt personalizado) que se aplican automáticamente.

```
Prompt Copilot:
Implementa perfiles de usuario persistentes:
1. Crea internal/domain/profile.go con struct Profile{UserID, PreferredModel,
   Temperature float64, SystemPrompt, MaxTokens int, CreatedAt, UpdatedAt}.
2. Crea internal/services/profile.go con ProfileService (CRUD en MongoDB).
3. Modifica internal/middleware/auth.go para extraer el userID del JWT claims
   y añadirlo al context de la request.
4. Modifica internal/handlers/openai.go ChatCompletions para cargar el perfil
   del usuario autenticado y aplicar sus preferencias como defaults (si el
   request no las especifica explícitamente).
5. Crea endpoints CRUD: GET/PUT /api/profile (protegidos con JWT).
Inyecta ProfileService vía constructor en NewOpenAIHandler.
```

---

## 6. Modo Multi-Repositorio

Indexar y buscar en múltiples repositorios simultáneamente, con aislamiento de colecciones en Qdrant.

**Estado actual:** `REPO_ROOT` apunta a un solo directorio.

```
Prompt Copilot:
Extiende el sistema para soportar múltiples repositorios:
1. Modifica internal/config/config.go para aceptar REPO_ROOTS como lista
   separada por comas (ej: "/path/a,/path/b"). Mantén REPO_ROOT como alias
   del primer elemento para retrocompatibilidad.
2. Modifica internal/services/indexer.go para que IndexRepo() itere sobre
   todos los repos, usando una colección Qdrant separada por repo
   (nombre: "repo_<hash_del_path>").
3. Modifica internal/services/rag.go search() para buscar en todas las
   colecciones y mergear resultados por score.
4. Añade un query param opcional "repo" a POST /api/search para filtrar
   por repositorio específico.
5. Cada repo debe tener su propio .indexer_state.json (sufijado con hash del path).
```

---

## 7. Streaming Bidireccional con WebSockets

Añadir soporte WebSocket además de SSE para comunicación bidireccional en tiempo real.

```
Prompt Copilot:
Añade soporte WebSocket al gateway para streaming bidireccional:
1. Agrega la dependencia github.com/gorilla/websocket.
2. Crea internal/handlers/ws.go con un handler WS en /ws/chat que:
   - Upgrade la conexión HTTP a WebSocket.
   - Lea mensajes JSON del cliente con formato {model, messages[], stream}.
   - Llame a rag.StreamGenerateWithContext y envíe chunks como mensajes WS.
   - Soporte mensajes de control: {type:"ping"}, {type:"cancel"}.
3. Registra la ruta en internal/server/server.go (sin middleware de rate limit).
4. Actualiza vscode-extension/extension.js para usar WebSocket cuando esté
   disponible, con fallback automático a HTTP SSE.
No expongas el WebSocket sin autenticación: valida JWT del query param "token".
```

---

## 8. Sistema de Caché de Respuestas RAG

Cachear respuestas completas de RAG para prompts idénticos o similares.

```
Prompt Copilot:
Implementa un caché de respuestas RAG en internal/services/rag.go:
1. Crea un struct ResponseCache similar al EmbeddingCache de ollama.go
   (LRU + TTL), con clave = hash SHA256 del prompt normalizado.
2. En GenerateWithContext, antes de llamar a Ollama, busca en el caché.
   Si hay hit y el TTL no expiró, devuelve la respuesta cacheada.
3. En StreamGenerateWithContext, si hay hit, emite la respuesta cacheada
   chunk a chunk (simulando streaming) para mantener la UX consistente.
4. Agrega config vars RAG_CACHE_TTL_SECONDS (default: 1800) y
   RAG_CACHE_MAX_ENTRIES (default: 500) a internal/config/config.go.
5. Invalida entradas del caché cuando el indexer detecte cambios en archivos
   (hook desde IndexerService).
```

---

## 9. Análisis de Código con AST (Abstract Syntax Tree)

Usar el AST de Go para analizar código de forma estructurada en vez de enviar texto plano al LLM.

```
Prompt Copilot:
Crea internal/services/ast_analyzer.go con un ASTAnalyzer que:
1. Use go/parser y go/ast para parsear archivos .go del REPO_ROOT.
2. Extraiga un resumen estructurado: funciones (nombre, params, returns),
   structs (campos y tags), interfaces, imports y comentarios doc.
3. Genere un mapa de dependencias entre paquetes del proyecto.
4. Expón un método AnalyzeFile(path string) (*FileAnalysis, error) y
   AnalyzePackage(pkgPath string) (*PackageAnalysis, error).
5. Integra con el indexer: en vez de indexar texto plano del archivo,
   indexa el resumen estructurado como payload en Qdrant para que las
   búsquedas RAG retornen contexto más relevante.
Valida paths con filepath.Abs() dentro de REPO_ROOT.
```

---

## 10. Dashboard Web de Monitoreo

Una UI web simple para visualizar métricas, estado del indexer y logs en tiempo real.

```
Prompt Copilot:
Crea un dashboard web embebido en el gateway:
1. Crea internal/handlers/dashboard.go que sirva una SPA mínima en /dashboard.
   El HTML/CSS/JS va inline en el handler (como getChatPanelHTML en la extensión VS Code).
2. La UI debe mostrar:
   - Estado del servidor (uptime, versión, config activa).
   - Métricas en tiempo real: requests/min, latencia p50/p95, errores (consume /metrics).
   - Estado del indexer: archivos indexados, último reindex, watcher activo (consume los endpoints /internal/index/*).
   - Logs recientes vía SSE (crea un endpoint /internal/logs/stream que emita logs del server).
3. Estiliza con CSS embebido (tema oscuro, responsivo).
4. No requiere autenticación (es un endpoint interno). Protege el path con
   un middleware que solo acepte conexiones desde localhost.
```
