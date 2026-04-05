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
    const output = vscode.window.createOutputChannel('Copilot Local');
    context.subscriptions.push(output);
    const openChatStable = vscode.commands.registerCommand('copilot-local.openChat', async () => {
        try {
            await vscode.commands.executeCommand('copilot-local.openChatLegacy');
        }
        catch (err) {
            const details = err instanceof Error ? (err.stack || err.message) : String(err);
            output.appendLine('[openChat][stable-fallback] ' + details);
            const panel = vscode.window.createWebviewPanel('copilotLocalChatFallback', 'Copilot Local Chat', vscode.ViewColumn.Beside, { enableScripts: false, retainContextWhenHidden: true });
            panel.webview.html = `<!doctype html><html><body style="font-family: var(--vscode-font-family); padding: 16px;">
<h3>Copilot Local - Safe Mode</h3>
<p>No se pudo abrir el chat legacy.</p>
<p>Revisa Output: <strong>Copilot Local</strong>.</p>
<pre style="white-space: pre-wrap;">${details.replace(/[<&>]/g, (c) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;' }[c] || c))}</pre>
</body></html>`;
        }
    });
    context.subscriptions.push(openChatStable);
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
