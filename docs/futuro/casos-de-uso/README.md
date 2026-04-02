# Casos de Uso Posibles

> Escenarios concretos de uso del gateway con el **prompt de Copilot** necesario para implementar cada uno.

---

## 1. Code Review Automatizado en PRs

Revisar diffs de Pull Requests y generar comentarios con sugerencias de mejora.

```
Prompt Copilot:
Implementa un servicio de code review automatizado:
1. Crea internal/services/review.go con ReviewService que tenga:
   - ReviewDiff(diff string) ([]ReviewComment, error): recibe un unified diff,
     lo envía al LLM con system prompt "You are a senior code reviewer. Analyze
     this diff and return a JSON array of {file, line, severity, comment}."
     Parsea la respuesta JSON.
   - ReviewFile(path string) ([]ReviewComment, error): lee el archivo,
     lo envía para revisión completa.
2. Crea internal/domain/review.go con structs ReviewComment{File, Line int,
   Severity string, Comment string} y ReviewResult{Comments[], Summary}.
3. Crea internal/handlers/review.go con endpoints:
   - POST /api/review/diff {diff: string} → devuelve ReviewResult.
   - POST /api/review/file {path: string} → revisa un archivo del REPO_ROOT.
4. Valida que el path esté dentro de REPO_ROOT con filepath.Abs().
5. Integra con RAG: busca contexto relevante del repo para que el reviewer
   entienda las convenciones del proyecto antes de opinar.
```

---

## 2. Generación de Documentación Automática

Generar docstrings, README y API docs a partir del código fuente.

```
Prompt Copilot:
Implementa generación automática de documentación:
1. Crea internal/services/docgen.go con DocGenService que tenga:
   - GenerateDocForFile(path string) (string, error): lee el archivo Go,
     envía al LLM con prompt "Generate Go doc comments for all exported
     functions, types and methods in this file. Return the complete file
     with comments added." Devuelve el archivo con docs.
   - GenerateREADME(repoRoot string) (string, error): analiza los archivos
     principales y genera un README.md con: descripción, instalación, uso,
     API endpoints, configuración y arquitectura.
2. Crea internal/handlers/docgen.go con endpoints:
   - POST /api/docs/file {path: string} → devuelve el archivo documentado.
   - POST /api/docs/readme → genera un README basado en el repo actual.
3. En ambos endpoints, usa RAG para dar contexto del proyecto al LLM.
4. Valida paths con filepath.Abs() dentro de REPO_ROOT.
5. Añade un flag "apply: bool" que si es true, escriba el resultado
   directamente al disco (con backup del archivo original con sufijo .bak).
```

---

## 3. Debugging Asistido por IA

Analizar stack traces y logs de error para sugerir la causa raíz y posibles soluciones.

```
Prompt Copilot:
Implementa un asistente de debugging:
1. Crea internal/services/debugger.go con DebugService que tenga:
   - AnalyzeError(stackTrace string) (DebugAnalysis, error): envía el
     stack trace al LLM con system prompt "You are a debugging expert.
     Analyze this Go stack trace. Return JSON: {root_cause, explanation,
     suggested_fixes: [], related_files: []}".
   - AnalyzeLog(logLines string) (DebugAnalysis, error): analiza logs
     del servidor buscando patrones de error.
2. Crea internal/domain/debug.go con struct DebugAnalysis{RootCause,
   Explanation string, SuggestedFixes []string, RelatedFiles []string}.
3. Crea internal/handlers/debug.go con endpoints:
   - POST /api/debug/error {stack_trace: string}
   - POST /api/debug/log {log: string, lines: int}
4. Integra con RAG: busca en el índice del repo los archivos mencionados
   en el stack trace y pásalos como contexto adicional al LLM.
5. En la extensión VS Code, añade un comando copilot-local.debugError que
   tome la selección del terminal/output y la envíe a este endpoint,
   mostrando el resultado en el chat panel.
```

---

## 4. Migración de Código entre Lenguajes

