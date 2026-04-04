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
// Reuse existing JS extension features while migrating to TypeScript entrypoint.
// eslint-disable-next-line @typescript-eslint/no-var-requires
const legacy = require('../extension.js');
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
function activate(context) {
    if (typeof legacy.activate === 'function') {
        legacy.activate(context);
    }
    startLsp(context);
}
async function deactivate() {
    if (client) {
        await client.stop();
        client = undefined;
    }
    if (typeof legacy.deactivate === 'function') {
        await legacy.deactivate();
    }
}
