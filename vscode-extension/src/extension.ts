import * as path from 'path';
import * as vscode from 'vscode';
import { LanguageClient, LanguageClientOptions, ServerOptions, TransportKind } from 'vscode-languageclient/node';
import { CopilotWorkingSetViewProvider } from './workingSetView';
import { JupyterNotebookSerializer } from './jupyterNotebookSerializer';

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
      // Integración nativa con API de Chat VS Code
      if ((vscode as any).chat && typeof (vscode as any).chat.createChatParticipant === 'function') {
        const handler = async (request: any, _ctx: any, stream: any, _token: any) => {
          // Usar la lógica JS existente para obtener la respuesta
          try {
            // eslint-disable-next-line @typescript-eslint/no-var-requires
            const legacy = require('../extension.js');
            if (typeof legacy.requestChatCompletion === 'function') {
              const res = await legacy.requestChatCompletion(request.prompt, undefined, undefined);
              if (res && typeof res === 'string') {
                stream.markdown(res);
              } else if (res && res.completion) {
                stream.markdown(res.completion);
              } else {
                stream.markdown('No response.');
              }
            } else {
              stream.markdown('Chat handler not available.');
            }
          } catch (err) {
            stream.markdown('Error: ' + (err?.message || String(err)));
          }
        };
        const chatParticipant = (vscode as any).chat.createChatParticipant('copilot-local', handler);
        chatParticipant.iconPath = vscode.Uri.joinPath(context.extensionUri, 'resources/copilot-local.svg');
        chatParticipant.label = 'Copilot Local';
        context.subscriptions.push(chatParticipant);
      }
    // Registrar notebookSerializer para Jupyter
    if ((vscode as any).notebook && (vscode as any).notebook.registerNotebookSerializer) {
      context.subscriptions.push(
        (vscode as any).notebook.registerNotebookSerializer(
          'jupyter-notebook',
          new JupyterNotebookSerializer(),
          { transientOutputs: false, transientCellMetadata: {}, transientDocumentMetadata: {} }
        )
      );
    }
  const output = vscode.window.createOutputChannel('Copilot Local');
  context.subscriptions.push(output);

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
  // Registrar la nueva vista Working Set
  const workingSetProvider = new CopilotWorkingSetViewProvider(context);
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider(
      CopilotWorkingSetViewProvider.viewType,
      workingSetProvider
    )
  );
  // Escucha mensajes globales y reenvía a la webview
  vscode.window.onDidChangeActiveTextEditor(() => {
    // (Opcional: refrescar vista si cambia el editor)
  });
  // Recibe mensajes desde extension.js
  // Para recibir mensajes desde extension.js, usar un EventEmitter global o vscode.commands.registerCommand
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
