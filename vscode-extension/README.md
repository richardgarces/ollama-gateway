# Copilot Local — VS Code Extension

Extensión VS Code que se conecta al gateway local (`/openai/v1/chat/completions`) vía HTTP con streaming SSE. Incluye un panel de chat Webview interactivo y fallback automático al binario `copilot-cli` si el servidor no está disponible.

## Funciones

| Comando | Atajo | Descripción |
|---------|-------|-------------|
| **Copilot Local: Send Selection** | `Cmd+Shift+L` | Envía la selección (o documento completo) al gateway y muestra la respuesta streaming en el Output Channel |
| **Copilot Local: Open Chat Panel** | `Cmd+Shift+K` | Abre un panel lateral de chat con UI interactiva |
| **Copilot Local: Quick Prompt** | - | Abre plantillas rápidas y las inserta en el input del chat usando la selección activa |
| **Copilot Local: Search Chat History** | - | Busca mensajes previos del chat en el workspace y resalta el resultado en el panel |
| **Copilot Local: Compare Models** | - | Compara la respuesta de dos modelos para el mismo prompt y muestra resultados lado a lado |
| **Copilot Local: Open Favorites** | - | Abre favoritos guardados para copiar, aplicar al editor o eliminar |
| **Copilot Local: Switch Workspace Profile** | - | Cambia entre perfiles por workspace (`fast`, `balanced`, `deep-analysis`) y actualiza modelo/idioma/temperatura |
| **Copilot Local: Clear Session State** | - | Limpia el estado persistido de sesión del chat (mensajes/modelo/sesión activa) |
| **Copilot Local: Explain Test Failure** | - | Analiza salida de tests fallidos y envía al chat hipótesis, plan de corrección y test de regresión sugerido |
| **Copilot Local: Reset Quality Alerts** | - | Reinicia contadores y limpia warnings de calidad en status bar |

## Configuración

| Setting | Default | Descripción |
|---------|---------|-------------|
| `copilotLocal.apiUrl` | `http://localhost:8081` | Base URL del gateway |
| `copilotLocal.model` | `local-rag` | Modelo por defecto |
| `copilotLocal.workspaceModel` | `` | Override de modelo por workspace |
| `copilotLocal.workspaceLang` | `` | Override de idioma de respuesta por workspace |
| `copilotLocal.workspaceTemperature` | `0.3` | Override de temperatura por workspace |
| `copilotLocal.workspaceActiveProfile` | `balanced` | Perfil activo por workspace |
| `copilotLocal.workspaceProfiles` | `{ fast, balanced, deep-analysis }` | Perfiles por workspace con `{model, lang, temperature}` |
| `copilotLocal.cliPath` | (vacío) | Ruta al binario `copilot-cli` (fallback) |
| `copilotLocal.jwtToken` | (vacío) | Token JWT para endpoints autenticados |
| `copilotLocal.voiceInputEnabled` | `false` | Habilita dictado por voz en el chat panel (si el Web Speech API está disponible) |
| `copilotLocal.quickPromptTemplates` | `{}` | Plantillas custom (JSON), ejemplo: `{ "review": "Review this code:\n{{selection}}" }` |
| `copilotLocal.qualityAlertsEnabled` | `true` | Habilita alertas locales de calidad |
| `copilotLocal.qualityLatencyThresholdMs` | `8000` | Umbral de latencia promedio para disparar warning |
| `copilotLocal.qualityConsecutiveErrorsThreshold` | `3` | Umbral de errores consecutivos para warning |

## Instalación (desarrollo)

1. Abre el workspace raíz en VS Code.
2. Navega a la carpeta `vscode-extension/`.
3. Presiona **F5** para lanzar el Extension Host.
4. En la ventana nueva, usa `Cmd+Shift+P` → "Copilot Local: Open Chat Panel".

## Arquitectura

```
┌────────────────────────┐
│   VS Code Extension    │
│  (extension.js)        │
│                        │
│  ┌──────────────────┐  │
│  │ Chat Webview     │  │    HTTP POST + SSE
│  │ (HTML/JS panel)  │──┼──────────────────────┐
│  └──────────────────┘  │                      │
│                        │                      ▼
│  ┌──────────────────┐  │    ┌──────────────────────────┐
│  │ Output Channel   │──┼───▶│ POST /openai/v1/         │
│  │ (Send Selection) │  │    │   chat/completions       │
│  └──────────────────┘  │    │   stream: true            │
│                        │    └──────────────────────────┘
│  ┌──────────────────┐  │                      │
│  │ CLI Fallback     │──┼───▶ copilot-cli      │
│  └──────────────────┘  │    (si HTTP falla)   │
└────────────────────────┘
```
