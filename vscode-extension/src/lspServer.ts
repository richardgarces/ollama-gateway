import * as http from 'http';
import * as https from 'https';
import { URL } from 'url';
import {
  createConnection,
  ProposedFeatures,
  InitializeParams,
  InitializeResult,
  TextDocuments,
  CompletionItem,
  CompletionItemKind,
  TextDocumentSyncKind,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

type InitOpts = { apiUrl?: string; model?: string };

const connection = createConnection(ProposedFeatures.all);
const documents: TextDocuments<TextDocument> = new TextDocuments(TextDocument);

let apiUrl = 'http://localhost:8081';
let model = 'local-rag';

connection.onInitialize((params: InitializeParams): InitializeResult => {
  const opts = (params.initializationOptions || {}) as InitOpts;
  apiUrl = String(opts.apiUrl || apiUrl).replace(/\/+$/, '');
  model = String(opts.model || model).trim() || model;

  return {
    capabilities: {
      textDocumentSync: TextDocumentSyncKind.Incremental,
      completionProvider: { resolveProvider: false, triggerCharacters: ['.', '(', ','] },
    },
  };
});

async function postJSON(endpoint: string, payload: unknown): Promise<any> {
  const url = new URL(apiUrl + endpoint);
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
        let parsed: any = {};
        try { parsed = raw ? JSON.parse(raw) : {}; } catch { parsed = { raw }; }
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

connection.onCompletion(async (params): Promise<CompletionItem[]> => {
  const doc = documents.get(params.textDocument.uri);
  if (!doc) return [];

  const text = doc.getText();
  const offset = doc.offsetAt(params.position);
  const startOffset = Math.max(0, offset - 4000);
  const endOffset = Math.min(text.length, offset + 4000);

  const prefix = text.slice(startOffset, offset);
  const suffix = text.slice(offset, endOffset);

  try {
    const res = await postJSON('/complete', {
      model,
      prefix,
      suffix,
      language: doc.languageId,
      num_predict: 120,
    });

    const completion = String(res?.completion || '').trim();
    if (!completion) return [];

    return [{
      label: completion.split('\n')[0].slice(0, 80) || 'completion',
      kind: CompletionItemKind.Text,
      insertText: completion,
      detail: 'Copilot Local LSP',
    }];
  } catch (err) {
    connection.console.error('completion error: ' + String(err));
    return [];
  }
});

documents.listen(connection);
connection.listen();
