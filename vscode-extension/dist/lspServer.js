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
const http = __importStar(require("http"));
const https = __importStar(require("https"));
const url_1 = require("url");
const node_1 = require("vscode-languageserver/node");
const vscode_languageserver_textdocument_1 = require("vscode-languageserver-textdocument");
const connection = (0, node_1.createConnection)(node_1.ProposedFeatures.all);
const documents = new node_1.TextDocuments(vscode_languageserver_textdocument_1.TextDocument);
let apiUrl = 'http://localhost:8081';
let model = 'local-rag';
connection.onInitialize((params) => {
    const opts = (params.initializationOptions || {});
    apiUrl = String(opts.apiUrl || apiUrl).replace(/\/+$/, '');
    model = String(opts.model || model).trim() || model;
    return {
        capabilities: {
            textDocumentSync: node_1.TextDocumentSyncKind.Incremental,
            completionProvider: {
                resolveProvider: false,
                triggerCharacters: ['.', '(', ',', ':', '{', '[', '<', ' ', '\n', '=', '/', '@', '#'],
            },
        },
    };
});
async function postJSON(endpoint, payload) {
    const url = new url_1.URL(apiUrl + endpoint);
    const lib = url.protocol === 'https:' ? https : http;
    const body = JSON.stringify(payload || {});
    return await new Promise((resolve, reject) => {
        const req = lib.request(url, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(body),
            },
        }, (res) => {
            let raw = '';
            res.on('data', (d) => { raw += d.toString(); });
            res.on('end', () => {
                const status = res.statusCode || 0;
                let parsed = {};
                try {
                    parsed = raw ? JSON.parse(raw) : {};
                }
                catch {
                    parsed = { raw };
                }
                if (status < 200 || status >= 300) {
                    reject(new Error(parsed?.error || ('HTTP ' + status)));
                    return;
                }
                resolve(parsed || {});
            });
        });
        req.on('error', reject);
        req.write(body);
        req.end();
    });
}
// Language normalization map — expanded language support
const languageMap = {
    javascript: 'javascript', javascriptreact: 'javascript', typescript: 'typescript', typescriptreact: 'typescript',
    python: 'python', go: 'go', java: 'java', rust: 'rust', c: 'c', cpp: 'cpp', csharp: 'csharp',
    ruby: 'ruby', php: 'php', swift: 'swift', kotlin: 'kotlin', scala: 'scala', lua: 'lua',
    perl: 'perl', r: 'r', dart: 'dart', elixir: 'elixir', haskell: 'haskell', clojure: 'clojure',
    shellscript: 'bash', bash: 'bash', zsh: 'bash', fish: 'bash', powershell: 'powershell',
    sql: 'sql', html: 'html', css: 'css', scss: 'css', less: 'css', json: 'json', jsonc: 'json',
    yaml: 'yaml', toml: 'toml', xml: 'xml', markdown: 'markdown', dockerfile: 'dockerfile',
    makefile: 'makefile', cmake: 'cmake', groovy: 'groovy', vue: 'vue', svelte: 'svelte',
    zig: 'zig', nim: 'nim', ocaml: 'ocaml', fsharp: 'fsharp', erlang: 'erlang',
    terraform: 'terraform', hcl: 'hcl', proto: 'protobuf', graphql: 'graphql',
};
function normalizeLanguage(id) {
    return languageMap[id.toLowerCase()] || id.toLowerCase() || 'unknown';
}
connection.onCompletion(async (params) => {
    const doc = documents.get(params.textDocument.uri);
    if (!doc)
        return [];
    const text = doc.getText();
    const offset = doc.offsetAt(params.position);
    const startOffset = Math.max(0, offset - 4000);
    const endOffset = Math.min(text.length, offset + 4000);
    const prefix = text.slice(startOffset, offset);
    const suffix = text.slice(offset, endOffset);
    const language = normalizeLanguage(doc.languageId);
    try {
        const res = await postJSON('/complete', {
            model,
            prefix,
            suffix,
            language,
            num_predict: 120,
        });
        const completion = String(res?.completion || '').trim();
        if (!completion)
            return [];
        return [{
                label: completion.split('\n')[0].slice(0, 80) || 'completion',
                kind: node_1.CompletionItemKind.Text,
                insertText: completion,
                detail: 'Copilot Local LSP',
            }];
    }
    catch (err) {
        connection.console.error('completion error: ' + String(err));
        return [];
    }
});
documents.listen(connection);
connection.listen();
