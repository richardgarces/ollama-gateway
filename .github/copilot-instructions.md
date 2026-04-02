# Instrucciones para GitHub Copilot - Ollama SaaS Gateway

Este proyecto es un Gateway SaaS escrito en Go (1.24) para exponer Ollama interactuando con bases de datos vectoriales (Qdrant) implementando RAG, Agentes y perfiles.

Al sugerir, autocompletar o generar código para este proyecto, debes seguir ESTRICTAMENTE las siguientes reglas:

## 1. Arquitectura (Clean Architecture)
El proyecto utiliza una arquitectura limpia separada en capas dentro del directorio `api/`. **No escribas código monolítico.**
- **`cmd/server/main.go`**: Punto de entrada de la aplicación. Configura la inicialización y dependencias. No pongas lógica de negocio aquí.
- **`internal/handlers/`**: Capa de presentación HTTP. Solo se encarga de parsear el Body, extraer parámetros, llamar a los `services` correspondientes y devolver la respuesta. No debe contener lógica de negocio ni acceso directo a base de datos.
- **`internal/services/`**: Contiene toda la lógica de negocio. Toda comunicación con APIs externas (Ollama, Qdrant) se hace aquí.
- **`internal/domain/`**: Modelos, structs de Base de Datos y DTOs (Data Transfer Objects). Sin comportamiento.
- **`internal/middleware/`**: Middlewares globales y de enrutamiento (Auth, Logging, CORS).
- **`internal/config/`**: Configuración unificada mediante variables de entorno cruzada con carga estructurada (`.env`).
- **`pkg/`**: Paquetes de utilidades genéricas, como `httputil/response.go`.

## 2. Convenciones de Go y Enrutamiento
- Usa el enrutador estándar del proyecto: `gorilla/mux`.
- **Inyección de dependencias:** PROHIBIDO usar variables globales para mantener estado o instanciar clientes. Inyecta dependencias a través de constructores explícitos (ej. `func NewXxxService(dep *Dep) *XxxService`).
- **Manejo de Respuestas HTTP:** Utiliza siempre los helpers creados en `pkg/httputil/` (`WriteJSON` y `WriteError`) en lugar de escribir a `http.ResponseWriter` directamente usando `json.NewEncoder`.

## 3. Seguridad Estricta (Obligatorio)
- **Autenticación (JWT):** Todas las rutas bajo el path `/api/...` están protegidas. No sugieras bypasses o desactivación de autenticación "para pruebas locales".
- **Prevención de Path Traversal:** Si creas handlers o servicios que leen/modifican archivos locales (`os.ReadFile`, `os.WriteFile`), **SIEMPRE** usa `filepath.Abs()` y asegúrate de que el archivo resida obligatoriamente dentro del entorno restringido (`cfg.RepoRoot`).
- **Ejecución de Comandos:** No uses `exec.Command` pasando argumentos concatenados con `fmt.Sprintf` o similares sin un control estricto de saneamiento. Nunca pases respuestas crudas de LLMs directo a una shell.

## 4. Concurrencia
- Si sugieres procesamiento de archivos en lote (por ejemplo, analizar código), hazlo de manera paralela con `goroutines`.
- Siempre administra el ciclo de vida de las goroutines usando `sync.WaitGroup` o canales. No dejes goroutines huérfanas corriendo de fondo (ejemplo disponible en `internal/services/repo.go`).

## 5. Testing
- Los archivos de pruebas deben sufijarse correctamente `_test.go` y mantenerse junto a la abstracción que están probando o delegados al paquete para test de caja negra.
- Usa subtests (`t.Run`) para aislar los escenarios de éxito de los de error.
