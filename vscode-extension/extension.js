const vscode = require('vscode');
const http = require('http');
const https = require('https');
const { spawn } = require('child_process');
const path = require('path');
const { LocalMetrics } = require('./metrics');

function getConfig() {
  const cfg = vscode.workspace.getConfiguration('copilotLocal');
  return {
    apiUrl: (cfg.get('apiUrl') || 'http://localhost:8081').replace(/\/+$/, ''),
    model: cfg.get('model') || 'local-rag',
    cliPath: cfg.get('cliPath') || '',
    jwtToken: cfg.get('jwtToken') || '',
    inlineCompletions: cfg.get('inlineCompletions', true),
    chatFontSize: cfg.get('chatFontSize', 13),
  };
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

function safeJSONParse(raw) {
  try { return JSON.parse(raw); } catch { return { raw }; }
}

function requestJSON(method, endpoint, payload) {
  const { apiUrl, jwtToken } = getConfig();
  const url = new URL(apiUrl + endpoint);
  const lib = url.protocol === 'https:' ? https : http;
  const body = payload ? JSON.stringify(payload) : '';
  const headers = {};
  if (body) {
    headers['Content-Type'] = 'application/json';
    headers['Content-Length'] = Buffer.byteLength(body);
  }
  if (jwtToken) headers['Authorization'] = 'Bearer ' + jwtToken;

  return new Promise((resolve, reject) => {
    const req = lib.request(url, { method, headers }, (res) => {
      let raw = '';
      res.on('data', (chunk) => { raw += chunk.toString(); });
      res.on('end', () => {
        const status = res.statusCode || 0;
        const parsed = raw ? safeJSONParse(raw) : {};
        if (status < 200 || status >= 300) {
          reject(new Error(parsed?.error || ('HTTP ' + status)));
          return;
        }
        resolve(parsed || {});
      });
      res.on('error', reject);
    });
    req.on('error', reject);
    if (body) req.write(body);
    req.end();
  });
}

function postJSON(endpoint, payload) {
  return requestJSON('POST', endpoint, payload);
}

function getJSON(endpoint) {
  return requestJSON('GET', endpoint);
}

function streamHTTP(prompt, model, onChunk, onDone, abortCtl, metrics) {
  const { apiUrl, jwtToken } = getConfig();
  const url = new URL(apiUrl + '/openai/v1/chat/completions');
  const lib = url.protocol === 'https:' ? https : http;
  const startedAt = Date.now();
  let firstChunkAt = 0;
  let chars = 0;
  let finished = false;

  const finish = async (err) => {
    if (finished) return;
    finished = true;
    if (metrics) {
      await metrics.trackRequest(Date.now() - startedAt, chars, firstChunkAt ? (firstChunkAt - startedAt) : 0, !!err);
    }
    onDone(err);
  };

  const body = JSON.stringify({
    model,
    messages: [{ role: 'user', content: prompt }],
    stream: true,
  });

  const headers = { 'Content-Type': 'application/json' };
  if (jwtToken) headers['Authorization'] = 'Bearer ' + jwtToken;

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
        if (payload === '[DONE]') { finish(); return; }
        try {
          const obj = JSON.parse(payload);
          const delta = obj?.choices?.[0]?.delta;
          if (delta?.content) {
            if (!firstChunkAt) firstChunkAt = Date.now();
            chars += delta.content.length;
            onChunk(delta.content);
          }
        } catch {}
      }
    });
    res.on('end', () => finish());
    res.on('error', (e) => finish(e));
  });

  req.on('error', (e) => finish(e));
  if (abortCtl) {
    abortCtl.signal.addEventListener('abort', () => req.destroy());
  }
  req.write(body);
  req.end();
}

