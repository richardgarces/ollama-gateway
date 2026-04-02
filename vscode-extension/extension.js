const vscode = require('vscode');
const http = require('http');
const https = require('https');
const { spawn } = require('child_process');
const path = require('path');

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getConfig() {
  const cfg = vscode.workspace.getConfiguration('copilotLocal');
  return {
    apiUrl: (cfg.get('apiUrl') || 'http://localhost:8081').replace(/\/+$/, ''),
    model: cfg.get('model') || 'local-rag',
    cliPath: cfg.get('cliPath') || '',
    jwtToken: cfg.get('jwtToken') || '',
  };
}

/**
 * Stream a chat completion via HTTP SSE.
 * @param {string} prompt
 * @param {(chunk: string) => void} onChunk  called per content delta
 * @param {(err?: Error) => void}   onDone   called when complete
 * @param {AbortController} [abortCtl]
 */
function streamHTTP(prompt, onChunk, onDone, abortCtl) {
  const { apiUrl, model, jwtToken } = getConfig();
  const url = new URL(apiUrl + '/openai/v1/chat/completions');
  const lib = url.protocol === 'https:' ? https : http;

  const body = JSON.stringify({
    model,
    messages: [{ role: 'user', content: prompt }],
    stream: true,
  });

  const headers = { 'Content-Type': 'application/json' };
  if (jwtToken) {
    headers['Authorization'] = 'Bearer ' + jwtToken;
  }

  const req = lib.request(url, { method: 'POST', headers }, (res) => {
    let buf = '';
    res.on('data', (raw) => {
      buf += raw.toString();
      const lines = buf.split('\n');
      buf = lines.pop() || '';
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || !trimmed.startsWith('data:')) continue;
        const payload = trimmed.slice(5).trim();
        if (payload === '[DONE]') { onDone(); return; }
        try {
          const obj = JSON.parse(payload);
          const delta = obj?.choices?.[0]?.delta;
          if (delta?.content) {
            onChunk(delta.content);
          }
        } catch { /* ignore malformed lines */ }
      }
    });
    res.on('end', () => onDone());
    res.on('error', (e) => onDone(e));
  });

  req.on('error', (e) => onDone(e));
  if (abortCtl) {
    abortCtl.signal.addEventListener('abort', () => req.destroy());
  }
  req.write(body);
  req.end();
}

/**
 * Fallback: invoke copilot-cli binary via spawn.
 */
