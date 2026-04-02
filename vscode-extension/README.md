# Copilot Local — VS Code Extension

Extensión VS Code que se conecta al gateway local (`/openai/v1/chat/completions`) vía HTTP con streaming SSE. Incluye un panel de chat Webview interactivo y fallback automático al binario `copilot-cli` si el servidor no está disponible.

## Funciones

| Comando | Atajo | Descripción |
|---------|-------|-------------|
| **Copilot Local: Send Selection** | `Cmd+Shift+L` | Envía la selección (o documento completo) al gateway y muestra la respuesta streaming en el Output Channel |
| **Copilot Local: Open Chat Panel** | `Cmd+Shift+K` | Abre un panel lateral de chat con UI interactiva |

## Configuración

| Setting | Default | Descripción |
|---------|---------|-------------|
| `copilotLocal.apiUrl` | `http://localhost:8081` | Base URL del gateway |
| `copilotLocal.model` | `local-rag` | Modelo por defecto |
| `copilotLocal.cliPath` | (vacío) | Ruta al binario `copilot-cli` (fallback) |
| `copilotLocal.jwtToken` | (vacío) | Token JWT para endpoints autenticados |

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
