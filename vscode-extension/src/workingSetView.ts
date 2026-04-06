import * as vscode from 'vscode';

export class CopilotWorkingSetViewProvider implements vscode.WebviewViewProvider {
  public static readonly viewType = 'copilotLocal.workingSetView';
  private _view?: vscode.WebviewView;
  private _context: vscode.ExtensionContext;

  constructor(context: vscode.ExtensionContext) {
    this._context = context;
  }

  resolveWebviewView(webviewView: vscode.WebviewView) {
    this._view = webviewView;
    webviewView.webview.options = { enableScripts: true };
    webviewView.webview.html = this.getHtml();
    // TODO: Add message passing for file ops
  }

  getHtml(): string {
    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <style>
    body { font-family: var(--vscode-editor-font-family); margin: 0; padding: 0; }
    .header { font-weight: bold; padding: 8px; background: var(--vscode-sideBar-background); }
    .fileop-list { padding: 0 8px; }
    .fileop { border-bottom: 1px solid #eee; padding: 6px 0; }
    .fileop.accepted { background: #e6ffe6; }
    .fileop.rejected { background: #ffe6e6; }
    .fileop.pending { background: #fffbe6; }
    .fileop .meta { font-size: 11px; color: #888; }
    .fileop .actions { margin-top: 4px; }
    button { font-size: 11px; margin-right: 4px; }
  </style>
</head>
<body>
  <div class="header">Working Set / Timeline</div>
  <div id="fileop-list" class="fileop-list"></div>
  <script>
    const vscode = acquireVsCodeApi();
    window.addEventListener('message', event => {
      const { type, ops } = event.data;
      if (type === 'updateFileOps') renderFileOps(ops);
    });
    function renderFileOps(ops) {
      const el = document.getElementById('fileop-list');
      if (!el) return;
      el.innerHTML = '';
      for (const op of ops) {
        const div = document.createElement('div');
        div.className = 'fileop ' + op.status;
        div.innerHTML = '<div><b>' + op.type.toUpperCase() + '</b> <span class="meta">' + (op.path || '') + '</span></div>' +
          '<div class="meta">' + op.status + ' | Iter: ' + op.iteration + '</div>' +
          '<div class="actions">' +
            (op.status === 'pending' ? '<button onclick="vscode.postMessage({type:\'acceptFileOp\',opId:\'' + op.opId + '\'})">Aceptar</button><button onclick="vscode.postMessage({type:\'rejectFileOp\',opId:\'' + op.opId + '\'})">Rechazar</button>' : '') +
            '<button onclick="vscode.postMessage({type:\'showDiff\',opId:\'' + op.opId + '\'})">Ver Diff</button>' +
          '</div>';
        el.appendChild(div);
      }
    }
  </script>
</body>
</html>`;
  }
}
