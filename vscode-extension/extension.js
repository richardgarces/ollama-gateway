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
 * Stream a chat completion via WebSocket.
 * Server must authenticate using ?token=<jwt>.
 * @param {string} prompt
 * @param {(chunk: string) => void} onChunk
 * @param {(err?: Error) => void} onDone
 * @param {AbortController} [abortCtl]
 */
function streamWS(prompt, onChunk, onDone, abortCtl) {
  const WS = globalThis.WebSocket;
  if (typeof WS !== 'function') {
    onDone(new Error('WebSocket no disponible en runtime de la extension'));
    return;
  }

  const { apiUrl, model, jwtToken } = getConfig();
  if (!jwtToken) {
    onDone(new Error('JWT token requerido para WebSocket'));
    return;
  }

  const wsBase = apiUrl.replace(/^http:/i, 'ws:').replace(/^https:/i, 'wss:');
  const url = new URL(wsBase + '/ws/chat');
  url.searchParams.set('token', jwtToken);

  let done = false;
  const finish = (err) => {
    if (done) return;
    done = true;
    onDone(err);
  };

  let ws;
  try {
    ws = new WS(url.toString());
  } catch (e) {
    finish(e instanceof Error ? e : new Error(String(e)));
    return;
  }

  ws.onopen = () => {
    ws.send(JSON.stringify({
      model,
      messages: [{ role: 'user', content: prompt }],
      stream: true,
    }));
  };

  ws.onmessage = (event) => {
    const raw = typeof event.data === 'string' ? event.data : event.data?.toString?.();
    if (!raw) return;
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch {
      return;
    }

    if (msg.type === 'chunk' && msg.content) {
      onChunk(msg.content);
      return;
    }
    if (msg.type === 'message' && msg.content) {
      onChunk(msg.content);
      return;
    }
    if (msg.type === 'error') {
      finish(new Error(msg.error || 'WebSocket stream error'));
      try { ws.close(); } catch {}
      return;
    }
    if (msg.type === 'done' || msg.type === 'canceled') {
      finish();
      try { ws.close(); } catch {}
    }
  };

  ws.onerror = () => {
    finish(new Error('WebSocket connection failed'));
  };

  ws.onclose = (event) => {
    if (!done) {
      finish(new Error(event?.reason || 'WebSocket closed before completion'));
    }
  };

  if (abortCtl) {
    abortCtl.signal.addEventListener('abort', () => {
      try {
        ws.send(JSON.stringify({ type: 'cancel' }));
      } catch {}
      try {
        ws.close();
      } catch {}
      finish(new Error('aborted'));
    });
  }
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

/**
 * Send JSON to gateway endpoint and parse JSON response.
 * @param {string} endpoint
 * @param {object} payload
 * @returns {Promise<any>}
 */
function postJSON(endpoint, payload) {
  const { apiUrl, jwtToken } = getConfig();
  const url = new URL(apiUrl + endpoint);
  const lib = url.protocol === 'https:' ? https : http;

  const body = JSON.stringify(payload || {});
  const headers = {
    'Content-Type': 'application/json',
    'Content-Length': Buffer.byteLength(body),
  };
  if (jwtToken) {
    headers['Authorization'] = 'Bearer ' + jwtToken;
  }

  return new Promise((resolve, reject) => {
    const req = lib.request(url, { method: 'POST', headers }, (res) => {
      let raw = '';
      res.on('data', (chunk) => { raw += chunk.toString(); });
      res.on('end', () => {
        const status = res.statusCode || 0;
        const parsed = raw ? safeJSONParse(raw) : {};
        if (status < 200 || status >= 300) {
          const message = parsed?.error || ('HTTP ' + status);
          reject(new Error(message));
          return;
        }
        resolve(parsed || {});
      });
      res.on('error', reject);
    });
    req.on('error', reject);
    req.write(body);
    req.end();
  });
}

function safeJSONParse(raw) {
  try {
    return JSON.parse(raw);
  } catch {
    return { raw };
  }
}

function normalizeLanguageId(id) {
  const v = String(id || '').toLowerCase();
  const map = {
    javascript: 'javascript',
    typescript: 'typescript',
    python: 'python',
    go: 'go',
    java: 'java',
    rust: 'rust',
    c: 'c',
    cpp: 'cpp',
    csharp: 'csharp',
    ruby: 'ruby',
    php: 'php',
  };
  return map[v] || v || 'unknown';
}

function targetEditorLanguage(toLang) {
  const v = String(toLang || '').toLowerCase();
  if (v === 'csharp') return 'csharp';
  if (v === 'cpp') return 'cpp';
  return v || 'plaintext';
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
    } else if (msg.type === 'externalResult') {
      addMsg('assistant', msg.text || '');
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

  // --- Send Selection command (WS first → HTTP SSE fallback → CLI fallback) ---
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

      streamWS(text, (chunk) => output.append(chunk), (wsErr) => {
        if (!wsErr) {
          done();
          return;
        }
        output.appendLine('[WS failed, falling back to HTTP SSE] ' + wsErr.message);
        streamHTTP(text, (chunk) => output.append(chunk), (httpErr) => {
          if (httpErr) {
            output.appendLine('[HTTP failed, falling back to CLI] ' + httpErr.message);
            streamCLI(text, (c) => output.append(c), done);
            return;
          }
          done();
        });
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
          streamWS(msg.text,
            (chunk) => chatPanel?.webview.postMessage({ type: 'chunk', text: chunk }),
            (wsErr) => {
              if (!wsErr) {
                chatPanel?.webview.postMessage({ type: 'done' });
                return;
              }
              streamHTTP(
                msg.text,
                (chunk) => chatPanel?.webview.postMessage({ type: 'chunk', text: chunk }),
                (httpErr) => {
                  if (httpErr) {
                    streamCLI(
                      msg.text,
                      (c) => chatPanel?.webview.postMessage({ type: 'chunk', text: c }),
                      (e) => {
                        if (e) chatPanel?.webview.postMessage({ type: 'error', text: e.message });
                        else chatPanel?.webview.postMessage({ type: 'done' });
                      },
                    );
                    return;
                  }
                  chatPanel?.webview.postMessage({ type: 'done' });
                },
              );
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

  // Analyze selected stack trace using backend debug endpoint
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.debugError', async () => {
      const editor = vscode.window.activeTextEditor;
      const selected = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection)?.trim() || '';
      const clipboard = (await vscode.env.clipboard.readText()).trim();
      const initial = selected || clipboard;

      const stackTrace = await vscode.window.showInputBox({
        title: 'Debug Error',
        prompt: 'Pega stack trace o logs de error para analizar',
        value: initial,
        ignoreFocusOut: true,
      });
      if (!stackTrace || !stackTrace.trim()) {
        vscode.window.showInformationMessage('No hay contenido para analizar');
        return;
      }

      try {
        const result = await postJSON('/api/debug/error', { stack_trace: stackTrace });
        const text = [
          'Root cause: ' + (result.root_cause || 'N/A'),
          '',
          'Explanation:',
          result.explanation || 'N/A',
          '',
          'Suggested fixes:',
          ...(Array.isArray(result.suggested_fixes) && result.suggested_fixes.length > 0
            ? result.suggested_fixes.map((v) => '- ' + v)
            : ['- N/A']),
          '',
          'Related files:',
          ...(Array.isArray(result.related_files) && result.related_files.length > 0
            ? result.related_files.map((v) => '- ' + v)
            : ['- N/A']),
        ].join('\n');

        await vscode.commands.executeCommand('copilot-local.openChat');
        if (chatPanel) {
          chatPanel.webview.postMessage({ type: 'externalResult', text });
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        vscode.window.showErrorMessage('Debug analysis failed: ' + msg);
      }
    }),
  );

  // Translate current selection using backend translator endpoint
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.translateSelection', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showInformationMessage('No active editor');
        return;
      }

      const selected = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection).trim();
      if (!selected) {
        vscode.window.showInformationMessage('No hay contenido para traducir');
        return;
      }

      const options = [
        { label: 'Go', value: 'go' },
        { label: 'Python', value: 'python' },
        { label: 'TypeScript', value: 'typescript' },
        { label: 'JavaScript', value: 'javascript' },
        { label: 'Java', value: 'java' },
        { label: 'Rust', value: 'rust' },
        { label: 'C#', value: 'csharp' },
        { label: 'C++', value: 'cpp' },
      ];
      const picked = await vscode.window.showQuickPick(options, {
        title: 'Translate Selection',
        placeHolder: 'Elige lenguaje destino',
      });
      if (!picked) {
        return;
      }

      const from = normalizeLanguageId(editor.document.languageId);
      try {
        const result = await postJSON('/api/translate', {
          code: selected,
          from,
          to: picked.value,
        });

        const translated = (result && result.translated_code) ? String(result.translated_code) : '';
        if (!translated) {
          vscode.window.showErrorMessage('La API no devolvió translated_code');
          return;
        }

        const doc = await vscode.workspace.openTextDocument({
          content: translated,
          language: targetEditorLanguage(picked.value),
        });
        await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        vscode.window.showErrorMessage('Translate failed: ' + msg);
      }
    }),
  );

  // Generate unit tests for selection or current file
  context.subscriptions.push(
    vscode.commands.registerCommand('copilot-local.addTests', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showInformationMessage('No active editor');
        return;
      }

      const mode = await vscode.window.showQuickPick([
        { label: 'Generate tests from selection/current file', value: 'generate' },
        { label: 'Generate and apply tests for current file', value: 'apply' },
      ], {
        title: 'Add Tests',
        placeHolder: 'Elige el modo de generación',
      });
      if (!mode) {
        return;
      }

      const selected = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection).trim();
      const fullText = editor.document.getText().trim();
      const code = selected || fullText;
      if (!code && mode.value === 'generate') {
        vscode.window.showInformationMessage('No hay contenido para generar tests');
        return;
      }

      try {
        let result;
        if (mode.value === 'apply') {
          result = await postJSON('/api/testgen/file', {
            path: editor.document.fileName,
            apply: true,
          });
        } else {
          const lang = normalizeLanguageId(editor.document.languageId);
          result = await postJSON('/api/testgen', {
            code,
            lang,
          });
        }

        const testCode = (result && result.test_code) ? String(result.test_code) : '';
        if (!testCode) {
          vscode.window.showErrorMessage('La API no devolvió test_code');
          return;
        }

        const doc = await vscode.workspace.openTextDocument({
          content: testCode,
          language: editor.document.languageId || 'plaintext',
        });
        await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });

        if (mode.value === 'apply' && result.applied_path) {
          vscode.window.showInformationMessage('Tests aplicados en: ' + result.applied_path);
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        vscode.window.showErrorMessage('Add tests failed: ' + msg);
      }
    }),
  );
}

function deactivate() {}

module.exports = { activate, deactivate };
