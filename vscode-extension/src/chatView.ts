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
    const path = require('path');
    webviewView.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'sendPrompt') {
        const prompt = msg.prompt || '';
        const mode = msg.mode || 'qa';
        let response = '';
        let finished = false;
        try {
          const legacy = require(path.resolve(__dirname, '../extension.js'));
          // Preferir WebSocket, fallback HTTP, luego CLI
          const onChunk = (token: string) => {
            response += token;
            webviewView.webview.postMessage({ type: 'chatStream', token });
          };
          const onDone = (err?: Error) => {
            if (finished) return;
            finished = true;
            if (err) {
              webviewView.webview.postMessage({ type: 'chatResponse', response: 'Error: ' + (err.message || String(err)) });
            } else {
              webviewView.webview.postMessage({ type: 'chatResponse', response });
            }
          };
          if (typeof legacy.streamWS === 'function') {
            legacy.streamWS(prompt, undefined, onChunk, onDone);
          } else if (typeof legacy.streamHTTP === 'function') {
            legacy.streamHTTP(prompt, undefined, onChunk, onDone);
          } else if (typeof legacy.streamCLI === 'function') {
            legacy.streamCLI(prompt, undefined, onChunk, onDone);
          } else {
            throw new Error('No hay método de streaming disponible');
          }
        } catch (err) {
          webviewView.webview.postMessage({ type: 'chatResponse', response: 'Error: ' + (err?.message || String(err)) });
        }
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
      '    body { font-family: var(--vscode-editor-font-family); margin: 0; padding: 0; background: var(--vscode-editor-background,#1e1e1e); }',
      '    .header { font-weight: bold; padding: 12px 16px; background: var(--vscode-sideBar-background,#23272e); border-bottom: 1px solid #2226; font-size: 1.1em; }',
      '    .chat-container { padding: 0 0 12px 0; display: flex; flex-direction: column; height: 100%; }',
      '    .chat-log { flex: 1; min-height: 220px; max-height: 400px; overflow-y: auto; border: none; padding: 16px 12px 8px 12px; margin-bottom: 8px; background: var(--vscode-editorWidget-background,#23272e); border-radius: 12px; display: flex; flex-direction: column; gap: 0; }',
      '    .bubble-row { display: flex; align-items: flex-end; margin-bottom: 8px; }',
      '    .bubble.user { background: linear-gradient(90deg,#2563eb 80%,#60a5fa 100%); color: #fff; align-self: flex-end; margin-left: 48px; margin-right: 0; border-bottom-right-radius: 4px; box-shadow: 0 2px 8px #0002; }',
      '    .bubble.bot { background: #23272e; color: #e0e6f0; align-self: flex-start; margin-right: 48px; margin-left: 0; border-bottom-left-radius: 4px; box-shadow: 0 2px 8px #0002; border: 1px solid #3336; }',
      '    .bubble { margin: 0; padding: 12px 16px; border-radius: 16px; max-width: 80%; word-break: break-word; font-size: 15px; position: relative; }',
      '    .bubble pre, .bubble code { background: #181c22; color: #b5e0ff; border-radius: 8px; padding: 8px; font-size: 13px; }',
      '    .avatar { width: 32px; height: 32px; border-radius: 50%; background: #2563eb; color: #fff; display: flex; align-items: center; justify-content: center; font-weight: bold; font-size: 18px; margin: 0 8px; box-shadow: 0 1px 4px #0003; }',
      '    .avatar.bot { background: #23272e; color: #60a5fa; border: 1.5px solid #2563eb; }',
      '    .chat-input-row { display: flex; gap: 8px; align-items: flex-end; padding: 8px 12px 0 12px; background: none; }',
      '    .chat-input { flex: 1; width: 100%; box-sizing: border-box; border-radius: 8px; border: 1.5px solid #2563eb; padding: 10px; font-size: 15px; background: var(--vscode-input-background,#181c22); color: var(--vscode-input-foreground,#e0e6f0); }',
      '    .send-btn, .clear-btn { padding: 8px 20px; border-radius: 8px; border: none; background: #2563eb; color: #fff; font-weight: bold; cursor: pointer; font-size: 15px; box-shadow: 0 1px 4px #0002; }',
      '    .clear-btn { background: #aaa; margin-left: 8px; }',
      '    .send-btn:disabled { background: #bbb; cursor: not-allowed; }',
      '    ::selection { background: #2563eb44; }',
      '    @media (max-width: 600px) { .bubble { font-size: 13px; padding: 8px 8px; } .avatar { width: 24px; height: 24px; font-size: 13px; } }',
      '  </style>',
      '</head>',
      '<body>',
      '  <div class="header">Copilot Local — Chat',
      '    <span style="float:right; font-weight:normal; font-size:0.95em;">',
      '      <select id="mode-select" style="background:#23272e;color:#fff;border-radius:6px;padding:2px 8px;margin-left:8px;">',
      '        <option value="qa">Pregunta/Respuesta</option>',
      '        <option value="agent">Agente</option>',
      '      </select>',
      '    </span>',
      '  </div>',
      '  <div class="chat-container">',
      '    <div id="chat-log" class="chat-log"></div>',
      '    <div class="chat-input-row">',
      '      <textarea id="chat-input" class="chat-input" rows="2" placeholder="Escribe tu mensaje..."></textarea>',
      '      <button id="send-btn" class="send-btn">Enviar</button>',
      '      <button id="clear-btn" class="clear-btn">Limpiar</button>',
      '    </div>',
      '  </div>',
      '  <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>',
      '  <script>',
      '    const chatLog = document.getElementById("chat-log");',
      '    const chatInput = document.getElementById("chat-input");',
      '    const sendBtn = document.getElementById("send-btn");',
      '    const clearBtn = document.getElementById("clear-btn");',
      '    let history = [];',
      '    let chatMode = "qa";',
      '    document.addEventListener("DOMContentLoaded", function() {',
      '      const modeSelect = document.getElementById("mode-select");',
      '      if (modeSelect) {',
      '        modeSelect.addEventListener("change", function() {',
      '          chatMode = modeSelect.value;',
      '        });',
      '      }',
      '    });',
      '    function renderHistory() {',
      '      chatLog.innerHTML = history.map((msg, idx) => {',
      '        const isUser = msg.role === "user";',
      '        const avatar = isUser',
      '          ? `<div class=\"avatar\">Tú</div>`',
      '          : `<div class=\"avatar bot\">🤖</div>`;',
      '        const bubble = isUser',
      '          ? `<div class=\"bubble user\">${msg.content.replace(/</g, "&lt;")}</div>`',
      '          : `<div class=\"bubble bot\">${window.marked.parse(msg.content)}</div>`;',
      '        return `<div class=\"bubble-row\">${isUser ? bubble + avatar : avatar + bubble}</div>`;',
      '      }).join("");',
      '      chatLog.scrollTop = chatLog.scrollHeight;',
      '    }',
      '    let streamingBotIdx = null;',
      '    function sendPrompt() {',
      '      const prompt = chatInput.value.trim();',
      '      if (!prompt) return;',
      '      history.push({ role: "user", content: prompt, mode: chatMode });',
      '      streamingBotIdx = history.length;',
      '      history.push({ role: "bot", content: "", mode: chatMode });',
      '      renderHistory();',
      '      window.acquireVsCodeApi().postMessage({ type: "sendPrompt", prompt, mode: chatMode });',
      '      chatInput.value = "";',
      '      sendBtn.disabled = true;',
      '    }',
      '    sendBtn.onclick = sendPrompt;',
      '    chatInput.addEventListener("keydown", function(e) {',
      '      if (e.key === "Enter" && !e.shiftKey) {',
      '        e.preventDefault(); sendPrompt();',
      '      }',
      '    });',
      '    clearBtn.onclick = function() { history = []; renderHistory(); };',
      '    window.addEventListener("message", function(event) {',
      '      const msg = event.data;',
      '      if (msg.type === "chatStream" && streamingBotIdx !== null) {',
      '        history[streamingBotIdx].content += msg.token;',
      '        renderHistory();',
      '      }',
      '      if (msg.type === "chatResponse") {',
      '        streamingBotIdx = null;',
      '        if (msg.response && msg.response.length > 0) {',
      '          history[history.length - 1].content = msg.response;',
      '        }',
      '        renderHistory();',
      '        sendBtn.disabled = false;',
      '      }',
      '    });',
      '  <\/script>',
      '</body>',
      '</html>'
    ].join('\n');
  }
}