function streamWS(prompt, model, onChunk, onDone, abortCtl, metrics) {
  const WS = globalThis.WebSocket;
  if (typeof WS !== 'function') {
    onDone(new Error('WebSocket no disponible en runtime de la extension'));
    return;
  }

  const { apiUrl, jwtToken } = getConfig();
  if (!jwtToken) {
    onDone(new Error('JWT token requerido para WebSocket'));
    return;
  }

  const wsBase = apiUrl.replace(/^http:/i, 'ws:').replace(/^https:/i, 'wss:');
  const url = new URL(wsBase + '/ws/chat');
  url.searchParams.set('token', jwtToken);

  const startedAt = Date.now();
  let firstChunkAt = 0;
  let chars = 0;
  let finished = false;

  const finish = async (err) => {
    if (finished) return;
    finished = true;
    if (metrics) {
      await metrics.trackRequest(Date.now() - startedAt, chars, firstChunkAt ? (firstChunkAt - startedAt) : 0, !!err);
    }
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
    ws.send(JSON.stringify({ model, messages: [{ role: 'user', content: prompt }], stream: true }));
  };

  ws.onmessage = (event) => {
    const raw = typeof event.data === 'string' ? event.data : event.data?.toString?.();
    if (!raw) return;
    let msg;
    try { msg = JSON.parse(raw); } catch { return; }

    if (msg.type === 'chunk' && msg.content) {
      if (!firstChunkAt) firstChunkAt = Date.now();
      chars += msg.content.length;
      onChunk(msg.content);
      return;
    }
    if (msg.type === 'message' && msg.content) {
      if (!firstChunkAt) firstChunkAt = Date.now();
      chars += msg.content.length;
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

  ws.onerror = () => finish(new Error('WebSocket connection failed'));
  ws.onclose = (event) => {
    if (!finished) finish(new Error(event?.reason || 'WebSocket closed before completion'));
  };

  if (abortCtl) {
    abortCtl.signal.addEventListener('abort', () => {
      try { ws.send(JSON.stringify({ type: 'cancel' })); } catch {}
      try { ws.close(); } catch {}
      finish(new Error('aborted'));
    });
  }
}

function streamCLI(prompt, model, onChunk, onDone) {
  const { cliPath } = getConfig();
  const resolved = cliPath || path.join(vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || '.', 'api', 'bin', 'copilot-cli');
  const proc = spawn(resolved, ['--model', model, '--prompt', prompt], { stdio: ['pipe', 'pipe', 'pipe'] });
  proc.stdout.on('data', (d) => onChunk(d.toString()));
  proc.stderr.on('data', (d) => onChunk(d.toString()));
  proc.on('close', () => onDone());
  proc.on('error', (e) => onDone(e));
}

function runGitDiffCached(repoRoot) {
  return new Promise((resolve, reject) => {
    const args = ['-C', repoRoot, 'diff', '--cached'];
    const proc = spawn('git', args, { stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    proc.stdout.on('data', (d) => { stdout += d.toString(); });
    proc.stderr.on('data', (d) => { stderr += d.toString(); });
    proc.on('error', (err) => reject(err));
    proc.on('close', (code) => {
      if (code !== 0) {
        reject(new Error((stderr || ('git diff --cached failed with code ' + code)).trim()));
        return;
      }
      resolve(stdout);
    });
  });
}

async function trySetSCMInput(message) {
  const msg = String(message || '').trim();
  if (!msg) return false;
  await vscode.commands.executeCommand('workbench.view.scm');
  const commands = ['git.setInputBoxValue', 'git.setCommitInput', 'git.setCommitMessage'];
  for (const id of commands) {
    try {
      await vscode.commands.executeCommand(id, msg);
      return true;
    } catch {}
  }
  return false;
}

function getChatPanelHTML(fontSize) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/styles/github-dark.min.css">
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: var(--vscode-font-family); font-size: ${fontSize}px; background: var(--vscode-editor-background); color: var(--vscode-editor-foreground); height: 100vh; display: flex; flex-direction: column; }
#header { display: flex; align-items: center; justify-content: space-between; padding: 8px 10px; border-bottom: 1px solid var(--vscode-panel-border); background: linear-gradient(90deg, var(--vscode-editor-background), var(--vscode-sideBar-background)); gap: 8px; }
#leftTools { display: flex; align-items: center; gap: 8px; }
#models { background: var(--vscode-dropdown-background); color: var(--vscode-dropdown-foreground); border: 1px solid var(--vscode-dropdown-border); padding: 4px; }
#messages { flex: 1; overflow-y: auto; padding: 12px; }
.msg { margin-bottom: 12px; line-height: 1.5; word-wrap: break-word; }
.msg .role { font-weight: 700; margin-right: 6px; }
.msg.user .role { color: var(--vscode-textLink-foreground); }
.msg-content { white-space: pre-wrap; }
pre { background: color-mix(in srgb, var(--vscode-editor-background) 85%, #000 15%); border: 1px solid var(--vscode-panel-border); border-radius: 8px; padding: 10px; overflow-x: auto; margin-top: 6px; position: relative; }
pre code { font-family: var(--vscode-editor-font-family); font-size: 0.95em; }
.code-actions { position: absolute; top: 6px; right: 6px; display: flex; gap: 4px; }
.code-actions button { padding: 2px 8px; font-size: 11px; }
#inputRow { display: flex; gap: 8px; border-top: 1px solid var(--vscode-panel-border); padding: 8px; }
#prompt { flex: 1; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); border-radius: 4px; padding: 8px; min-height: 36px; max-height: 140px; resize: vertical; }
button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: 1px solid var(--vscode-button-border); border-radius: 4px; padding: 6px 12px; cursor: pointer; }
button:hover { filter: brightness(1.06); }
button:disabled { opacity: 0.5; cursor: default; }
</style>
</head>
<body>
<div id="header"><div id="leftTools"><strong>Copilot Local Chat</strong><select id="models"></select></div><button id="exportBtn">Export</button></div>
<div id="messages"></div>
<div id="inputRow"><textarea id="prompt" rows="1" placeholder="Ask something..."></textarea><button id="send">Send</button></div>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/core.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/go.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/javascript.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/typescript.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/python.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/bash.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/json.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/yaml.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/highlight.js@11.10.0/lib/languages/sql.min.js"></script>
<script>
const vscode = acquireVsCodeApi();
const messagesEl = document.getElementById('messages');
const promptEl = document.getElementById('prompt');
const sendBtn = document.getElementById('send');
const exportBtn = document.getElementById('exportBtn');
const modelsEl = document.getElementById('models');
let pending = null;
const chatHistory = [];

function sanitizeHTML(text) {
  return String(text || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\"/g, '&quot;').replace(/'/g, '&#39;');
}

function renderMarkdownWithCode(text) {
  const src = String(text || '');
  const fence = String.fromCharCode(96) + String.fromCharCode(96) + String.fromCharCode(96);
  const re = new RegExp(fence + '([a-zA-Z0-9_-]*)\\\\n([\\\\s\\\\S]*?)' + fence, 'g');
  let i = 0;
  let out = '';
  let m;
  while ((m = re.exec(src)) !== null) {
    out += '<span>' + sanitizeHTML(src.slice(i, m.index)).replace(/\\n/g, '<br>') + '</span>';
    const lang = sanitizeHTML(m[1] || 'plaintext');
    const code = sanitizeHTML(m[2] || '');
    out += '<pre><div class="code-actions"><button data-copy="1">Copy</button><button data-apply="1" data-lang="' + lang + '">Apply</button></div><code class="language-' + lang + '">' + code + '</code></pre>';
    i = m.index + m[0].length;
  }
  out += '<span>' + sanitizeHTML(src.slice(i)).replace(/\\n/g, '<br>') + '</span>';
  return out;
}

function attachCodeActions(root) {
  root.querySelectorAll('button[data-copy]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const code = btn.closest('pre')?.querySelector('code')?.innerText || '';
      await navigator.clipboard.writeText(code);
    });
  });
  root.querySelectorAll('button[data-apply]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const code = btn.closest('pre')?.querySelector('code')?.innerText || '';
      const lang = btn.getAttribute('data-lang') || '';
      vscode.postMessage({ type: 'apply', code, lang });
    });
  });
}

