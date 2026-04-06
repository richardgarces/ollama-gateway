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