function streamCLI(prompt, onChunk, onDone) {
  const { model, cliPath } = getConfig();
  const resolved = cliPath || path.join(
    vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || '.',
    'api', 'bin', 'copilot-cli',
  );
  const proc = spawn(resolved, ['--model', model, '--prompt', prompt], {
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  proc.stdout.on('data', (d) => onChunk(d.toString()));
  proc.stderr.on('data', (d) => onChunk(d.toString()));
  proc.on('close', () => onDone());
  proc.on('error', (e) => onDone(e));
}

// ---------------------------------------------------------------------------
// Chat Panel (Webview)
// ---------------------------------------------------------------------------

function getChatPanelHTML() {
  return /* html */`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<style>
  :root { --bg:#1e1e1e; --fg:#d4d4d4; --accent:#569cd6; --input-bg:#252526; --border:#3c3c3c; }
  * { box-sizing:border-box; margin:0; padding:0; }
  body { font-family: var(--vscode-font-family, 'Segoe UI', sans-serif); background:var(--bg); color:var(--fg); height:100vh; display:flex; flex-direction:column; }
  #messages { flex:1; overflow-y:auto; padding:12px; }
  .msg { margin-bottom:10px; line-height:1.5; white-space:pre-wrap; word-wrap:break-word; }
  .msg.user { color:var(--accent); }
  .msg.assistant { color:var(--fg); }
  .msg .role { font-weight:bold; margin-right:6px; }
  #inputRow { display:flex; border-top:1px solid var(--border); padding:8px; gap:6px; }
  #prompt { flex:1; background:var(--input-bg); color:var(--fg); border:1px solid var(--border); border-radius:4px; padding:6px 10px; font-size:13px; resize:none; min-height:36px; max-height:120px; }
  #prompt:focus { outline:none; border-color:var(--accent); }
  button { background:var(--accent); color:#fff; border:none; border-radius:4px; padding:6px 14px; cursor:pointer; font-size:13px; }
  button:hover { opacity:0.85; }
  button:disabled { opacity:0.4; cursor:default; }
</style>
</head>
<body>
  <div id="messages"></div>
  <div id="inputRow">
    <textarea id="prompt" rows="1" placeholder="Ask something..."></textarea>
    <button id="send">Send</button>
  </div>
<script>
  const vscode = acquireVsCodeApi();
  const messagesEl = document.getElementById('messages');
  const promptEl = document.getElementById('prompt');
  const sendBtn = document.getElementById('send');
  let pendingEl = null;

  function addMsg(role, text) {
    const d = document.createElement('div');
    d.className = 'msg ' + role;
    const r = document.createElement('span');
    r.className = 'role';
    r.textContent = role === 'user' ? 'You:' : 'AI:';
    d.appendChild(r);
    d.appendChild(document.createTextNode(text));
    messagesEl.appendChild(d);
    messagesEl.scrollTop = messagesEl.scrollHeight;
    return d;
  }

  function startAssistant() {
    pendingEl = document.createElement('div');
    pendingEl.className = 'msg assistant';
    const r = document.createElement('span');
    r.className = 'role';
    r.textContent = 'AI:';
    pendingEl.appendChild(r);
    pendingEl.appendChild(document.createTextNode(''));
    messagesEl.appendChild(pendingEl);
    return pendingEl;
  }

  sendBtn.addEventListener('click', send);
  promptEl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
  });

  function send() {
    const text = promptEl.value.trim();
    if (!text) return;
    addMsg('user', text);
    promptEl.value = '';
    sendBtn.disabled = true;
    startAssistant();
    vscode.postMessage({ type: 'chat', text });
  }

  window.addEventListener('message', (e) => {
    const msg = e.data;
    if (msg.type === 'chunk' && pendingEl) {
      pendingEl.lastChild.textContent += msg.text;
      messagesEl.scrollTop = messagesEl.scrollHeight;
    } else if (msg.type === 'done') {
      pendingEl = null;
      sendBtn.disabled = false;
    } else if (msg.type === 'error') {
      if (pendingEl) { pendingEl.lastChild.textContent += '\\n[Error: ' + msg.text + ']'; }
      pendingEl = null;
      sendBtn.disabled = false;
    } else if (msg.type === 'prefill') {
      promptEl.value = msg.text;
      promptEl.focus();
    }
  });
</script>
</body>
</html>`;
}

// ---------------------------------------------------------------------------
// Activation
// ---------------------------------------------------------------------------

function activate(context) {
  const output = vscode.window.createOutputChannel('Copilot Local');
  let chatPanel = null;

  // --- Send Selection command (streams to output channel, HTTP first → CLI fallback) ---
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.sendSelection', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) { vscode.window.showInformationMessage('No active editor'); return; }

      const text = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection);
      if (!text || !text.trim()) { vscode.window.showInformationMessage('Nothing to send'); return; }

      output.show(true);
      output.appendLine('--- Request ---');

      const done = (err) => {
        if (err) output.appendLine('\n[Error] ' + err.message);
        output.appendLine('\n--- Done ---');
      };

      // Try HTTP first
      streamHTTP(text, (chunk) => output.append(chunk), (err) => {
        if (err) {
          output.appendLine('[HTTP failed, falling back to CLI] ' + err.message);
          streamCLI(text, (c) => output.append(c), done);
        } else {
          done();
        }
      });
    }),
  );

  // --- Open Chat Panel command ---
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.openChat', () => {
      if (chatPanel) { chatPanel.reveal(); return; }

      chatPanel = vscode.window.createWebviewPanel(
        'copilotLocalChat',
        'Copilot Local Chat',
        vscode.ViewColumn.Beside,
        { enableScripts: true, retainContextWhenHidden: true },
      );
      chatPanel.webview.html = getChatPanelHTML();

      chatPanel.webview.onDidReceiveMessage((msg) => {
        if (msg.type === 'chat') {
          streamHTTP(
            msg.text,
            (chunk) => chatPanel?.webview.postMessage({ type: 'chunk', text: chunk }),
            (err) => {
              if (err) {
                // fallback to CLI
                streamCLI(
                  msg.text,
                  (c) => chatPanel?.webview.postMessage({ type: 'chunk', text: c }),
                  (e) => {
                    if (e) chatPanel?.webview.postMessage({ type: 'error', text: e.message });
                    else chatPanel?.webview.postMessage({ type: 'done' });
                  },
                );
              } else {
                chatPanel?.webview.postMessage({ type: 'done' });
              }
            },
          );
        }
      });

      chatPanel.onDidDispose(() => { chatPanel = null; });
    }),
  );

  // If user opens chat and has a selection, pre-fill the prompt
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.sendSelectionToChat', () => {
      const editor = vscode.window.activeTextEditor;
      const text = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection) || '';
      vscode.commands.executeCommand('copilot-local.openChat').then(() => {
        if (chatPanel && text.trim()) {
          chatPanel.webview.postMessage({ type: 'prefill', text });
        }
      });
    }),
  );
}

function deactivate() {}

module.exports = { activate, deactivate };