function addMessage(role, text) {
  const d = document.createElement('div');
  d.className = 'msg ' + role;
  const roleEl = document.createElement('span');
  roleEl.className = 'role';
  roleEl.textContent = role === 'user' ? 'You:' : 'AI:';
  const contentEl = document.createElement('div');
  contentEl.className = 'msg-content';
  contentEl.innerHTML = renderMarkdownWithCode(text);
  d.appendChild(roleEl);
  d.appendChild(contentEl);
  messagesEl.appendChild(d);
  d.querySelectorAll('pre code').forEach((el) => { try { hljs.highlightElement(el); } catch {} });
  attachCodeActions(d);
  chatHistory.push({ role, content: text, timestamp: Date.now() });
  messagesEl.scrollTop = messagesEl.scrollHeight;
  return d;
}

function startAssistant() { pending = addMessage('assistant', ''); return pending; }

function updatePending(text) {
  if (!pending) return;
  const prev = pending.querySelector('.msg-content')?.innerText || '';
  const next = prev + text;
  pending.querySelector('.msg-content').innerHTML = renderMarkdownWithCode(next);
  pending.querySelectorAll('pre code').forEach((el) => { try { hljs.highlightElement(el); } catch {} });
  attachCodeActions(pending);
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function sendNow(text) {
  if (!text.trim()) return;
  addMessage('user', text);
  sendBtn.disabled = true;
  startAssistant();
  vscode.postMessage({ type: 'chat', text, model: modelsEl.value || '' });
}

function send() {
  const text = promptEl.value.trim();
  if (!text) return;
  promptEl.value = '';
  sendNow(text);
}

sendBtn.addEventListener('click', send);
promptEl.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    send();
  }
});