Traducir código de un lenguaje a otro usando el LLM con contexto del proyecto.

```
Prompt Copilot:
Implementa un servicio de traducción de código:
1. Crea internal/services/translator.go con TranslatorService que tenga:
   - Translate(code, fromLang, toLang string) (string, error): envía al
     LLM con system prompt "Translate this {fromLang} code to idiomatic
     {toLang}. Preserve logic, add appropriate error handling for the
     target language, and include necessary imports."
   - TranslateFile(path, toLang string) (string, error): lee el archivo,
     detecta el lenguaje por extensión, y traduce.
2. Crea internal/handlers/translator.go con endpoint:
   - POST /api/translate {code, from, to} → devuelve código traducido.
   - POST /api/translate/file {path, to} → traduce un archivo del repo.
3. Integra con RAG: si traduce código del repo, busca contexto
   (interfaces, tipos) para mantener coherencia con el proyecto.
4. En la extensión VS Code, añade copilot-local.translateSelection que
   muestre un QuickPick para elegir el lenguaje destino y envíe la
   selección al endpoint, mostrando el resultado en un nuevo editor tab.
```

---

## 5. Generación de Tests Automatizada

Crear tests unitarios a partir del código fuente existente.

```
Prompt Copilot:
Implementa generación automática de unit tests:
1. Crea internal/services/testgen.go con TestGenService que tenga:
   - GenerateTests(code, lang string) (string, error): analiza el código
     y genera tests usando el framework de testing del lenguaje.
     System prompt: "Generate comprehensive unit tests for this {lang} code.
     Use subtests (t.Run) for Go. Include edge cases, error paths, and
     table-driven tests. Use testify/assert if the code imports it."
   - GenerateTestsForFile(path string) (string, error): lee el archivo
     y genera tests. Detecta el lenguaje por extensión.
2. Crea internal/handlers/testgen.go con endpoints:
   - POST /api/testgen {code, lang} → devuelve código de test generado.
   - POST /api/testgen/file {path} → genera tests para un archivo del repo.
3. Integra con RAG: busca otros test files del proyecto para copiar el
   estilo de testing existente (imports, helpers, patrones).
4. En la extensión VS Code, el comando copilot-local.addTests (del doc
   de deseables) debe usar este endpoint.
5. Si el body incluye "apply: true", escribe el archivo de test junto
   al archivo original (ej: service.go → service_test.go).
```

---

## 6. Asistente de Arquitectura

Analizar el proyecto completo y sugerir mejoras arquitectónicas, detectar code smells y violaciones de principios SOLID.

```
Prompt Copilot:
Implementa un asistente de arquitectura:
1. Crea internal/services/architect.go con ArchitectService que tenga:
   - AnalyzeProject() (ArchReport, error): usa el IndexerService para obtener
     la lista de archivos, lee los principales, y envía al LLM con prompt:
     "Analyze this Go project architecture. Evaluate: SOLID compliance,
     dependency direction, separation of concerns, error handling patterns,
     cyclomatic complexity indicators. Return JSON: {score_1_10, strengths[],
     weaknesses[], recommendations[], dependency_graph:{}}".
   - SuggestRefactor(path string) (string, error): analiza un archivo y
     sugiere refactorizaciones concretas con código.
2. Crea internal/domain/architect.go con structs ArchReport y Recommendation.
3. Crea internal/handlers/architect.go con endpoints:
   - GET /api/architect/analyze → análisis completo del proyecto.
   - POST /api/architect/refactor {path} → sugerencias para un archivo.
4. Integra con RAG para dar contexto completo del proyecto.
5. Limita el análisis completo a máximo 20 archivos para no sobrecargar
   el contexto del LLM. Prioriza archivos por tamaño y complejidad.
```

---

## 7. Chat Multiusuario con Sesiones Compartidas

Permitir que varios desarrolladores compartan una sesión de chat para pair programming asistido por IA.

