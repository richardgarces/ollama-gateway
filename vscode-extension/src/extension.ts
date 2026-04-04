import * as path from 'path';
import * as vscode from 'vscode';
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind } from 'vscode-languageclient/node';

// Reuse existing JS extension features while migrating to TypeScript entrypoint.
// eslint-disable-next-line @typescript-eslint/no-var-requires
const legacy = require('../extension.js');

let client: LanguageClient | undefined;

function startLsp(context: vscode.ExtensionContext): void {
  const serverModule = context.asAbsolutePath(path.join('dist', 'lspServer.js'));
  const serverOptions: ServerOptions = {
    run: { module: serverModule, transport: TransportKind.ipc },
    debug: { module: serverModule, transport: TransportKind.ipc },
  };

  const cfg = vscode.workspace.getConfiguration('copilotLocal');
  const clientOptions: LanguageClientOptions = {
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

  client = new LanguageClient('copilotLocalLsp', 'Copilot Local LSP', serverOptions, clientOptions);
  client.start();
  context.subscriptions.push({ dispose: () => { void client?.stop(); } });
}

export function activate(context: vscode.ExtensionContext): void {
  if (typeof legacy.activate === 'function') {
    legacy.activate(context);
  }
  startLsp(context);
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
  if (typeof legacy.deactivate === 'function') {
    await legacy.deactivate();
  }
}
