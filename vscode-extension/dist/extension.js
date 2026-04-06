"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const path = __importStar(require("path"));
const vscode = __importStar(require("vscode"));
const node_1 = require("vscode-languageclient/node");
const workingSetView_1 = require("./workingSetView");
const chatView_1 = require("./chatView");
const jupyterNotebookSerializer_1 = require("./jupyterNotebookSerializer");
let legacy;
let client;
function startLsp(context) {
    const serverModule = context.asAbsolutePath(path.join('dist', 'lspServer.js'));
    const serverOptions = {
        run: { module: serverModule, transport: node_1.TransportKind.ipc },
        debug: { module: serverModule, transport: node_1.TransportKind.ipc },
    };
    const cfg = vscode.workspace.getConfiguration('copilotLocal');
    const clientOptions = {
        documentSelector: [
            { scheme: 'file', language: 'go' },
            { scheme: 'file', language: 'typescript' },
            { scheme: 'file', language: 'javascript' },
            { scheme: 'file', language: 'python' },
            { scheme: 'file', language: 'sql' },
        ],
        initializationOptions: {
            apiUrl: String(cfg.get('apiUrl') || 'http://localhost:8081'),
            model: String(cfg.get('fimModel') || cfg.get('model') || 'local-rag'),
        },
    };
    client = new node_1.LanguageClient('copilotLocalLsp', 'Copilot Local LSP', serverOptions, clientOptions);
    client.start();
    context.subscriptions.push({ dispose: () => { void client?.stop(); } });
}
async function activate(context) {
    // Inline Completion (FIM) - ghost text
    const enableInline = vscode.workspace.getConfiguration('copilotLocal').get('inlineCompletions', true);
    if (enableInline && vscode.languages.registerInlineCompletionItemProvider) {
        const langs = ['go', 'typescript', 'javascript', 'python', 'sql'];
        for (const lang of langs) {
            context.subscriptions.push(vscode.languages.registerInlineCompletionItemProvider({ language: lang }, {
                async provideInlineCompletionItems(document, position, context_, token) {
                    // Solicita completions al LSP
                    const lsp = client;
                    if (!lsp)
                        return [];
                    const res = await lsp.sendRequest('textDocument/completion', {
                        textDocument: { uri: document.uri.toString() },
                        position: position,
                        context: context_,
                    });
                    if (!res || !Array.isArray(res))
                        return [];
                    return res.map((item) => ({
                        insertText: item.insertText || item.label,
                        range: new vscode.Range(position, position),
                        filterText: item.label,
                        command: undefined,
                    }));
                },
            }));
        }
    }
    // === Registro de comandos principales Copilot Local ===
    // 1. Send Selection
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.sendSelection', async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showWarningMessage('No hay editor activo.');
            return;
        }
        const selection = editor.selection.isEmpty ? editor.document.getText() : editor.document.getText(editor.selection);
        const output = vscode.window.createOutputChannel('Copilot Local');
        output.show(true);
        output.appendLine('⏳ Enviando selección al gateway...');
        try {
            // eslint-disable-next-line @typescript-eslint/no-var-requires
            const legacy = require('../extension.js');
            if (typeof legacy.requestChatCompletion === 'function') {
                const res = await legacy.requestChatCompletion(selection);
                output.appendLine(res?.choices?.[0]?.message?.content || 'Sin respuesta.');
            }
            else {
                output.appendLine('Handler legacy no disponible.');
            }
        }
        catch (err) {
            output.appendLine('Error: ' + (err?.message || String(err)));
        }
    }));
    // 2. Open Chat Panel
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openChat', async () => {
        await vscode.commands.executeCommand('workbench.view.extension.copilotLocal');
        await vscode.commands.executeCommand('copilotLocal.chatView.focus');
    }));
    // 3. Quick Prompt (plantillas rápidas)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.quickPrompt', async () => {
        const cfg = vscode.workspace.getConfiguration('copilotLocal');
        const templates = cfg.get('quickPromptTemplates') || {};
        const keys = Object.keys(templates);
        if (keys.length === 0) {
            vscode.window.showInformationMessage('No hay plantillas rápidas configuradas.');
            return;
        }
        const pick = await vscode.window.showQuickPick(keys, { placeHolder: 'Selecciona una plantilla' });
        if (!pick)
            return;
        const editor = vscode.window.activeTextEditor;
        if (!editor)
            return;
        const selection = editor.selection.isEmpty ? '' : editor.document.getText(editor.selection);
        const template = templates[pick].replace('{{selection}}', selection);
        await vscode.commands.executeCommand('copilot-local.openChat');
        // Enviar al input del chat (requiere integración JS adicional si se quiere autofill)
        vscode.window.showInformationMessage('Plantilla copiada al input del chat: ' + template);
    }));
    // 4. Search Chat History (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.searchHistory', async () => {
        vscode.window.showInformationMessage('Funcionalidad de búsqueda de historial aún no implementada.');
    }));
    // 5. Compare Models (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.compareModels', async () => {
        vscode.window.showInformationMessage('Funcionalidad de comparación de modelos aún no implementada.');
    }));
    // 6. Open Favorites (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openFavorites', async () => {
        vscode.window.showInformationMessage('Funcionalidad de favoritos aún no implementada.');
    }));
    // 7. Switch Workspace Profile (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.switchProfile', async () => {
        vscode.window.showInformationMessage('Cambio de perfil aún no implementado.');
    }));
    // 8. Clear Session State (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.clearSessionState', async () => {
        vscode.window.showInformationMessage('Limpieza de sesión aún no implementada.');
    }));
    // 9. Explain Test Failure (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.explainTestFailure', async () => {
        vscode.window.showInformationMessage('Explicación de fallo de test aún no implementada.');
    }));
    // 10. Reset Quality Alerts (dummy)
    context.subscriptions.push(vscode.commands.registerCommand('copilot-local.resetQualityAlerts', async () => {
        vscode.window.showInformationMessage('Reset de alertas de calidad aún no implementado.');
    }));
    // Integración nativa con API de Chat VS Code
    if (vscode.chat && typeof vscode.chat.createChatParticipant === 'function') {
        const handler = async (request, _ctx, stream, _token) => {
            // Usar la lógica JS existente para obtener la respuesta
            try {
                // eslint-disable-next-line @typescript-eslint/no-var-requires
                const legacy = require('../extension.js');
                if (typeof legacy.requestChatCompletion === 'function') {
                    const res = await legacy.requestChatCompletion(request.prompt, undefined, undefined);
                    if (res && typeof res === 'string') {
                        stream.markdown(res);
                    }
                    else if (res && res.completion) {
                        stream.markdown(res.completion);
                    }
                    else {
                        stream.markdown('No response.');
                    }
                }
                else {
                    stream.markdown('Chat handler not available.');
                }
            }
            catch (err) {
                stream.markdown('Error: ' + (err?.message || String(err)));
            }
        };
        const chatParticipant = vscode.chat.createChatParticipant('copilot-local', handler);
        chatParticipant.iconPath = vscode.Uri.joinPath(context.extensionUri, 'resources/copilot-local.svg');
        chatParticipant.label = 'Copilot Local';
        context.subscriptions.push(chatParticipant);
    }
    // Registrar notebookSerializer para Jupyter
    if (vscode.notebook && vscode.notebook.registerNotebookSerializer) {
        context.subscriptions.push(vscode.notebook.registerNotebookSerializer('jupyter-notebook', new jupyterNotebookSerializer_1.JupyterNotebookSerializer(), { transientOutputs: false, transientCellMetadata: {}, transientDocumentMetadata: {} }));
    }
    const output = vscode.window.createOutputChannel('Copilot Local');
    context.subscriptions.push(output);
    try {
        // Reuse existing JS extension features while migrating to TypeScript entrypoint.
        // eslint-disable-next-line @typescript-eslint/no-var-requires
        legacy = require('../extension.js');
        if (typeof legacy.activate === 'function') {
            await legacy.activate(context);
        }
    }
    catch (err) {
        const details = err instanceof Error ? (err.stack || err.message) : String(err);
        output.appendLine('[activate][legacy-error] ' + details);
        void vscode.window.showWarningMessage('Copilot Local loaded in safe mode (legacy module failed).');
    }
    try {
        startLsp(context);
    }
    catch (err) {
        const details = err instanceof Error ? (err.stack || err.message) : String(err);
        output.appendLine('[activate][lsp-error] ' + details);
        void vscode.window.showWarningMessage('Copilot Local: LSP no pudo iniciar (chat sigue disponible).');
    }
    // Registrar la nueva vista Working Set
    const workingSetProvider = new workingSetView_1.CopilotWorkingSetViewProvider(context);
    context.subscriptions.push(vscode.window.registerWebviewViewProvider(workingSetView_1.CopilotWorkingSetViewProvider.viewType, workingSetProvider));
    // Registrar el panel de chat básico
    const chatViewProvider = new chatView_1.CopilotChatViewProvider(context);
    context.subscriptions.push(vscode.window.registerWebviewViewProvider(chatView_1.CopilotChatViewProvider.viewType, chatViewProvider));
    // Escucha mensajes globales y reenvía a la webview
    vscode.window.onDidChangeActiveTextEditor(() => {
        // (Opcional: refrescar vista si cambia el editor)
    });
    // Recibe mensajes desde extension.js
    // Para recibir mensajes desde extension.js, usar un EventEmitter global o vscode.commands.registerCommand
}
async function deactivate() {
    if (client) {
        await client.stop();
        client = undefined;
    }
    if (legacy && typeof legacy.deactivate === 'function') {
        await legacy.deactivate();
    }
}