```
Prompt Copilot:
Implementa sesiones de chat compartidas:
1. Crea internal/domain/session.go con struct ChatSession{ID, OwnerID,
   Participants []string, Messages []Message, CreatedAt, IsActive bool}.
2. Crea internal/services/session.go con SessionService que tenga:
   - Create(ownerID string) (*ChatSession, error)
   - Join(sessionID, userID string) error
   - AddMessage(sessionID string, msg Message) error
   - GetMessages(sessionID string, since time.Time) ([]Message, error)
3. Crea internal/handlers/session.go con endpoints:
   - POST /api/sessions → crear sesión, devuelve session_id.
   - POST /api/sessions/{id}/join → unirse a sesión.
   - GET /api/sessions/{id}/messages?since={timestamp} → long-polling para
     nuevos mensajes.
   - POST /api/sessions/{id}/chat → enviar mensaje (se procesa con RAG y
     la respuesta se agrega a la sesión para todos).
4. Todos los endpoints requieren JWT auth.
5. Almacena sesiones en MongoDB (o en memoria con sync.Map como MVP).
6. En la extensión VS Code, añade copilot-local.joinSession que pida
   un session_id y abra el chat panel conectado a esa sesión.
```

---

## 8. Notificaciones Proactivas de Seguridad

Analizar código recién indexado buscando vulnerabilidades comunes (OWASP Top 10).

```
Prompt Copilot:
Implementa análisis proactivo de seguridad:
1. Crea internal/services/security.go con SecurityService que tenga:
   - ScanFile(path string) ([]SecurityFinding, error): envía el código
     al LLM con system prompt "You are a security auditor. Scan this
     code for OWASP Top 10 vulnerabilities: injection, broken auth,
     sensitive data exposure, XXE, broken access control, misconfig,
     XSS, insecure deserialization, known vulnerabilities, insufficient
     logging. Return JSON: [{severity, category, line, description, fix}]."
   - ScanRepo() (SecurityReport, error): escanea los archivos principales
     del repo y genera un reporte consolidado.
2. Crea internal/domain/security.go con structs SecurityFinding y SecurityReport.
3. Crea internal/handlers/security.go con endpoints:
   - POST /api/security/scan/file {path} → escanea un archivo.
   - POST /api/security/scan/repo → escanea el repo completo.
4. Hook en el indexer: cuando se indexe un archivo nuevo o modificado,
   ejecuta ScanFile en background y loguea findings con severity >= "high".
5. Valida paths con filepath.Abs() dentro de REPO_ROOT.
```

---

## 9. Asistente de Base de Datos (SQL Generator)

Generar queries SQL, migraciones y schemas a partir de descripciones en lenguaje natural.

```
Prompt Copilot:
Implementa un asistente de base de datos:
1. Crea internal/services/sqlgen.go con SQLGenService que tenga:
   - GenerateQuery(description, dialect string) (string, error): genera
     SQL a partir de descripción natural. Dialectos: postgres, mysql, sqlite.
     System prompt: "Generate a {dialect} SQL query for: {description}.
     Return ONLY the SQL query, no explanations."
   - GenerateMigration(description, dialect string) (string, error):
     genera un script de migración (CREATE/ALTER TABLE).
   - ExplainQuery(sql string) (string, error): analiza un query SQL
     y explica qué hace, su complejidad y posibles optimizaciones.
2. Crea internal/handlers/sqlgen.go con endpoints:
   - POST /api/sql/query {description, dialect}
   - POST /api/sql/migration {description, dialect}
   - POST /api/sql/explain {sql}
3. Integra con RAG: si el proyecto tiene archivos .sql o structs con
   tags `db:` o `bson:`, inclúyelos como contexto para generar queries
   coherentes con el schema existente.
4. IMPORTANTE: nunca ejecutes las queries generadas. Solo devuelve el SQL
   como texto. Indica claramente en la respuesta que es código generado
   que debe ser revisado antes de ejecutar.
```

---

## 10. Pipeline de CI/CD Assisted (Generar Workflows)

