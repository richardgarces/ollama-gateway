import * as path from 'path';
import * as vscode from 'vscode';
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind } from 'vscode-languageclient/node';

type LegacyModule = {
  activate?: (context: vscode.ExtensionContext) => void | Promise<void>;
  deactivate?: () => void | Promise<void>;
};

let legacy: LegacyModule | undefined;

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

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  const output = vscode.window.createOutputChannel('Copilot Local');
  context.subscriptions.push(output);

  const openChatStable = vscode.commands.registerCommand('copilot-local.openChat', async () => {
    try {
      await vscode.commands.executeCommand('copilot-local.openChatLegacy');
    } catch (err) {
      const details = err instanceof Error ? (err.stack || err.message) : String(err);
      output.appendLine('[openChat][stable-fallback] ' + details);
      const panel = vscode.window.createWebviewPanel(
        'copilotLocalChatFallback',
        'Copilot Local Chat',
        vscode.ViewColumn.Beside,
        { enableScripts: false, retainContextWhenHidden: true },
      );
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
    legacy = require('../extension.js') as LegacyModule;
    if (typeof legacy.activate === 'function') {
      await legacy.activate(context);
    }
  } catch (err) {
    const details = err instanceof Error ? (err.stack || err.message) : String(err);
    output.appendLine('[activate][legacy-error] ' + details);
    void vscode.window.showWarningMessage('Copilot Local loaded in safe mode (legacy module failed).');
  }
  try {
    startLsp(context);
  } catch (err) {
    const details = err instanceof Error ? (err.stack || err.message) : String(err);
    output.appendLine('[activate][lsp-error] ' + details);
    void vscode.window.showWarningMessage('Copilot Local: LSP no pudo iniciar (chat sigue disponible).');
  }
}

export async function deactivate(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
  if (legacy && typeof legacy.deactivate === 'function') {
    await legacy.deactivate();
  }
}