exportBtn.addEventListener('click', () => {
  vscode.postMessage({ type: 'export', messages: chatHistory });
});

window.addEventListener('message', (e) => {
  const msg = e.data;
  if (msg.type === 'chunk') { updatePending(msg.text || ''); return; }
  if (msg.type === 'done') { pending = null; sendBtn.disabled = false; return; }
  if (msg.type === 'error') { updatePending('\\n[Error: ' + (msg.text || 'unknown') + ']'); pending = null; sendBtn.disabled = false; return; }
  if (msg.type === 'prefill') { promptEl.value = msg.text || ''; promptEl.focus(); return; }
  if (msg.type === 'runPrompt') { sendNow(msg.text || ''); return; }
  if (msg.type === 'externalResult') { addMessage('assistant', msg.text || ''); return; }
  if (msg.type === 'models') {
    const models = Array.isArray(msg.models) ? msg.models : [];
    const current = msg.current || '';
    modelsEl.innerHTML = '';
    const arr = models.length > 0 ? models : [current || 'local-rag'];
    arr.forEach((m) => { const opt = document.createElement('option'); opt.value = m; opt.textContent = m; modelsEl.appendChild(opt); });
    modelsEl.value = arr.includes(current) ? current : arr[0];
  }
});
</script>
</body>
</html>`;
}

function activate(context) {
  const output = vscode.window.createOutputChannel('Copilot Local');
  const metrics = new LocalMetrics(context);
  let chatPanel = null;
  let activeSessionId = '';

  const inlineStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 90);
  inlineStatus.text = 'Copilot Local';
  inlineStatus.tooltip = 'Inline completions status';
  inlineStatus.show();
  context.subscriptions.push(inlineStatus);

  const indexerStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 90);
  indexerStatus.text = 'Indexer: ...';
  indexerStatus.show();
  context.subscriptions.push(indexerStatus);

  const refreshIndexerStatus = async () => {
    try {
      const status = await getJSON('/internal/index/status');
      indexerStatus.text = (status?.reindexing || status?.watcher_active) ? 'Indexer: Indexing...' : 'Indexer: Indexed ✓';
      indexerStatus.tooltip = JSON.stringify(status);
    } catch {
      indexerStatus.text = 'Indexer: unavailable';
    }
  };

  refreshIndexerStatus();
  const poll = setInterval(refreshIndexerStatus, 15000);
  context.subscriptions.push({ dispose: () => clearInterval(poll) });

  const completionDebounce = new Map();
  const inlineProvider = {
    provideInlineCompletionItems: async (document, position) => {
      const cfg = getConfig();
      if (!cfg.inlineCompletions) return [];
      if (document.lineCount < 3) return [];

      const key = document.uri.toString();
      if (completionDebounce.has(key)) clearTimeout(completionDebounce.get(key));
      await new Promise((resolve) => {
        const t = setTimeout(resolve, 500);
        completionDebounce.set(key, t);
      });

      const startLine = Math.max(0, position.line - 49);
      const range = new vscode.Range(new vscode.Position(startLine, 0), position);
      const contextText = document.getText(range);
      inlineStatus.text = 'Copilot Local: completando...';
      try {
        const res = await postJSON('/openai/v1/completions', { prompt: contextText, stream: false, max_tokens: 100 });
        const text = String(res?.choices?.[0]?.text || '').trim();
        if (!text) return [];
        return [new vscode.InlineCompletionItem(text, new vscode.Range(position, position))];
      } catch {
        return [];
      } finally {
        inlineStatus.text = 'Copilot Local';
      }
    },
  };
  context.subscriptions.push(vscode.languages.registerInlineCompletionItemProvider({ pattern: '**' }, inlineProvider));

  const streamWithFallback = (prompt, model, onChunk, onDone) => {
    streamWS(prompt, model, onChunk, (wsErr) => {
      if (!wsErr) {
        onDone();
        return;
      }
      streamHTTP(prompt, model, onChunk, (httpErr) => {
        if (!httpErr) {
          onDone();
          return;
        }
        streamCLI(prompt, model, onChunk, onDone);
      }, undefined, metrics);
    }, undefined, metrics);
  };

  async function openChatPanel() {
    if (chatPanel) {
      chatPanel.reveal();
      return chatPanel;
    }

    chatPanel = vscode.window.createWebviewPanel('copilotLocalChat', 'Copilot Local Chat', vscode.ViewColumn.Beside, {
      enableScripts: true,
      retainContextWhenHidden: true,
    });

    chatPanel.webview.html = getChatPanelHTML(getConfig().chatFontSize);

    const publishModels = async () => {
      const cfg = getConfig();
      try {
        const res = await getJSON('/api/models');
        chatPanel?.webview.postMessage({ type: 'models', models: res?.models || [], current: cfg.model });
      } catch {
        chatPanel?.webview.postMessage({ type: 'models', models: [cfg.model], current: cfg.model });
      }
    };
    publishModels();

    chatPanel.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'chat') {
        const model = String(msg.model || getConfig().model);
        if (activeSessionId) {
          const endpoint = '/api/sessions/' + encodeURIComponent(activeSessionId) + '/chat';
          try {
            const res = await postJSON(endpoint, { message: msg.text });
            if (res?.response) chatPanel?.webview.postMessage({ type: 'chunk', text: String(res.response) });
            chatPanel?.webview.postMessage({ type: 'done' });
          } catch (err) {
            chatPanel?.webview.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
          }
          return;
        }

        streamWithFallback(msg.text, model,
          (chunk) => chatPanel?.webview.postMessage({ type: 'chunk', text: chunk }),
          (err) => {
            if (err) chatPanel?.webview.postMessage({ type: 'error', text: err.message });
            else chatPanel?.webview.postMessage({ type: 'done' });
          },
        );
        return;
      }

      if (msg.type === 'export') {
        const format = await vscode.window.showQuickPick([
          { label: 'Markdown', value: 'md' },
          { label: 'JSON', value: 'json' },
          { label: 'Text', value: 'txt' },
        ], { title: 'Export chat history' });
        if (!format) return;
        const messages = Array.isArray(msg.messages) ? msg.messages : [];
        let content = '';
        let language = 'plaintext';
        if (format.value === 'md') {
          language = 'markdown';
          content = messages.map((m) => '## ' + (m.role === 'user' ? 'User' : 'Assistant') + '\n\n' + (m.content || '')).join('\n\n');
        } else if (format.value === 'json') {
          language = 'json';
          content = JSON.stringify(messages, null, 2);
        } else {
          language = 'plaintext';
          content = messages.map((m) => (m.role === 'user' ? 'You: ' : 'AI: ') + (m.content || '')).join('\n\n');
        }
        const doc = await vscode.workspace.openTextDocument({ content, language });
        await vscode.window.showTextDocument(doc, { preview: false });
        return;
      }

      if (msg.type === 'apply') {
        const code = String(msg.code || '');
        if (!code.trim()) return;
        const ok = await vscode.window.showInformationMessage('Apply code to editor?', 'Yes', 'No');
        if (ok !== 'Yes') return;

        const editor = vscode.window.activeTextEditor;
        if (!editor) {
          const doc = await vscode.workspace.openTextDocument({ content: code, language: targetEditorLanguage(msg.lang) });
          await vscode.window.showTextDocument(doc, { preview: false });
          return;
        }

        const isDiff = code.startsWith('---') || code.startsWith('@@');
        if (isDiff) {
          const edit = new vscode.WorkspaceEdit();
          const fullRange = new vscode.Range(editor.document.positionAt(0), editor.document.positionAt(editor.document.getText().length));
          edit.replace(editor.document.uri, fullRange, code);
          await vscode.workspace.applyEdit(edit);
        } else {
          await editor.edit((editBuilder) => editBuilder.insert(editor.selection.active, code));
        }

        try { await vscode.commands.executeCommand('editor.action.formatDocument'); } catch {}
      }
    });

    chatPanel.onDidDispose(() => { chatPanel = null; });
    return chatPanel;
  }

  async function sendSelectionPrompt(prefix) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showInformationMessage('No active editor');
      return;
    }
    const selected = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection).trim();
    if (!selected) {
      vscode.window.showInformationMessage('No hay contenido seleccionado');
      return;
    }
    const panel = await openChatPanel();
    panel.webview.postMessage({ type: 'runPrompt', text: prefix + '\n' + selected });
  }

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.sendSelection', async () => {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showInformationMessage('No active editor');
      return;
    }
    const text = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection);
    if (!text || !text.trim()) {
      vscode.window.showInformationMessage('Nothing to send');
      return;
    }

    output.show(true);
    output.appendLine('--- Request ---');
    streamWithFallback(text, getConfig().model,
      (chunk) => output.append(chunk),
      (err) => {
        if (err) output.appendLine('\n[Error] ' + err.message);
        output.appendLine('\n--- Done ---');
      },
    );
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openChat', async () => {
    await openChatPanel();
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.sendSelectionToChat', async () => {
    const editor = vscode.window.activeTextEditor;
    const text = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection) || '';
    const panel = await openChatPanel();
    if (text.trim()) panel.webview.postMessage({ type: 'prefill', text });
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.explainSelection', async () => {
    await sendSelectionPrompt('Explain this code:');
  }));
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.refactorSelection', async () => {
    await sendSelectionPrompt('Refactor this code for clarity:');
  }));
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.fixErrors', async () => {
    await sendSelectionPrompt('Fix the errors in this code:');
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.debugError', async () => {
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
        ...(Array.isArray(result.suggested_fixes) && result.suggested_fixes.length > 0 ? result.suggested_fixes.map((v) => '- ' + v) : ['- N/A']),
        '',
        'Related files:',
        ...(Array.isArray(result.related_files) && result.related_files.length > 0 ? result.related_files.map((v) => '- ' + v) : ['- N/A']),
      ].join('\n');

      const panel = await openChatPanel();
      panel.webview.postMessage({ type: 'externalResult', text });
    } catch (err) {
      vscode.window.showErrorMessage('Debug analysis failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.translateSelection', async () => {
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
    const picked = await vscode.window.showQuickPick(options, { title: 'Translate Selection', placeHolder: 'Elige lenguaje destino' });
    if (!picked) return;

    const from = normalizeLanguageId(editor.document.languageId);
    try {
      const result = await postJSON('/api/translate', { code: selected, from, to: picked.value });
      const translated = (result && result.translated_code) ? String(result.translated_code) : '';
      if (!translated) {
        vscode.window.showErrorMessage('La API no devolvió translated_code');
        return;
      }
      const doc = await vscode.workspace.openTextDocument({ content: translated, language: targetEditorLanguage(picked.value) });
      await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });
    } catch (err) {
      vscode.window.showErrorMessage('Translate failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.addTests', async () => {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showInformationMessage('No active editor');
      return;
    }

    const mode = await vscode.window.showQuickPick([
      { label: 'Generate tests from selection/current file', value: 'generate' },
      { label: 'Generate and apply tests for current file', value: 'apply' },
    ], { title: 'Add Tests', placeHolder: 'Elige el modo de generación' });
    if (!mode) return;

    const selected = editor.document.getText(editor.selection.isEmpty ? undefined : editor.selection).trim();
    const fullText = editor.document.getText().trim();
    const code = selected || fullText;

    try {
      let result;
      if (mode.value === 'apply') {
        result = await postJSON('/api/testgen/file', { path: editor.document.fileName, apply: true });
      } else {
        const lang = normalizeLanguageId(editor.document.languageId);
        result = await postJSON('/api/testgen', { code, lang });
      }

      const testCode = (result && result.test_code) ? String(result.test_code) : '';
      if (!testCode) {
        vscode.window.showErrorMessage('La API no devolvió test_code');
        return;
      }

      const doc = await vscode.workspace.openTextDocument({ content: testCode, language: editor.document.languageId || 'plaintext' });
      await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });
      if (mode.value === 'apply' && result.applied_path) {
        vscode.window.showInformationMessage('Tests aplicados en: ' + result.applied_path);
      }
    } catch (err) {
      vscode.window.showErrorMessage('Add tests failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.joinSession', async () => {
    const sessionId = await vscode.window.showInputBox({ title: 'Join Shared Session', prompt: 'Ingresa session_id', ignoreFocusOut: true });
    if (!sessionId || !sessionId.trim()) return;

    const cleanID = sessionId.trim();
    try {
      const endpoint = '/api/sessions/' + encodeURIComponent(cleanID) + '/join';
      await postJSON(endpoint, {});
      activeSessionId = cleanID;
      const panel = await openChatPanel();
      panel.webview.postMessage({ type: 'externalResult', text: 'Connected to shared session: ' + cleanID });
    } catch (err) {
      vscode.window.showErrorMessage('Join session failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.commitMessage', async () => {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!workspaceFolder) {
      vscode.window.showErrorMessage('No workspace folder abierto');
      return;
    }

    let diff = '';
    try { diff = await runGitDiffCached(workspaceFolder); }
    catch (err) {
      vscode.window.showErrorMessage('No se pudo leer staged diff: ' + (err instanceof Error ? err.message : String(err)));
      return;
    }

    if (!diff.trim()) {
      vscode.window.showInformationMessage('No hay cambios staged para generar commit message');
      return;
    }

    try {
      const result = await postJSON('/api/commit/message', { diff });
      const message = (result && result.message) ? String(result.message).trim() : '';
      if (!message) {
        vscode.window.showErrorMessage('La API no devolvió message');
        return;
      }

      const setOk = await trySetSCMInput(message);
      if (setOk) {
        vscode.window.showInformationMessage('Commit message sugerido en Source Control');
        return;
      }

      await vscode.env.clipboard.writeText(message);
      vscode.window.showWarningMessage('No se pudo setear el input de Source Control automáticamente. Mensaje copiado al portapapeles.');
    } catch (err) {
      vscode.window.showErrorMessage('Commit message generation failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.reindex', async () => {
    await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: 'Indexing repository...' }, async (progress) => {
      progress.report({ message: 'Starting reindex...' });
      const poller = setInterval(async () => {
        try {
          await getJSON('/health');
          progress.report({ message: 'Indexer running...' });
        } catch {
          progress.report({ message: 'Waiting for server health...' });
        }
      }, 2000);
      try {
        await postJSON('/internal/index/reindex', {});
        vscode.window.showInformationMessage('Repository indexing completed');
      } finally {
        clearInterval(poller);
        await refreshIndexerStatus();
      }
    });
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.showStats', async () => {
    const panel = vscode.window.createWebviewPanel('copilotLocalStats', 'Copilot Local Stats', vscode.ViewColumn.Beside, { enableScripts: false });
    const text = metrics.summaryText();
    panel.webview.html = '<!doctype html><html><body style="font-family: var(--vscode-editor-font-family); background: var(--vscode-editor-background); color: var(--vscode-editor-foreground); padding: 16px;"><h3>Copilot Local Stats</h3><pre>' + text.replace(/&/g, '&amp;').replace(/</g, '&lt;') + '</pre></body></html>';
  }));
}

function deactivate() {}

module.exports = { activate, deactivate };