Generar archivos de CI/CD (GitHub Actions, GitLab CI, Jenkinsfile) a partir del análisis del proyecto.

```
Prompt Copilot:
Implementa un generador de pipelines CI/CD:
1. Crea internal/services/cicd.go con CICDService que tenga:
   - GeneratePipeline(platform, repoRoot string) (string, error): analiza
     el proyecto (lenguaje, dependencias, Dockerfile, Makefile, tests) y
     genera un pipeline para la plataforma indicada.
     Plataformas: "github-actions", "gitlab-ci", "jenkins".
     System prompt: "Analyze this project structure and generate a CI/CD
     pipeline for {platform}. Include: lint, test, build, docker build,
     deploy stages. Use caching for dependencies."
   - OptimizePipeline(existing string, platform string) (string, error):
     toma un pipeline existente y sugiere optimizaciones.
2. Crea internal/handlers/cicd.go con endpoints:
   - POST /api/cicd/generate {platform} → genera pipeline.
   - POST /api/cicd/optimize {pipeline, platform} → optimiza existente.
3. Integra con RAG: lee Makefile, Dockerfile, go.mod, package.json del
   repo para entender el build process actual.
4. Incluye un flag "apply: true" que escriba el archivo directamente
   en la ubicación correcta (.github/workflows/ci.yml, .gitlab-ci.yml, etc.)
   Solo si la ruta está dentro de REPO_ROOT.
```

---

## 11. Documentación Interactiva (API Explorer)

Un endpoint que sirva una UI tipo Swagger/OpenAPI para explorar y probar la API del gateway.

```
Prompt Copilot:
Implementa un API explorer embebido:
1. Crea internal/handlers/api_explorer.go que sirva una SPA en /api-docs.
2. El HTML debe mostrar una lista de todos los endpoints del gateway con:
   - Method, Path, Description.
   - Body de ejemplo (JSON editable).
   - Botón "Try it" que ejecute el request desde el browser y muestre
     la respuesta con syntax highlighting.
3. Genera la lista de endpoints programáticamente: crea una función
   GetRouteDefinitions() []RouteDefinition en internal/server/server.go
   que retorne todas las rutas registradas con su método, path y descripción.
4. Para endpoints SSE, muestra la respuesta streaming en tiempo real
   usando EventSource del browser.
5. Incluye un campo para ingresar JWT token que se añada automáticamente
   como header Authorization: Bearer {token} en los requests protegidos.
6. Estiliza con tema oscuro consistente con el dashboard de monitoreo.
Solo accesible desde localhost (mismo middleware que el dashboard).
```

---

## 12. Asistente de Commits (Smart Commit Messages)

Generar mensajes de commit descriptivos basados en los cambios staged en git.

```
Prompt Copilot:
Implementa generación de commit messages:
1. Crea internal/services/commitgen.go con CommitGenService que tenga:
   - GenerateMessage(diff string) (string, error): analiza el diff y
     genera un mensaje de commit siguiendo Conventional Commits
     (feat/fix/refactor/docs/test/chore).
     System prompt: "Generate a concise git commit message following
     Conventional Commits format for this diff. Format: type(scope): description.
     Include a body with bullet points if the change is complex."
   - GenerateFromStaged(repoRoot string) (string, error): ejecuta
     git diff --cached en el repoRoot y pasa el resultado a GenerateMessage.
2. Crea internal/handlers/commitgen.go con endpoint:
   - POST /api/commit/message {diff} → devuelve mensaje sugerido.
   - POST /api/commit/staged → genera mensaje para cambios staged del repo.
3. SEGURIDAD: para GenerateFromStaged, ejecuta git como subprocess
   con exec.CommandContext y timeout de 5s. No concatenes argumentos
   con input del usuario. Valida que repoRoot esté dentro de REPO_ROOT.
4. En la extensión VS Code, añade copilot-local.commitMessage que ejecute
   git diff --cached localmente, lo envíe al endpoint, y abra el
   resultado en el Source Control input box.
```
