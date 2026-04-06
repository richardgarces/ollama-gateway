import * as vscode from 'vscode';

export class CopilotChatViewProvider implements vscode.WebviewViewProvider {
  public static readonly viewType = 'copilotLocal.chatView';
  private _view?: vscode.WebviewView;
  private _context: vscode.ExtensionContext;

  constructor(context: vscode.ExtensionContext) {
    this._context = context;
  }

  resolveWebviewView(webviewView: vscode.WebviewView) {
    this._view = webviewView;
    webviewView.webview.options = { enableScripts: true };
    webviewView.webview.html = this.getHtml();
    webviewView.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'sendPrompt') {
        const prompt = msg.prompt || '';
        let response = 'No response.';
        try {
          // eslint-disable-next-line @typescript-eslint/no-var-requires
          const legacy = require('../../extension.js');
          if (typeof legacy.requestChatCompletion === 'function') {
            const res = await legacy.requestChatCompletion(prompt);
            response = res?.choices?.[0]?.message?.content || 'No response.';
          }
        } catch (err) {
          response = 'Error: ' + (err?.message || String(err));
        }
        webviewView.webview.postMessage({ type: 'chatResponse', response });
      }
    });
  }

  getHtml(): string {
    return [
      '<!DOCTYPE html>',
      '<html lang="en">',
      '<head>',
      '  <meta charset="UTF-8">',
      '  <style>',
      '    body { font-family: var(--vscode-editor-font-family); margin: 0; padding: 0; }',
      '    .header { font-weight: bold; padding: 8px; background: var(--vscode-sideBar-background); }',
      '    .chat-container { padding: 8px; }',
      '    .chat-log { min-height: 120px; border: 1px solid #eee; padding: 8px; margin-bottom: 8px; background: #fafafa; }',
      '    .chat-input { width: 100%; box-sizing: border-box; }',
      '    button { margin-top: 4px; }',
      '  </style>',
      '</head>',
      '<body>',
      '  <div class="header">Copilot Local — Chat</div>',
      '  <div class="chat-container">',
      '    <div id="chat-log" class="chat-log"></div>',
      '    <textarea id="chat-input" class="chat-input" rows="2" placeholder="Escribe tu mensaje..."></textarea><br>',
      '    <button id="send-btn">Enviar</button>',
      '  </div>',
      '  <script>',
      '    const chatLog = document.getElementById("chat-log");',
      '    const chatInput = document.getElementById("chat-input");',
      '    const sendBtn = document.getElementById("send-btn");',
      '    sendBtn.onclick = function() {',
      '      const prompt = chatInput.value.trim();',
      '      if (!prompt) return;',
      '      chatLog.innerHTML += "<div><b>Tú:</b> " + prompt + "</div>";',
      '      window.acquireVsCodeApi().postMessage({ type: "sendPrompt", prompt: prompt });',
      '      chatInput.value = "";',
      '    };',
      '    window.addEventListener("message", function(event) {',
      '      const msg = event.data;',
      '      if (msg.type === "chatResponse") {',
      '        chatLog.innerHTML += "<div><b>Copilot:</b> " + msg.response + "</div>";',
      '        chatLog.scrollTop = chatLog.scrollHeight;',
      '      }',
      '    });',
      '  <\/script>',
      '</body>',
      '</html>'
    ].join('\n');
  }
}
