const vscode = require('vscode');
const http = require('http');
const https = require('https');
const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');
const { LocalMetrics } = require('./metrics');

const DEFAULT_WORKSPACE_PROFILES = {
  fast: { model: 'local-rag', lang: 'es', temperature: 0.1 },
  balanced: { model: 'local-rag', lang: 'es', temperature: 0.3 },
  'deep-analysis': { model: 'local-rag', lang: 'es', temperature: 0.6 },
};

function normalizeTemperature(value, fallback) {
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  return Math.max(0, Math.min(1.5, n));
}

function normalizeProfile(profile, fallbackModel) {
  const model = String(profile?.model || '').trim() || fallbackModel;
  const lang = String(profile?.lang || '').trim() || 'es';
  const temperature = normalizeTemperature(profile?.temperature, 0.3);
  return { model, lang, temperature };
}

function buildWorkspaceProfiles(customProfiles, fallbackModel) {
  const out = {};
  for (const [id, profile] of Object.entries(DEFAULT_WORKSPACE_PROFILES)) {
    out[id] = normalizeProfile(profile, fallbackModel);
  }
  if (!customProfiles || typeof customProfiles !== 'object' || Array.isArray(customProfiles)) {
    return out;
  }
  for (const [id, profile] of Object.entries(customProfiles)) {
    const key = String(id || '').trim();
    if (!key || !profile || typeof profile !== 'object' || Array.isArray(profile)) continue;
    out[key] = normalizeProfile(profile, fallbackModel);
  }
  return out;
}

function buildChatMessages(prompt, lang) {
  const desiredLang = String(lang || '').trim();
  const messages = [];
  if (desiredLang) {
    messages.push({ role: 'system', content: 'Respond in language: ' + desiredLang + '.' });
  }
  messages.push({ role: 'user', content: prompt });
  return messages;
}

function getConfig() {
  const cfg = vscode.workspace.getConfiguration('copilotLocal');
  const baseModel = String(cfg.get('model') || 'local-rag').trim() || 'local-rag';
  const profiles = buildWorkspaceProfiles(cfg.get('workspaceProfiles', {}), baseModel);
  const requestedProfileId = String(cfg.get('workspaceActiveProfile') || 'balanced').trim() || 'balanced';
  const activeProfileId = profiles[requestedProfileId] ? requestedProfileId : 'balanced';
  const activeProfile = profiles[activeProfileId] || normalizeProfile(DEFAULT_WORKSPACE_PROFILES.balanced, baseModel);

  const workspaceModel = String(cfg.get('workspaceModel') || '').trim();
  const workspaceLang = String(cfg.get('workspaceLang') || '').trim();
  const workspaceTempRaw = cfg.get('workspaceTemperature');
  const workspaceTemp = Number.isFinite(Number(workspaceTempRaw)) ? Number(workspaceTempRaw) : null;

  const resolvedModel = workspaceModel || activeProfile.model || baseModel;
  const chatModel = String(cfg.get('chatModel') || '').trim() || resolvedModel;
  const fimModel = String(cfg.get('fimModel') || '').trim() || resolvedModel;
  const embeddingModel = String(cfg.get('embeddingModel') || '').trim() || 'nomic-embed-text';
  const resolvedLang = workspaceLang || activeProfile.lang || 'es';
  const resolvedTemperature = workspaceTemp === null
    ? activeProfile.temperature
    : normalizeTemperature(workspaceTemp, activeProfile.temperature);

  return {
    apiUrl: (cfg.get('apiUrl') || 'http://localhost:8081').replace(/\/+$/, ''),
    model: resolvedModel,
    chatModel,
    fimModel,
    embeddingModel,
    cliPath: cfg.get('cliPath') || '',
    jwtToken: cfg.get('jwtToken') || '',
    inlineCompletions: cfg.get('inlineCompletions', true),
    chatFontSize: cfg.get('chatFontSize', 13),
    voiceInputEnabled: cfg.get('voiceInputEnabled', false),
    quickPromptTemplates: cfg.get('quickPromptTemplates', {}),
    qualityAlertsEnabled: cfg.get('qualityAlertsEnabled', true),
    qualityLatencyThresholdMs: cfg.get('qualityLatencyThresholdMs', 8000),
    qualityConsecutiveErrorsThreshold: cfg.get('qualityConsecutiveErrorsThreshold', 3),
    workspaceModel,
    workspaceLang,
    workspaceTemperature: resolvedTemperature,
    profiles,
    activeProfileId,
    activeProfile,
    lang: resolvedLang,
    temperature: resolvedTemperature,
  };
}

let promptRCText = '';

function loadPromptRC() {
  const folder = vscode.workspace.workspaceFolders?.[0]?.uri?.fsPath;
  if (!folder) {
    promptRCText = '';
    return;
  }
  const candidates = [path.join(folder, '.promptrc'), path.join(folder, '.promptrc.md')];
  for (const p of candidates) {
    try {
      const raw = fs.readFileSync(p, 'utf8');
      const trimmed = String(raw || '').trim();
      if (trimmed) {
        promptRCText = trimmed;
        return;
      }
    } catch {}
  }
  promptRCText = '';
}

function applyPromptRC(prompt) {
  const cleanPrompt = String(prompt || '');
  if (!promptRCText) return cleanPrompt;
  return '[project_instructions]\n' + promptRCText + '\n[/project_instructions]\n\n' + cleanPrompt;
}

const QUICK_PROMPT_LAST_KEY = 'copilotLocal.lastQuickPromptTemplate';
const CHAT_HISTORY_PREFIX = 'copilotLocal.chatHistory';
const FAVORITES_PREFIX = 'copilotLocal.favorites';
const CHAT_HISTORY_MAX = 500;
const FAVORITES_MAX = 300;
const CHAT_SESSION_STATE_KEY = 'copilotLocal.chatSessionState';
const CHAT_SESSION_SCHEMA_VERSION = 1;

function getQuickPromptTemplates() {
  const defaults = [
    { id: 'explain', label: 'explain', template: 'Explain this code:\n{{selection}}', source: 'built-in' },
    { id: 'optimize', label: 'optimize', template: 'Optimize this code for performance and readability:\n{{selection}}', source: 'built-in' },
    { id: 'secure', label: 'secure', template: 'Review this code for security issues and suggest fixes:\n{{selection}}', source: 'built-in' },
    { id: 'test', label: 'test', template: 'Create robust tests for this code:\n{{selection}}', source: 'built-in' },
  ];

  const cfg = getConfig();
  const custom = cfg.quickPromptTemplates;
  if (!custom || typeof custom !== 'object' || Array.isArray(custom)) {
    return defaults;
  }

  const customTemplates = [];
  for (const [id, value] of Object.entries(custom)) {
    const key = String(id || '').trim();
    const template = String(value || '').trim();
    if (!key || !template) continue;
    customTemplates.push({ id: 'custom:' + key, label: key, template, source: 'custom' });
  }
  return [...defaults, ...customTemplates];
}

function applyTemplateToSelection(template, selectedText) {
  const normalizedTemplate = String(template || '').trim();
  const selected = String(selectedText || '').trim();
  if (!normalizedTemplate) return selected;

  if (normalizedTemplate.includes('{{selection}}')) {
    return normalizedTemplate.replace(/\{\{selection\}\}/g, selected || '(sin selección activa)');
  }
  if (!selected) return normalizedTemplate;
  return normalizedTemplate + '\n\n' + selected;
}

function getWorkspaceScopeKey() {
  const folders = vscode.workspace.workspaceFolders || [];
  if (folders.length === 0) return 'no-workspace';
  return folders.map((f) => f.uri.toString()).sort().join('|');
}

function getChatHistoryStorageKey() {
  return CHAT_HISTORY_PREFIX + ':' + getWorkspaceScopeKey();
}

function normalizeHistoryMessages(messages) {
  if (!Array.isArray(messages)) return [];
  return messages
    .map((m) => ({
      id: String(m?.id || '').trim(),
      role: m?.role === 'assistant' ? 'assistant' : 'user',
      content: String(m?.content || ''),
      timestamp: Number(m?.timestamp || Date.now()),
    }))
    .filter((m) => m.id && m.content.trim())
    .slice(-CHAT_HISTORY_MAX);
}

function getFavoritesStorageKey() {
  return FAVORITES_PREFIX + ':' + getWorkspaceScopeKey();
}

function normalizeFavorites(items) {
  if (!Array.isArray(items)) return [];
  return items
    .map((f) => ({
      id: String(f?.id || '').trim(),
      title: String(f?.title || '').trim(),
      content: String(f?.content || ''),
      timestamp: Number(f?.timestamp || Date.now()),
    }))
    .filter((f) => f.id && f.title && f.content.trim())
    .slice(-FAVORITES_MAX);
}

function normalizeSessionState(raw) {
  const empty = {
    schemaVersion: CHAT_SESSION_SCHEMA_VERSION,
    messages: [],
    selectedModel: '',
    activeSessionId: '',
  };

  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return empty;
  const version = Number(raw.schemaVersion || raw.version || 0);
  if (version !== CHAT_SESSION_SCHEMA_VERSION) return empty;

  return {
    schemaVersion: CHAT_SESSION_SCHEMA_VERSION,
    messages: normalizeHistoryMessages(raw.messages),
    selectedModel: String(raw.selectedModel || '').trim(),
    activeSessionId: String(raw.activeSessionId || '').trim(),
  };
}

function buildSessionState(messages, selectedModel, activeSessionId) {
  return {
    schemaVersion: CHAT_SESSION_SCHEMA_VERSION,
    messages: normalizeHistoryMessages(messages),
    selectedModel: String(selectedModel || '').trim(),
    activeSessionId: String(activeSessionId || '').trim(),
  };
}

function buildFavoriteTitle(content) {
  const firstLine = String(content || '').split('\n')[0] || '';
  const clean = firstLine.replace(/\s+/g, ' ').trim();
  if (!clean) return 'Untitled favorite';
  return clean.length > 72 ? clean.slice(0, 72) + '…' : clean;
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

function requestJSON(method, endpoint, payload, abortSignal) {
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
    if (abortSignal) {
      const onAbort = () => {
        try { req.destroy(new Error('aborted')); } catch {}
      };
      if (abortSignal.aborted) {
        onAbort();
      } else {
        abortSignal.addEventListener('abort', onAbort, { once: true });
      }
    }
    if (body) req.write(body);
    req.end();
  });
}

function postJSON(endpoint, payload) {
  return requestJSON('POST', endpoint, payload);
}

function postJSONAbort(endpoint, payload, abortSignal) {
  return requestJSON('POST', endpoint, payload, abortSignal);
}

function getJSON(endpoint) {
  return requestJSON('GET', endpoint);
}

async function requestChatCompletion(prompt, model, metrics) {
  const startedAt = Date.now();
  const cfg = getConfig();
  const finalPrompt = applyPromptRC(prompt);
  let content = '';
  let hadError = false;
  try {
    const res = await postJSON('/openai/v1/chat/completions', {
      model: model || cfg.chatModel,
      messages: buildChatMessages(finalPrompt, cfg.lang),
      temperature: cfg.temperature,
      stream: false,
    });
    const elapsedMs = Date.now() - startedAt;
    content = String(res?.choices?.[0]?.message?.content || res?.choices?.[0]?.text || '').trim();
    return {
      model,
      content,
      elapsedMs,
      length: content.length,
    };
  } catch (err) {
    hadError = true;
    throw err;
  } finally {
    if (metrics) {
      await metrics.trackRequest(Date.now() - startedAt, content.length, 0, hadError);
    }
  }
}

function streamAgentAutopilot(task, model, wsContext, onEvent, onDone) {
  const { apiUrl, jwtToken } = getConfig();
  const url = new URL(apiUrl + '/api/agent/autopilot');
  const lib = url.protocol === 'https:' ? https : http;
  let finished = false;

  const finish = (err) => {
    if (finished) return;
    finished = true;
    onDone(err);
  };

  const body = JSON.stringify({ task, model: model || '', context: wsContext || {} });
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
          const ev = JSON.parse(payload);
          if (ev && ev.event) onEvent(ev);
        } catch {}
      }
    });
    res.on('end', () => finish());
    res.on('error', (e) => finish(e));
  });

  req.on('error', (e) => finish(e));
  req.write(body);
  req.end();
}

function streamHTTP(prompt, model, onChunk, onDone, abortCtl, metrics) {
  const { apiUrl, jwtToken, lang, temperature } = getConfig();
  const finalPrompt = applyPromptRC(prompt);
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
    model: model || getConfig().chatModel,
    messages: buildChatMessages(finalPrompt, lang),
    temperature,
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
    res.on('error', (e) => {
      if (abortCtl?.signal?.aborted) {
        finish();
        return;
      }
      finish(e);
    });
  });

  req.on('error', (e) => {
    if (abortCtl?.signal?.aborted) {
      finish();
      return;
    }
    finish(e);
  });
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

  const { apiUrl, jwtToken, lang, temperature } = getConfig();
  const finalPrompt = applyPromptRC(prompt);
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
    ws.send(JSON.stringify({ model: model || getConfig().chatModel, messages: buildChatMessages(finalPrompt, lang), temperature, stream: true }));
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

  ws.onerror = () => {
    if (abortCtl?.signal?.aborted) {
      finish();
      return;
    }
    finish(new Error('WebSocket connection failed'));
  };
  ws.onclose = (event) => {
    if (finished) return;
    if (abortCtl?.signal?.aborted) {
      finish();
      return;
    }
    finish(new Error(event?.reason || 'WebSocket closed before completion'));
  };

  if (abortCtl) {
    abortCtl.signal.addEventListener('abort', () => {
      try { ws.send(JSON.stringify({ type: 'cancel' })); } catch {}
      try { ws.close(); } catch {}
      finish();
    });
  }
}

function streamCLI(prompt, model, onChunk, onDone) {
  const { cliPath, chatModel } = getConfig();
  const finalPrompt = applyPromptRC(prompt);
  let resolved = cliPath;
  if (!resolved) {
    // Try to find copilot-cli in PATH first, then fallback to workspace location
    const which = require('child_process').spawnSync('which', ['copilot-cli'], { encoding: 'utf8' });
    if (which.status === 0 && which.stdout.trim()) {
      resolved = which.stdout.trim();
    } else {
      const wsPath = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
      if (wsPath) {
        const candidate = path.join(wsPath, 'api', 'bin', 'copilot-cli');
        if (require('fs').existsSync(candidate)) {
          resolved = candidate;
        }
      }
    }
  }
  if (!resolved) {
    onDone(new Error('copilot-cli no encontrado. Configura copilotLocal.cliPath en Settings o asegúrate de que el binario esté en el PATH.'));
    return;
  }
  const proc = spawn(resolved, ['--model', model || chatModel, '--prompt', finalPrompt], { stdio: ['pipe', 'pipe', 'pipe'] });
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

function clipText(text, maxLen) {
  const src = String(text || '').trim();
  if (src.length <= maxLen) return src;
  return src.slice(0, maxLen) + '\n...[truncated]...';
}

async function tryReadTerminalSelection() {
  const before = await vscode.env.clipboard.readText();
  try {
    await vscode.commands.executeCommand('workbench.action.terminal.copySelection');
  } catch {
    return '';
  }
  const after = await vscode.env.clipboard.readText();
  if (!after || after === before) return '';
  return after;
}

async function collectExplainTestFailureContext() {
  const editor = vscode.window.activeTextEditor;
  const filePath = editor?.document?.uri?.fsPath || '';
  const languageId = editor?.document?.languageId || 'plaintext';
  const selected = editor?.document?.getText(editor.selection.isEmpty ? undefined : editor.selection) || '';

  let raw = selected.trim();
  let source = raw ? 'editor-selection' : '';
  if (!raw) {
    const termSelection = (await tryReadTerminalSelection()).trim();
    if (termSelection) {
      raw = termSelection;
      source = 'terminal-selection';
    }
  }
  if (!raw) {
    const clipboard = (await vscode.env.clipboard.readText()).trim();
    if (clipboard) {
      raw = clipboard;
      source = 'clipboard';
    }
  }

  const input = await vscode.window.showInputBox({
    title: 'Explain Test Failure',
    prompt: 'Pega o edita la salida del test fallido',
    value: raw,
    ignoreFocusOut: true,
  });

  if (!input || !input.trim()) return null;

  const codeContext = selected.trim() ? clipText(selected, 2400) : '';
  const errorOutput = clipText(input, 6000);

  const analysisPrompt = [
    'Analyze this failed test output and return a concise debugging report.',
    'Answer in Spanish.',
    '',
    'Required sections:',
    '1) Hypotheses (top causes ranked)',
    '2) Fix Plan (ordered actionable steps)',
    '3) Suggested Regression Test (include code block)',
    '',
    'Project context:',
    '- File: ' + (filePath || '(unknown)'),
    '- Language: ' + languageId,
    '- Failure source: ' + (source || 'manual-input'),
    '',
    'Selected code context:',
    codeContext || '(none)',
    '',
    'Test failure output:',
    errorOutput,
  ].join('\n');

  const regressionPrompt = [
    'Generate a regression test from this failure context.',
    'Answer in Spanish and include only:',
    '1) Short rationale (max 4 lines)',
    '2) Final test code in one fenced code block',
    '',
    'File: ' + (filePath || '(unknown)'),
    'Language: ' + languageId,
    '',
    'Code context:',
    codeContext || '(none)',
    '',
    'Failure output:',
    errorOutput,
  ].join('\n');

  return {
    prompt: analysisPrompt,
    regressionPrompt,
    source: source || 'manual-input',
  };
}

function getChatPanelHTML(fontSize, voiceInputEnabled) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: var(--vscode-font-family); font-size: ${fontSize}px; background: var(--vscode-editor-background); color: var(--vscode-editor-foreground); height: 100vh; display: flex; flex-direction: column; }
#header { display: flex; align-items: center; justify-content: space-between; padding: 8px 10px; border-bottom: 1px solid var(--vscode-panel-border); background: linear-gradient(90deg, var(--vscode-editor-background), var(--vscode-sideBar-background)); gap: 8px; }
#leftTools { display: flex; align-items: center; gap: 8px; }
#rightTools { display: flex; align-items: center; gap: 8px; }
#profileBadge { font-size: 11px; color: var(--vscode-descriptionForeground); border: 1px solid var(--vscode-panel-border); border-radius: 999px; padding: 2px 8px; }
#modelsHealth { font-size: 11px; color: var(--vscode-descriptionForeground); border: 1px solid var(--vscode-panel-border); border-radius: 999px; padding: 2px 8px; }
#models { background: var(--vscode-dropdown-background); color: var(--vscode-dropdown-foreground); border: 1px solid var(--vscode-dropdown-border); padding: 4px; }
#codeOnlyWrap { display: flex; align-items: center; gap: 6px; font-size: 12px; }
#focusStats { font-size: 11px; color: var(--vscode-descriptionForeground); min-width: 70px; }
#voiceControls { display: flex; align-items: center; gap: 6px; }
#voiceStatus { font-size: 11px; padding: 2px 8px; border: 1px solid var(--vscode-panel-border); border-radius: 999px; }
#voiceStatus[data-state="listening"] { color: var(--vscode-testing-iconPassed); border-color: var(--vscode-testing-iconPassed); }
#voiceStatus[data-state="stopped"] { color: var(--vscode-descriptionForeground); }
#voiceStatus[data-state="unavailable"] { color: var(--vscode-testing-iconFailed); border-color: var(--vscode-testing-iconFailed); }
#compareView { display: none; border-bottom: 1px solid var(--vscode-panel-border); padding: 10px; gap: 10px; }
#sessionBanner { display: none; margin: 8px 12px 0; padding: 8px 10px; border: 1px solid var(--vscode-textLink-foreground); border-radius: 6px; color: var(--vscode-textLink-foreground); background: color-mix(in srgb, var(--vscode-textLink-foreground) 10%, transparent); font-size: 12px; }
#compareHeader { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
#compareMeta { font-size: 12px; color: var(--vscode-descriptionForeground); }
#compareGrid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.compareCol { border: 1px solid var(--vscode-panel-border); border-radius: 6px; padding: 8px; min-height: 120px; background: color-mix(in srgb, var(--vscode-editor-background) 92%, #000 8%); }
.compareTitle { font-weight: 700; margin-bottom: 4px; }
.compareStats { font-size: 11px; color: var(--vscode-descriptionForeground); margin-bottom: 6px; }
.compareContent { white-space: pre-wrap; line-height: 1.4; }
#messages { flex: 1; overflow-y: auto; padding: 12px; }
.msg { margin-bottom: 12px; line-height: 1.5; word-wrap: break-word; }
.msg.highlighted { outline: 2px solid var(--vscode-textLink-foreground); border-radius: 6px; animation: historyPulse 1.5s ease-in-out 1; }
.msg .role { font-weight: 700; margin-right: 6px; }
.msg .starBtn { padding: 2px 8px; font-size: 11px; margin-left: 6px; }
.msg.user .role { color: var(--vscode-textLink-foreground); }
.msg-content { white-space: pre-wrap; }
.focus-meta { font-size: 11px; color: var(--vscode-descriptionForeground); margin-bottom: 6px; }
.focus-empty { color: var(--vscode-descriptionForeground); font-style: italic; }
pre { background: color-mix(in srgb, var(--vscode-editor-background) 85%, #000 15%); border: 1px solid var(--vscode-panel-border); border-radius: 8px; padding: 10px; overflow-x: auto; margin-top: 6px; position: relative; }
pre code { font-family: var(--vscode-editor-font-family); font-size: 0.95em; }
.code-actions { position: absolute; top: 6px; right: 6px; display: flex; gap: 4px; }
.code-actions button { padding: 2px 8px; font-size: 11px; }
#inputRow { display: flex; gap: 8px; border-top: 1px solid var(--vscode-panel-border); padding: 8px; }
#prompt { flex: 1; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); border-radius: 4px; padding: 8px; min-height: 36px; max-height: 140px; resize: vertical; }
button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: 1px solid var(--vscode-button-border); border-radius: 4px; padding: 6px 12px; cursor: pointer; }
button:hover { filter: brightness(1.06); }
button:disabled { opacity: 0.5; cursor: default; }
@keyframes historyPulse {
  0% { background: color-mix(in srgb, var(--vscode-textLink-foreground) 24%, transparent); }
  100% { background: transparent; }
}
</style>
</head>
<body>
<div id="header"><div id="leftTools"><strong>Copilot Local Chat</strong><span id="profileBadge">profile: -</span><span id="modelsHealth" title="Estado de modelos locales">models: --</span><select id="models"></select></div><div id="rightTools"><label id="codeOnlyWrap"><input id="codeOnlyToggle" type="checkbox"/> Code only</label><span id="focusStats">0 blocks</span><div id="voiceControls"><button id="micBtn" title="Dictado por voz">Mic</button><span id="voiceStatus" data-state="stopped">detenido</span></div><button id="compareBtn">Compare</button><button id="regressionBtn" disabled>Regression Test</button><button id="stopBtn" disabled>Stop</button><button id="clearHistoryBtn">Clear History</button><button id="exportBtn">Export</button></div></div>
<div id="sessionBanner"></div>
<div id="compareView"><div id="compareHeader"><strong>Compare Models</strong><div><span id="compareMeta"></span> <button id="closeCompareBtn">Close</button></div></div><div id="compareGrid"><div class="compareCol"><div class="compareTitle" id="compareLeftTitle">-</div><div class="compareStats" id="compareLeftStats"></div><div class="compareContent" id="compareLeftContent"></div></div><div class="compareCol"><div class="compareTitle" id="compareRightTitle">-</div><div class="compareStats" id="compareRightStats"></div><div class="compareContent" id="compareRightContent"></div></div></div></div>
<div id="messages"></div>
<div id="inputRow"><textarea id="prompt" rows="1" placeholder="Ask something..."></textarea><button id="send">Send</button></div>
<script>
const vscode = acquireVsCodeApi();
const messagesEl = document.getElementById('messages');
const promptEl = document.getElementById('prompt');
const sendBtn = document.getElementById('send');
const exportBtn = document.getElementById('exportBtn');
const compareBtn = document.getElementById('compareBtn');
const regressionBtn = document.getElementById('regressionBtn');
const stopBtn = document.getElementById('stopBtn');
const closeCompareBtn = document.getElementById('closeCompareBtn');
const clearHistoryBtn = document.getElementById('clearHistoryBtn');
const codeOnlyToggleEl = document.getElementById('codeOnlyToggle');
const focusStatsEl = document.getElementById('focusStats');
const modelsEl = document.getElementById('models');
const micBtn = document.getElementById('micBtn');
const voiceStatusEl = document.getElementById('voiceStatus');
const compareViewEl = document.getElementById('compareView');
const compareMetaEl = document.getElementById('compareMeta');
const compareLeftTitleEl = document.getElementById('compareLeftTitle');
const compareLeftStatsEl = document.getElementById('compareLeftStats');
const compareLeftContentEl = document.getElementById('compareLeftContent');
const compareRightTitleEl = document.getElementById('compareRightTitle');
const compareRightStatsEl = document.getElementById('compareRightStats');
const compareRightContentEl = document.getElementById('compareRightContent');
const profileBadgeEl = document.getElementById('profileBadge');
const modelsHealthEl = document.getElementById('modelsHealth');
const sessionBannerEl = document.getElementById('sessionBanner');
const voiceEnabled = ${voiceInputEnabled ? 'true' : 'false'};
let pending = null;
const chatHistory = [];
let recognition = null;
let listening = false;
let historySyncTimer = null;
let codeOnlyMode = false;
let sessionBannerTimer = null;
let regressionPromptDraft = '';

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

function extractCodeBlocks(text) {
  const src = String(text || '');
  const fence = String.fromCharCode(96) + String.fromCharCode(96) + String.fromCharCode(96);
  const re = new RegExp(fence + '([a-zA-Z0-9_-]*)\\\\n([\\\\s\\\\S]*?)' + fence, 'g');
  const blocks = [];
  let m;
  while ((m = re.exec(src)) !== null) {
    blocks.push({
      lang: String(m[1] || 'plaintext').trim() || 'plaintext',
      code: String(m[2] || ''),
    });
  }
  return blocks;
}

function renderCodeOnly(text) {
  const blocks = extractCodeBlocks(text);
  if (blocks.length === 0) {
    return '<span class="focus-empty">No fenced code blocks</span>';
  }

  const langs = [...new Set(blocks.map((b) => b.lang.toLowerCase()))];
  let out = '<div class="focus-meta">' + String(blocks.length) + ' block(s) | ' + sanitizeHTML(langs.join(', ')) + '</div>';
  blocks.forEach((b) => {
    const lang = sanitizeHTML(b.lang || 'plaintext');
    const code = sanitizeHTML(b.code || '');
    out += '<pre><div class="code-actions"><button data-copy="1">Copy</button><button data-apply="1" data-lang="' + lang + '">Apply</button></div><code class="language-' + lang + '">' + code + '</code></pre>';
  });
  return out;
}

function renderMessageContent(text) {
  return codeOnlyMode ? renderCodeOnly(text) : renderMarkdownWithCode(text);
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

function generateMessageId() {
  return Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
}

function scheduleHistorySync() {
  if (historySyncTimer) clearTimeout(historySyncTimer);
  historySyncTimer = setTimeout(() => {
    vscode.postMessage({ type: 'historyUpdate', messages: chatHistory });
  }, 80);
}

function updateFocusStats() {
  let blocks = 0;
  const langs = new Set();
  chatHistory.forEach((m) => {
    const found = extractCodeBlocks(m.content || '');
    blocks += found.length;
    found.forEach((b) => langs.add(String(b.lang || 'plaintext').toLowerCase()));
  });
  const langText = langs.size > 0 ? Array.from(langs).join(', ') : '-';
  focusStatsEl.textContent = String(blocks) + ' blocks | ' + langText;
}

function updateProfileBadge(profile) {
  if (!profileBadgeEl) return;
  const id = String(profile?.id || '-');
  const model = String(profile?.model || '-');
  const lang = String(profile?.lang || '-');
  const temperature = Number(profile?.temperature || 0).toFixed(2);
  profileBadgeEl.textContent = 'profile: ' + id;
  profileBadgeEl.title = 'model=' + model + ' | lang=' + lang + ' | temperature=' + temperature;
}

function updateModelsHealth(status) {
  if (!modelsHealthEl) return;
  const ok = !!status?.ok;
  const count = Number(status?.count || 0);
  const source = String(status?.source || 'unknown');
  if (ok) {
    modelsHealthEl.textContent = 'models: ' + String(count);
    modelsHealthEl.title = 'Modelos locales listos (' + String(count) + ', source=' + source + ')';
    return;
  }
  modelsHealthEl.textContent = 'models: unavailable';
  modelsHealthEl.title = String(status?.error || 'No se pudieron validar modelos locales');
}

function showSessionBanner(text) {
  if (!sessionBannerEl) return;
  if (sessionBannerTimer) clearTimeout(sessionBannerTimer);
  sessionBannerEl.textContent = String(text || 'session restored');
  sessionBannerEl.style.display = 'block';
  sessionBannerTimer = setTimeout(() => {
    sessionBannerEl.style.display = 'none';
  }, 3200);
}

function rerenderChatKeepingScroll() {
  const prevScrollHeight = messagesEl.scrollHeight;
  const ratio = prevScrollHeight > 0 ? (messagesEl.scrollTop / prevScrollHeight) : 0;
  const pendingId = pending?.dataset.msgId || '';

  messagesEl.innerHTML = '';
  for (const item of chatHistory) {
    const d = document.createElement('div');
    d.className = 'msg ' + item.role;
    d.dataset.msgId = item.id;

    const roleEl = document.createElement('span');
    roleEl.className = 'role';
    roleEl.textContent = item.role === 'user' ? 'You:' : 'AI:';
    if (item.role === 'assistant') {
      const starBtn = document.createElement('button');
      starBtn.className = 'starBtn';
      starBtn.textContent = 'Star';
      starBtn.title = 'Guardar en favoritos';
      starBtn.addEventListener('click', () => {
        const current = chatHistory.find((h) => h.id === item.id);
        const content = current?.content || d.querySelector('.msg-content')?.innerText || '';
        if (!content.trim()) return;
        vscode.postMessage({ type: 'favoriteAdd', content });
        starBtn.textContent = 'Starred';
        starBtn.disabled = true;
      });
      d.appendChild(starBtn);
    }

    const contentEl = document.createElement('div');
    contentEl.className = 'msg-content';
    contentEl.innerHTML = renderMessageContent(item.content || '');

    d.appendChild(roleEl);
    d.appendChild(contentEl);
    messagesEl.appendChild(d);
    d.querySelectorAll('pre code').forEach((el) => {
      try {
        if (window.hljs && typeof window.hljs.highlightElement === 'function') {
          window.hljs.highlightElement(el);
        }
      } catch {}
    });
    attachCodeActions(d);
  }

  pending = pendingId ? Array.from(messagesEl.querySelectorAll('.msg')).find((n) => n.dataset.msgId === pendingId) || null : null;
  messagesEl.scrollTop = Math.max(0, Math.round(messagesEl.scrollHeight * ratio));
}

function addMessage(role, text, options = {}) {
  const d = document.createElement('div');
  d.className = 'msg ' + role;
  const msgId = options.id || generateMessageId();
  d.dataset.msgId = msgId;
  const roleEl = document.createElement('span');
  roleEl.className = 'role';
  roleEl.textContent = role === 'user' ? 'You:' : 'AI:';
  if (role === 'assistant') {
    const starBtn = document.createElement('button');
    starBtn.className = 'starBtn';
    starBtn.textContent = 'Star';
    starBtn.title = 'Guardar en favoritos';
    starBtn.addEventListener('click', () => {
      const item = chatHistory.find((h) => h.id === msgId);
      const content = item?.content || d.querySelector('.msg-content')?.innerText || '';
      if (!content.trim()) return;
      vscode.postMessage({ type: 'favoriteAdd', content });
      starBtn.textContent = 'Starred';
      starBtn.disabled = true;
    });
    d.appendChild(starBtn);
  }
  const contentEl = document.createElement('div');
  contentEl.className = 'msg-content';
  contentEl.innerHTML = renderMessageContent(text);
  d.appendChild(roleEl);
  d.appendChild(contentEl);
  messagesEl.appendChild(d);
  d.querySelectorAll('pre code').forEach((el) => {
    try {
      if (window.hljs && typeof window.hljs.highlightElement === 'function') {
        window.hljs.highlightElement(el);
      }
    } catch {}
  });
  attachCodeActions(d);
  chatHistory.push({ id: msgId, role, content: text, timestamp: options.timestamp || Date.now() });
  if (!options.skipSync) scheduleHistorySync();
  updateFocusStats();
  messagesEl.scrollTop = messagesEl.scrollHeight;
  return d;
}

function startAssistant() { pending = addMessage('assistant', ''); return pending; }

function updatePending(text) {
  if (!pending) return;
  const pendingId = pending.dataset.msgId || '';
  const pendingItem = pendingId ? chatHistory.find((h) => h.id === pendingId) : null;
  const prev = pendingItem?.content || pending.querySelector('.msg-content')?.innerText || '';
  const next = prev + text;
  pending.querySelector('.msg-content').innerHTML = renderMessageContent(next);
  if (pendingId) {
    const item = chatHistory.find((h) => h.id === pendingId);
    if (item) {
      item.content = next;
      scheduleHistorySync();
    }
  }
  pending.querySelectorAll('pre code').forEach((el) => {
    try {
      if (window.hljs && typeof window.hljs.highlightElement === 'function') {
        window.hljs.highlightElement(el);
      }
    } catch {}
  });
  attachCodeActions(pending);
  updateFocusStats();
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function resetHistoryUI() {
  messagesEl.innerHTML = '';
  chatHistory.length = 0;
  pending = null;
  updateFocusStats();
}

function hydrateHistory(messages) {
  resetHistoryUI();
  const list = Array.isArray(messages) ? messages : [];
  list.forEach((m) => {
    addMessage(m.role === 'assistant' ? 'assistant' : 'user', String(m.content || ''), {
      id: String(m.id || ''),
      timestamp: Number(m.timestamp || Date.now()),
      skipSync: true,
    });
  });
}

function highlightMessageById(messageId) {
  const id = String(messageId || '').trim();
  if (!id) return;
  const el = Array.from(messagesEl.querySelectorAll('.msg')).find((n) => n.dataset.msgId === id);
  if (!el) return;
  messagesEl.querySelectorAll('.msg.highlighted').forEach((n) => n.classList.remove('highlighted'));
  el.classList.add('highlighted');
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  setTimeout(() => el.classList.remove('highlighted'), 1800);
}

function sendNow(text) {
  if (!text.trim()) return;
  addMessage('user', text);
  sendBtn.disabled = true;
  if (stopBtn) stopBtn.disabled = false;
  if (text.trim().toLowerCase().startsWith('/agent')) {
    const task = text.replace(/^\\/agent\\s*/i, '').trim();
    if (!task) { addMessage('assistant', 'Uso: /agent <descripción de la tarea>'); sendBtn.disabled = false; return; }
    addMessage('assistant', '🤖 Agent Autopilot ejecutando...');
    vscode.postMessage({ type: 'agentChat', task, model: modelsEl.value || '' });
  } else {
    startAssistant();
    vscode.postMessage({ type: 'chat', text, model: modelsEl.value || '' });
  }
}

function setRegressionDraft(text) {
  regressionPromptDraft = String(text || '').trim();
  if (!regressionBtn) return;
  regressionBtn.disabled = !regressionPromptDraft;
  regressionBtn.title = regressionPromptDraft ? 'Generar test de regresion sugerido' : 'Disponible tras Explain Test Failure';
}

function openCompareView() {
  compareViewEl.style.display = 'block';
}

function closeCompareView() {
  compareViewEl.style.display = 'none';
}

function setComparePending() {
  openCompareView();
  compareMetaEl.textContent = 'comparando...';
  compareLeftTitleEl.textContent = '-';
  compareRightTitleEl.textContent = '-';
  compareLeftStatsEl.textContent = '';
  compareRightStatsEl.textContent = '';
  compareLeftContentEl.textContent = '';
  compareRightContentEl.textContent = '';
}

function renderCompareResult(payload) {
  if (!payload) return;
  openCompareView();
  compareMetaEl.textContent = payload.prompt ? ('Prompt length: ' + String(payload.prompt.length)) : '';

  const left = payload.left || {};
  const right = payload.right || {};
  compareLeftTitleEl.textContent = left.model || '-';
  compareLeftStatsEl.textContent = 'time: ' + String(left.elapsedMs || 0) + ' ms | chars: ' + String(left.length || 0);
  compareLeftContentEl.textContent = left.content || '';

  compareRightTitleEl.textContent = right.model || '-';
  compareRightStatsEl.textContent = 'time: ' + String(right.elapsedMs || 0) + ' ms | chars: ' + String(right.length || 0);
  compareRightContentEl.textContent = right.content || '';
}

function setVoiceStatus(state, label) {
  if (!voiceStatusEl) return;
  voiceStatusEl.dataset.state = state;
  voiceStatusEl.textContent = label;
}

function appendTranscript(text) {
  const t = String(text || '').trim();
  if (!t) return;
  const sep = promptEl.value && !/\s$/.test(promptEl.value) ? ' ' : '';
  promptEl.value += sep + t;
  promptEl.focus();
}

function speechCtor() {
  return window.SpeechRecognition || window.webkitSpeechRecognition;
}

function initVoiceInput() {
  if (!voiceEnabled) {
    if (micBtn) micBtn.style.display = 'none';
    if (voiceStatusEl) voiceStatusEl.style.display = 'none';
    return;
  }

  const Ctor = speechCtor();
  if (typeof Ctor !== 'function') {
    if (micBtn) micBtn.disabled = true;
    setVoiceStatus('unavailable', 'no disponible');
    return;
  }

  recognition = new Ctor();
  recognition.continuous = true;
  recognition.interimResults = true;
  recognition.lang = navigator.language || 'es-ES';

  recognition.onstart = () => {
    listening = true;
    setVoiceStatus('listening', 'escuchando');
  };

  recognition.onend = () => {
    listening = false;
    setVoiceStatus('stopped', 'detenido');
  };

  recognition.onerror = () => {
    listening = false;
    setVoiceStatus('unavailable', 'error');
  };

  recognition.onresult = (event) => {
    let finalText = '';
    for (let i = event.resultIndex; i < event.results.length; i++) {
      const result = event.results[i];
      if (result.isFinal && result[0] && result[0].transcript) {
        finalText += result[0].transcript + ' ';
      }
    }
    appendTranscript(finalText);
  };

  if (micBtn) {
    micBtn.addEventListener('click', () => {
      try {
        if (!listening) recognition.start();
        else recognition.stop();
      } catch {
        setVoiceStatus('unavailable', 'error');
      }
    });
  }
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

compareBtn.addEventListener('click', () => {
  const prompt = promptEl.value.trim();
  if (!prompt) return;
  setComparePending();
  vscode.postMessage({ type: 'compare', text: prompt });
});

regressionBtn.addEventListener('click', () => {
  if (!regressionPromptDraft) return;
  sendNow(regressionPromptDraft);
});

stopBtn.addEventListener('click', () => {
  vscode.postMessage({ type: 'cancelChat' });
  if (stopBtn) stopBtn.disabled = true;
});

modelsEl.addEventListener('change', () => {
  vscode.postMessage({ type: 'modelSelected', model: modelsEl.value || '' });
});

closeCompareBtn.addEventListener('click', () => {
  closeCompareView();
});

clearHistoryBtn.addEventListener('click', () => {
  vscode.postMessage({ type: 'clearHistoryRequest' });
});

codeOnlyToggleEl.addEventListener('change', () => {
  codeOnlyMode = !!codeOnlyToggleEl.checked;
  rerenderChatKeepingScroll();
});

initVoiceInput();
updateFocusStats();

window.addEventListener('message', (e) => {
  const msg = e.data;
  if (msg.type === 'chunk') { updatePending(msg.text || ''); return; }
  if (msg.type === 'done') { pending = null; sendBtn.disabled = false; if (stopBtn) stopBtn.disabled = true; return; }
  if (msg.type === 'error') { updatePending('\\n[Error: ' + (msg.text || 'unknown') + ']'); pending = null; sendBtn.disabled = false; if (stopBtn) stopBtn.disabled = true; return; }
  if (msg.type === 'agentThinking') { addMessage('assistant', '💭 ' + (msg.content || '')); return; }
  if (msg.type === 'agentToolCall') { addMessage('assistant', '🔧 ' + (msg.tool || '') + '(' + JSON.stringify(msg.args || {}) + ')'); return; }
  if (msg.type === 'agentToolResult') { addMessage('assistant', (msg.success ? '✅' : '❌') + ' ' + (msg.output || '').substring(0, 500)); return; }
  if (msg.type === 'agentAnswer') { addMessage('assistant', msg.content || ''); return; }
  if (msg.type === 'agentDone') { sendBtn.disabled = false; if (stopBtn) stopBtn.disabled = true; return; }
  if (msg.type === 'agentError') { addMessage('assistant', '[Error: ' + (msg.text || 'unknown') + ']'); sendBtn.disabled = false; if (stopBtn) stopBtn.disabled = true; return; }
  if (msg.type === 'prefill') { promptEl.value = msg.text || ''; promptEl.focus(); return; }
  if (msg.type === 'runPrompt') { sendNow(msg.text || ''); return; }
  if (msg.type === 'externalResult') { addMessage('assistant', msg.text || ''); return; }
  if (msg.type === 'hydrateHistory') { hydrateHistory(msg.messages || []); return; }
  if (msg.type === 'highlightHistory') { highlightMessageById(msg.id || ''); return; }
  if (msg.type === 'historyCleared') { resetHistoryUI(); return; }
  if (msg.type === 'compareStart') { setComparePending(); return; }
  if (msg.type === 'compareResult') { renderCompareResult(msg); return; }
  if (msg.type === 'openCompareMode') { openCompareView(); return; }
  if (msg.type === 'models') {
    const models = Array.isArray(msg.models) ? msg.models : [];
    const current = msg.current || '';
    modelsEl.innerHTML = '';
    const arr = models.length > 0 ? models : [current || 'local-rag'];
    arr.forEach((m) => { const opt = document.createElement('option'); opt.value = m; opt.textContent = m; modelsEl.appendChild(opt); });
    modelsEl.value = arr.includes(current) ? current : arr[0];
    return;
  }
  if (msg.type === 'profileUpdate') {
    updateProfileBadge(msg.profile || {});
    return;
  }
  if (msg.type === 'modelsValidation') {
    updateModelsHealth(msg);
    return;
  }
  if (msg.type === 'sessionRestored') {
    showSessionBanner(msg.text || 'session restored');
    return;
  }
  if (msg.type === 'regressionDraft') {
    setRegressionDraft(msg.text || '');
  }
});
</script>
</body>
</html>`;
}

function getChatSidebarHTML(fontSize, voiceInputEnabled) {
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --chat-font: ${fontSize || 13}px;
  --accent: var(--vscode-textLink-foreground);
  --muted: var(--vscode-descriptionForeground);
  --border: var(--vscode-panel-border);
  --msg-user-bg: color-mix(in srgb, var(--vscode-textLink-foreground) 8%, transparent);
  --msg-ai-bg: transparent;
  --code-bg: color-mix(in srgb, var(--vscode-editor-background) 82%, #000 18%);
  --hover-bg: color-mix(in srgb, var(--vscode-textLink-foreground) 6%, transparent);
}
body { font-family: var(--vscode-font-family); font-size: var(--chat-font); color: var(--vscode-editor-foreground); background: var(--vscode-editor-background); height: 100vh; display: flex; flex-direction: column; overflow: hidden; }

/* ── Header ── */
#header { display: flex; align-items: center; justify-content: space-between; padding: 4px 10px; border-bottom: 1px solid var(--border); min-height: 32px; }
#leftTools { display: flex; align-items: center; gap: 6px; }
#rightTools { display: flex; align-items: center; gap: 2px; }
#profileBadge { font-size: 10px; padding: 1px 6px; border: 1px solid var(--accent); border-radius: 10px; color: var(--accent); cursor: pointer; white-space: nowrap; }
#modelsHealth { font-size: 10px; color: var(--muted); white-space: nowrap; }
select#models { background: var(--vscode-dropdown-background); color: var(--vscode-dropdown-foreground); border: 1px solid var(--vscode-dropdown-border); border-radius: 4px; padding: 2px 4px; font-size: 11px; max-width: 130px; }
.hdr-btn { background: none; border: none; color: var(--muted); cursor: pointer; padding: 3px 4px; border-radius: 4px; font-size: 13px; line-height: 1; display: flex; align-items: center; }
.hdr-btn:hover { background: var(--hover-bg); color: var(--vscode-editor-foreground); }
.hdr-btn.active { color: var(--accent); }
#sessionBanner { display: none; margin: 4px 10px 0; padding: 6px 8px; border: 1px solid var(--accent); border-radius: 6px; color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); font-size: 11px; }

/* ── Compare ── */
#compareView { display: none; border-bottom: 1px solid var(--border); padding: 8px; }
#compareHeader { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 12px; }
#compareMeta { font-size: 11px; color: var(--muted); }
#compareGrid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.compareCol { border: 1px solid var(--border); border-radius: 6px; padding: 6px; min-height: 80px; background: var(--code-bg); font-size: 12px; }
.compareTitle { font-weight: 700; margin-bottom: 2px; }
.compareStats { font-size: 10px; color: var(--muted); margin-bottom: 4px; }
.compareContent { white-space: pre-wrap; line-height: 1.35; }

/* ── Welcome ── */
#welcome { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; padding: 20px; text-align: center; }
#welcome.hidden { display: none; }
.welcome-icon { font-size: 40px; line-height: 1; opacity: 0.5; }
.welcome-title { font-size: 16px; font-weight: 600; }
.welcome-subtitle { font-size: 12px; color: var(--muted); max-width: 280px; line-height: 1.5; }
.welcome-suggestions { display: flex; flex-direction: column; gap: 6px; width: 100%; max-width: 280px; }
.suggest-btn { background: var(--code-bg); border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; cursor: pointer; text-align: left; font-size: 12px; color: var(--vscode-editor-foreground); transition: background 0.15s; }
.suggest-btn:hover { background: var(--hover-bg); border-color: var(--accent); }
.suggest-btn .suggest-label { font-weight: 600; display: block; margin-bottom: 2px; }
.suggest-btn .suggest-desc { color: var(--muted); font-size: 11px; }

/* ── Messages ── */
#messages { flex: 1; overflow-y: auto; padding: 8px 10px; display: none; }
#messages.visible { display: block; }
.msg { margin-bottom: 14px; line-height: 1.55; word-wrap: break-word; padding: 8px 10px; border-radius: 10px; }
.msg.user { background: var(--msg-user-bg); }
.msg.assistant { background: var(--msg-ai-bg); }
.msg.highlighted { outline: 2px solid var(--accent); animation: historyPulse 1.5s ease-in-out 1; }
.msg-header { display: flex; align-items: center; gap: 6px; margin-bottom: 4px; }
.msg-avatar { width: 20px; height: 20px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 11px; flex-shrink: 0; }
.msg.user .msg-avatar { background: var(--accent); color: #fff; }
.msg.assistant .msg-avatar { background: color-mix(in srgb, var(--accent) 20%, transparent); color: var(--accent); }
.msg-role { font-weight: 600; font-size: 12px; }
.msg-actions { margin-left: auto; display: flex; gap: 2px; opacity: 0; transition: opacity 0.15s; }
.msg:hover .msg-actions { opacity: 1; }
.msg-actions button { background: none; border: none; color: var(--muted); cursor: pointer; padding: 2px 4px; border-radius: 3px; font-size: 12px; }
.msg-actions button:hover { color: var(--accent); background: var(--hover-bg); }
.msg-content { white-space: pre-wrap; }
.focus-meta { font-size: 11px; color: var(--muted); margin-bottom: 4px; }
.focus-empty { color: var(--muted); font-style: italic; }

/* ── Code blocks ── */
pre { background: var(--code-bg); border: 1px solid var(--border); border-radius: 8px; padding: 10px; overflow-x: auto; margin: 6px 0; position: relative; }
pre code { font-family: var(--vscode-editor-font-family); font-size: 0.92em; line-height: 1.45; }
.code-header { display: flex; align-items: center; justify-content: space-between; padding: 4px 10px; background: color-mix(in srgb, var(--border) 30%, transparent); border-radius: 8px 8px 0 0; margin: 6px 0 0; font-size: 11px; color: var(--muted); }
.code-actions { position: absolute; top: 4px; right: 4px; display: flex; gap: 3px; opacity: 0; transition: opacity 0.15s; }
pre:hover .code-actions { opacity: 1; }
.code-actions button { padding: 2px 8px; font-size: 11px; background: var(--vscode-button-secondaryBackground); color: var(--vscode-button-secondaryForeground); border: 1px solid var(--border); border-radius: 4px; cursor: pointer; }
.code-actions button:hover { background: var(--vscode-button-background); color: var(--vscode-button-foreground); }

/* ── Slash command popup ── */
#slashPopup { display: none; position: absolute; bottom: 100%; left: 0; right: 0; background: var(--vscode-editorWidget-background, var(--vscode-editor-background)); border: 1px solid var(--border); border-radius: 6px; margin-bottom: 4px; max-height: 180px; overflow-y: auto; box-shadow: 0 2px 8px rgba(0,0,0,0.2); z-index: 10; }
#slashPopup.show { display: block; }
.slash-item { padding: 6px 10px; cursor: pointer; display: flex; align-items: center; gap: 8px; font-size: 12px; }
.slash-item:hover, .slash-item.active { background: var(--hover-bg); }
.slash-cmd { font-weight: 600; color: var(--accent); min-width: 80px; }
.slash-desc { color: var(--muted); }

/* ── Context chips ── */
#contextChips { display: flex; gap: 4px; flex-wrap: wrap; padding: 0 10px; }
#contextChips:empty { display: none; }
.ctx-chip { display: inline-flex; align-items: center; gap: 3px; padding: 2px 8px; font-size: 11px; border: 1px solid var(--border); border-radius: 12px; background: var(--code-bg); color: var(--muted); }
.ctx-chip .remove { cursor: pointer; font-size: 13px; color: var(--muted); margin-left: 2px; }
.ctx-chip .remove:hover { color: var(--accent); }

/* ── Input ── */
#inputArea { position: relative; border-top: 1px solid var(--border); padding: 8px 10px; background: var(--vscode-editor-background); }
#inputRow { display: flex; align-items: flex-end; gap: 6px; }
#prompt { flex: 1; background: var(--chat-input-bg, var(--vscode-input-background)); color: var(--chat-input-fg, var(--vscode-input-foreground)); border: 1px solid var(--chat-input-border, var(--vscode-input-border)); border-radius: 8px; padding: 8px 10px; min-height: 36px; max-height: 140px; resize: none; font-family: inherit; font-size: var(--chat-font); line-height: 1.4; outline: none; }
#prompt:focus { border-color: var(--accent); }
#prompt::placeholder { color: var(--muted); }
#sendBtn { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; border-radius: 8px; padding: 8px 10px; cursor: pointer; display: flex; align-items: center; font-size: 14px; }
#sendBtn:hover { filter: brightness(1.08); }
#sendBtn:disabled { opacity: 0.4; cursor: default; }
#stopBtn { display: none; background: var(--vscode-errorForeground); color: #fff; border: none; border-radius: 8px; padding: 8px 10px; cursor: pointer; font-size: 14px; }
.input-hint { font-size: 11px; color: var(--muted); padding: 4px 0 0; }
.input-hint kbd { background: var(--code-bg); padding: 1px 4px; border-radius: 3px; font-size: 10px; border: 1px solid var(--border); }
#voiceControls { display: inline-flex; align-items: center; gap: 3px; }
#micBtn { background: none; border: 1px solid var(--border); border-radius: 8px; padding: 6px 8px; cursor: pointer; color: var(--muted); font-size: 13px; }
#micBtn:hover { border-color: var(--accent); color: var(--accent); }
#voiceStatus { font-size: 10px; color: var(--muted); }
#voiceStatus[data-state="listening"] { color: var(--vscode-testing-iconPassed); }

button.secondary { background: var(--vscode-button-secondaryBackground); color: var(--vscode-button-secondaryForeground); border: 1px solid var(--border); }

/* ── Agent Autopilot Steps ── */
.agent-steps { display: flex; flex-direction: column; gap: 6px; margin-top: 6px; }
.agent-step { border: 1px solid var(--border); border-radius: 8px; overflow: hidden; font-size: 12px; }
.agent-step-header { display: flex; align-items: center; gap: 6px; padding: 6px 10px; background: var(--code-bg); cursor: pointer; user-select: none; }
.agent-step-header:hover { background: var(--hover-bg); }
.agent-step-icon { font-size: 13px; flex-shrink: 0; }
.agent-step-label { flex: 1; font-weight: 600; }
.agent-step-badge { font-size: 10px; padding: 1px 6px; border-radius: 8px; }
.agent-step-badge.success { background: color-mix(in srgb, var(--vscode-testing-iconPassed) 18%, transparent); color: var(--vscode-testing-iconPassed); }
.agent-step-badge.error { background: color-mix(in srgb, var(--vscode-errorForeground) 18%, transparent); color: var(--vscode-errorForeground); }
.agent-step-badge.running { background: color-mix(in srgb, var(--accent) 18%, transparent); color: var(--accent); }
.agent-step-body { display: none; padding: 6px 10px; border-top: 1px solid var(--border); }
.agent-step-body.open { display: block; }
.agent-step-body pre { margin: 4px 0; font-size: 11px; max-height: 200px; overflow-y: auto; }
.agent-thinking { color: var(--muted); font-style: italic; padding: 4px 0; }
.agent-answer { padding: 6px 0; }
.agent-progress { display: flex; align-items: center; gap: 6px; padding: 4px 0; font-size: 11px; color: var(--muted); }
.agent-spinner { display: inline-block; width: 12px; height: 12px; border: 2px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: agentSpin 0.8s linear infinite; }
@keyframes agentSpin { to { transform: rotate(360deg); } }

.agent-file-op { background: color-mix(in srgb, var(--accent) 8%, transparent); border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px; margin: 6px 0; }
.agent-file-op-header { display: flex; align-items: center; gap: 6px; font-size: 11px; font-weight: 600; }
.agent-file-op-header .icon { font-size: 14px; }
.agent-file-op-path { font-family: var(--vscode-editor-font-family, monospace); font-size: 11px; color: var(--accent); word-break: break-all; }
.agent-file-op-actions { display: flex; gap: 6px; margin-top: 6px; }
.agent-file-op-actions button { font-size: 11px; padding: 3px 10px; border-radius: 4px; border: 1px solid var(--border); cursor: pointer; background: var(--vscode-button-secondaryBackground, transparent); color: var(--fg); }
.agent-file-op-actions .accept-btn { background: color-mix(in srgb, var(--vscode-testing-iconPassed) 18%, transparent); color: var(--vscode-testing-iconPassed); border-color: var(--vscode-testing-iconPassed); }
.agent-file-op-actions .reject-btn { background: color-mix(in srgb, var(--vscode-errorForeground) 18%, transparent); color: var(--vscode-errorForeground); border-color: var(--vscode-errorForeground); }
.agent-file-op-status { font-size: 10px; margin-top: 4px; font-style: italic; color: var(--muted); }
.agent-file-op-status.accepted { color: var(--vscode-testing-iconPassed); }
.agent-file-op-status.rejected { color: var(--vscode-errorForeground); }
#addFileBtn { background: none; border: 1px solid var(--border); color: var(--muted); border-radius: 4px; padding: 2px 8px; cursor: pointer; font-size: 11px; white-space: nowrap; }
#addFileBtn:hover { color: var(--fg); border-color: var(--accent); }

@keyframes historyPulse {
  0% { background: color-mix(in srgb, var(--accent) 24%, transparent); }
  100% { background: transparent; }
}
@keyframes fadeIn { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }
.msg { animation: fadeIn 0.2s ease-out; }
</style>
</head>
<body>

<div id="header">
  <div id="leftTools">
    <select id="models" title="Modelo activo"></select>
    <span id="profileBadge" title="Click para cambiar perfil">balanced</span>
    <span id="modelsHealth" title="Estado de modelos locales"></span>
  </div>
  <div id="rightTools">
    <button class="hdr-btn" id="codeOnlyToggle" title="Code-only mode">{ }</button>
    <button class="hdr-btn" id="compareBtn" title="Compare models">⇄</button>
    <button class="hdr-btn" id="exportBtn" title="Export chat">↗</button>
  </div>
</div>

<div id="sessionBanner"></div>

<div id="compareView">
  <div id="compareHeader"><strong>Compare Models</strong><div><span id="compareMeta"></span> <button class="hdr-btn" id="closeCompareBtn" title="Close compare">✕</button></div></div>
  <div id="compareGrid">
    <div class="compareCol"><div class="compareTitle" id="compareLeftTitle">-</div><div class="compareStats" id="compareLeftStats"></div><div class="compareContent" id="compareLeftContent"></div></div>
    <div class="compareCol"><div class="compareTitle" id="compareRightTitle">-</div><div class="compareStats" id="compareRightStats"></div><div class="compareContent" id="compareRightContent"></div></div>
  </div>
</div>

<div id="welcome">
  <div class="welcome-icon">✦</div>
  <div class="welcome-title">Copilot Local</div>
  <div class="welcome-subtitle">Tu asistente de IA local. Haz preguntas sobre tu código, genera tests, refactoriza y más.</div>
  <div class="welcome-suggestions">
    <button class="suggest-btn" data-prompt="/explain"><span class="suggest-label">✦ Explain Code</span><span class="suggest-desc">Selecciona código y pide una explicación</span></button>
    <button class="suggest-btn" data-prompt="/test"><span class="suggest-label">🧪 Generate Tests</span><span class="suggest-desc">Crea tests para el código seleccionado</span></button>
    <button class="suggest-btn" data-prompt="/refactor"><span class="suggest-label">🔧 Refactor</span><span class="suggest-desc">Mejora la claridad y mantenibilidad</span></button>
    <button class="suggest-btn" data-prompt="/fix"><span class="suggest-label">🐛 Fix Errors</span><span class="suggest-desc">Corrige errores en el código</span></button>
    <button class="suggest-btn" data-prompt="/agent"><span class="suggest-label">🤖 Agent Autopilot</span><span class="suggest-desc">Ejecuta tareas complejas paso a paso</span></button>
  </div>
</div>

<div id="messages"></div>

<div id="contextChips"></div>

<div id="inputArea">
  <div id="slashPopup"></div>
  <div id="inputRow">
    <button id="addFileBtn" title="Agregar archivo al contexto">+ 📄</button>
    <div id="voiceControls">
      <button id="micBtn" title="Dictado por voz">🎙</button>
      <span id="voiceStatus" data-state="stopped"></span>
    </div>
    <textarea id="prompt" rows="1" placeholder="Pregunta algo o usa /explain, /test, /fix..."></textarea>
    <button id="sendBtn" title="Enviar (Enter)">▶</button>
    <button id="stopBtn" title="Detener generación">■</button>
  </div>
  <div class="input-hint"><kbd>Enter</kbd> enviar · <kbd>Shift+Enter</kbd> nueva línea · <kbd>/</kbd> comandos</div>
</div>

<script>
const vscode = acquireVsCodeApi();
const messagesEl = document.getElementById('messages');
const welcomeEl = document.getElementById('welcome');
const promptEl = document.getElementById('prompt');
const sendBtn = document.getElementById('sendBtn');
const exportBtn = document.getElementById('exportBtn');
const compareBtn = document.getElementById('compareBtn');
const stopBtn = document.getElementById('stopBtn');
const closeCompareBtn = document.getElementById('closeCompareBtn');
const codeOnlyToggleEl = document.getElementById('codeOnlyToggle');
const modelsEl = document.getElementById('models');
const micBtn = document.getElementById('micBtn');
const voiceStatusEl = document.getElementById('voiceStatus');
const compareViewEl = document.getElementById('compareView');
const compareMetaEl = document.getElementById('compareMeta');
const compareLeftTitleEl = document.getElementById('compareLeftTitle');
const compareLeftStatsEl = document.getElementById('compareLeftStats');
const compareLeftContentEl = document.getElementById('compareLeftContent');
const compareRightTitleEl = document.getElementById('compareRightTitle');
const compareRightStatsEl = document.getElementById('compareRightStats');
const compareRightContentEl = document.getElementById('compareRightContent');
const profileBadgeEl = document.getElementById('profileBadge');
const modelsHealthEl = document.getElementById('modelsHealth');
const sessionBannerEl = document.getElementById('sessionBanner');
const slashPopupEl = document.getElementById('slashPopup');
const contextChipsEl = document.getElementById('contextChips');
const addFileBtnEl = document.getElementById('addFileBtn');
const voiceEnabled = ${voiceInputEnabled ? 'true' : 'false'};
let pending = null;
const chatHistory = [];
let recognition = null;
let listening = false;
let historySyncTimer = null;
let codeOnlyMode = false;
let sessionBannerTimer = null;
let regressionPromptDraft = '';
let slashActive = -1;

const SLASH_COMMANDS = [
  { cmd: '/explain', desc: 'Explica el código seleccionado' },
  { cmd: '/test', desc: 'Genera tests para el código' },
  { cmd: '/fix', desc: 'Corrige errores en el código' },
  { cmd: '/refactor', desc: 'Refactoriza para mayor claridad' },
  { cmd: '/doc', desc: 'Agrega documentación/docstrings' },
  { cmd: '/agent', desc: 'Modo autopilot: ejecuta tareas paso a paso' },
];

function sanitizeHTML(text) {
  return String(text || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\\"/g, '&quot;').replace(/'/g, '&#39;');
}

function showWelcome() { welcomeEl.classList.remove('hidden'); messagesEl.classList.remove('visible'); }
function hideWelcome() { welcomeEl.classList.add('hidden'); messagesEl.classList.add('visible'); }

function renderMarkdownWithCode(text) {
  const src = String(text || '');
  const fence = String.fromCharCode(96) + String.fromCharCode(96) + String.fromCharCode(96);
  const re = new RegExp(fence + '([a-zA-Z0-9_-]*)\\\\n([\\\\s\\\\S]*?)' + fence, 'g');
  let i = 0, out = '', m;
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

function extractCodeBlocks(text) {
  const src = String(text || '');
  const fence = String.fromCharCode(96) + String.fromCharCode(96) + String.fromCharCode(96);
  const re = new RegExp(fence + '([a-zA-Z0-9_-]*)\\\\n([\\\\s\\\\S]*?)' + fence, 'g');
  const blocks = [];
  let m;
  while ((m = re.exec(src)) !== null) {
    blocks.push({ lang: String(m[1] || 'plaintext').trim() || 'plaintext', code: String(m[2] || '') });
  }
  return blocks;
}

function renderCodeOnly(text) {
  const blocks = extractCodeBlocks(text);
  if (blocks.length === 0) return '<span class="focus-empty">No fenced code blocks</span>';
  const langs = [...new Set(blocks.map((b) => b.lang.toLowerCase()))];
  let out = '<div class="focus-meta">' + blocks.length + ' block(s) | ' + sanitizeHTML(langs.join(', ')) + '</div>';
  blocks.forEach((b) => {
    const lang = sanitizeHTML(b.lang || 'plaintext');
    const code = sanitizeHTML(b.code || '');
    out += '<pre><div class="code-actions"><button data-copy="1">Copy</button><button data-apply="1" data-lang="' + lang + '">Apply</button></div><code class="language-' + lang + '">' + code + '</code></pre>';
  });
  return out;
}

function renderMessageContent(text) {
  return codeOnlyMode ? renderCodeOnly(text) : renderMarkdownWithCode(text);
}

function attachCodeActions(root) {
  root.querySelectorAll('button[data-copy]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const code = btn.closest('pre')?.querySelector('code')?.innerText || '';
      await navigator.clipboard.writeText(code);
      btn.textContent = '✓';
      setTimeout(() => { btn.textContent = 'Copy'; }, 1200);
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

function generateMessageId() {
  return Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
}

function scheduleHistorySync() {
  if (historySyncTimer) clearTimeout(historySyncTimer);
  historySyncTimer = setTimeout(() => {
    vscode.postMessage({ type: 'historyUpdate', messages: chatHistory });
  }, 80);
}

function updateFocusStats() {}

function updateProfileBadge(profile) {
  if (!profileBadgeEl) return;
  const id = String(profile?.id || 'balanced');
  profileBadgeEl.textContent = id;
  profileBadgeEl.title = 'model=' + String(profile?.model || '-') + ' | lang=' + String(profile?.lang || '-') + ' | temp=' + Number(profile?.temperature || 0).toFixed(2) + ' (click to switch)';
}

function updateModelsHealth(status) {
  if (!modelsHealthEl) return;
  const ok = !!status?.ok;
  const count = Number(status?.count || 0);
  if (ok) {
    modelsHealthEl.textContent = count + ' models';
    modelsHealthEl.title = 'Modelos locales listos (' + count + ')';
    modelsHealthEl.style.color = '';
    return;
  }
  modelsHealthEl.textContent = '⚠ models';
  modelsHealthEl.title = String(status?.error || 'Modelos no disponibles');
  modelsHealthEl.style.color = 'var(--vscode-errorForeground)';
}

function showSessionBanner(text) {
  if (!sessionBannerEl) return;
  if (sessionBannerTimer) clearTimeout(sessionBannerTimer);
  sessionBannerEl.textContent = String(text || 'session restored');
  sessionBannerEl.style.display = 'block';
  sessionBannerTimer = setTimeout(() => { sessionBannerEl.style.display = 'none'; }, 3200);
}

function rerenderChatKeepingScroll() {
  const ratio = messagesEl.scrollHeight > 0 ? (messagesEl.scrollTop / messagesEl.scrollHeight) : 0;
  const pendingId = pending?.dataset.msgId || '';
  messagesEl.innerHTML = '';
  for (const item of chatHistory) {
    const d = createMessageEl(item.role, item.content, item.id);
    messagesEl.appendChild(d);
  }
  pending = pendingId ? messagesEl.querySelector('[data-msg-id="' + pendingId + '"]') || null : null;
  messagesEl.scrollTop = Math.max(0, Math.round(messagesEl.scrollHeight * ratio));
}

function createMessageEl(role, text, msgId) {
  const d = document.createElement('div');
  d.className = 'msg ' + role;
  d.dataset.msgId = msgId || generateMessageId();

  const header = document.createElement('div');
  header.className = 'msg-header';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  avatar.textContent = role === 'user' ? '👤' : '✦';

  const roleLabel = document.createElement('span');
  roleLabel.className = 'msg-role';
  roleLabel.textContent = role === 'user' ? 'You' : 'Copilot Local';

  header.appendChild(avatar);
  header.appendChild(roleLabel);

  if (role === 'assistant') {
    const actions = document.createElement('div');
    actions.className = 'msg-actions';
    const copyBtn = document.createElement('button');
    copyBtn.textContent = '📋';
    copyBtn.title = 'Copy response';
    copyBtn.addEventListener('click', () => {
      const item = chatHistory.find((h) => h.id === d.dataset.msgId);
      navigator.clipboard.writeText(item?.content || d.querySelector('.msg-content')?.innerText || '');
      copyBtn.textContent = '✓';
      setTimeout(() => { copyBtn.textContent = '📋'; }, 1200);
    });
    const starBtn = document.createElement('button');
    starBtn.textContent = '⭐';
    starBtn.title = 'Guardar en favoritos';
    starBtn.addEventListener('click', () => {
      const item = chatHistory.find((h) => h.id === d.dataset.msgId);
      const content = item?.content || d.querySelector('.msg-content')?.innerText || '';
      if (!content.trim()) return;
      vscode.postMessage({ type: 'favoriteAdd', content });
      starBtn.textContent = '★';
    });
    actions.appendChild(copyBtn);
    actions.appendChild(starBtn);
    header.appendChild(actions);
  }

  const contentEl = document.createElement('div');
  contentEl.className = 'msg-content';
  contentEl.innerHTML = renderMessageContent(text);

  d.appendChild(header);
  d.appendChild(contentEl);
  d.querySelectorAll('pre code').forEach((el) => {
    try { if (window.hljs) window.hljs.highlightElement(el); } catch {}
  });
  attachCodeActions(d);
  return d;
}

function addMessage(role, text, options = {}) {
  hideWelcome();
  const msgId = options.id || generateMessageId();
  const d = createMessageEl(role, text, msgId);
  messagesEl.appendChild(d);
  chatHistory.push({ id: msgId, role, content: text, timestamp: options.timestamp || Date.now() });
  if (!options.skipSync) scheduleHistorySync();
  messagesEl.scrollTop = messagesEl.scrollHeight;
  return d;
}

function startAssistant() { pending = addMessage('assistant', ''); return pending; }

function updatePending(text) {
  if (!pending) return;
  const pendingId = pending.dataset.msgId || '';
  const item = pendingId ? chatHistory.find((h) => h.id === pendingId) : null;
  const prev = item?.content || '';
  const next = prev + text;
  const contentEl = pending.querySelector('.msg-content');
  if (contentEl) contentEl.innerHTML = renderMessageContent(next);
  if (item) { item.content = next; scheduleHistorySync(); }
  pending.querySelectorAll('pre code').forEach((el) => {
    try { if (window.hljs) window.hljs.highlightElement(el); } catch {}
  });
  attachCodeActions(pending);
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

let agentPending = null;
let agentStepsContainer = null;

function startAgentAssistant() {
  hideWelcome();
  const msgId = generateMessageId();
  const d = document.createElement('div');
  d.className = 'msg assistant';
  d.dataset.msgId = msgId;

  const header = document.createElement('div');
  header.className = 'msg-header';
  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  avatar.textContent = '🤖';
  const roleLabel = document.createElement('span');
  roleLabel.className = 'msg-role';
  roleLabel.textContent = 'Agent Autopilot';
  header.appendChild(avatar);
  header.appendChild(roleLabel);

  const contentEl = document.createElement('div');
  contentEl.className = 'msg-content';

  const progress = document.createElement('div');
  progress.className = 'agent-progress';
  progress.id = 'agentProgress';
  progress.innerHTML = '<span class="agent-spinner"></span> Ejecutando...';

  const steps = document.createElement('div');
  steps.className = 'agent-steps';
  steps.id = 'agentSteps';

  contentEl.appendChild(progress);
  contentEl.appendChild(steps);
  d.appendChild(header);
  d.appendChild(contentEl);
  messagesEl.appendChild(d);
  chatHistory.push({ id: msgId, role: 'assistant', content: '[Agent Autopilot]', timestamp: Date.now() });
  scheduleHistorySync();
  messagesEl.scrollTop = messagesEl.scrollHeight;

  agentPending = d;
  agentStepsContainer = steps;
  return d;
}

function addAgentThinking(iteration, content) {
  if (!agentStepsContainer) return;
  const step = document.createElement('div');
  step.className = 'agent-step';
  step.innerHTML = '<div class="agent-step-header"><span class="agent-step-icon">💭</span><span class="agent-step-label">Paso ' + iteration + ': Pensando</span><span class="agent-step-badge running">thinking</span></div>' +
    '<div class="agent-step-body open"><div class="agent-thinking">' + sanitizeHTML(content) + '</div></div>';
  agentStepsContainer.appendChild(step);
  step.querySelector('.agent-step-header').addEventListener('click', () => step.querySelector('.agent-step-body').classList.toggle('open'));
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function addAgentToolCall(iteration, tool, args) {
  if (!agentStepsContainer) return;
  const argsStr = args ? Object.entries(args).map(([k,v]) => sanitizeHTML(k) + '=' + sanitizeHTML(String(v).substring(0, 80))).join(', ') : '';
  const step = document.createElement('div');
  step.className = 'agent-step';
  step.dataset.iteration = iteration;
  step.dataset.tool = tool;
  step.innerHTML = '<div class="agent-step-header"><span class="agent-step-icon">🔧</span><span class="agent-step-label">' + sanitizeHTML(tool) + '(' + argsStr + ')</span><span class="agent-step-badge running">running</span></div>' +
    '<div class="agent-step-body"></div>';
  agentStepsContainer.appendChild(step);
  step.querySelector('.agent-step-header').addEventListener('click', () => step.querySelector('.agent-step-body').classList.toggle('open'));
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function addAgentToolResult(iteration, tool, success, output) {
  if (!agentStepsContainer) return;
  const existing = agentStepsContainer.querySelector('[data-iteration="' + iteration + '"][data-tool="' + tool + '"]');
  const target = existing || agentStepsContainer.lastElementChild;
  if (!target) return;
  const badge = target.querySelector('.agent-step-badge');
  if (badge) { badge.className = 'agent-step-badge ' + (success ? 'success' : 'error'); badge.textContent = success ? 'ok' : 'error'; }
  const body = target.querySelector('.agent-step-body');
  if (body) {
    const truncated = String(output || '').substring(0, 2000);
    body.innerHTML = '<pre><code>' + sanitizeHTML(truncated) + '</code></pre>';
    body.classList.add('open');
  }
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function addAgentAnswer(content) {
  if (!agentStepsContainer) return;
  const progressEl = agentPending?.querySelector('#agentProgress');
  if (progressEl) progressEl.remove();
  const answer = document.createElement('div');
  answer.className = 'agent-answer';
  answer.innerHTML = renderMarkdownWithCode(content);
  attachCodeActions(answer);
  agentStepsContainer.parentElement.appendChild(answer);
  const item = chatHistory.find((h) => h.id === agentPending?.dataset.msgId);
  if (item) { item.content = '[Agent Autopilot]\\n' + content; scheduleHistorySync(); }
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function finishAgent() {
  const progressEl = agentPending?.querySelector('#agentProgress');
  if (progressEl) progressEl.remove();
  agentPending = null;
  agentStepsContainer = null;
  sendBtn.disabled = false;
  sendBtn.style.display = 'flex';
  stopBtn.style.display = 'none';
}

let fileOpCounter = 0;
function addAgentFileOp(type, iteration, filePath, opId) {
  if (!agentStepsContainer) return null;
  if (!opId) opId = 'fileop-' + (++fileOpCounter);
  const icon = type === 'create' ? '🆕' : '✏️';
  const label = type === 'create' ? 'Crear archivo' : 'Modificar archivo';
  const step = document.createElement('div');
  step.className = 'agent-step';
  step.innerHTML =
    '<div class="agent-step-header"><span class="agent-step-icon">' + icon + '</span><span class="agent-step-label">Paso ' + iteration + ': ' + label + '</span><span class="agent-step-badge running">pendiente</span></div>' +
    '<div class="agent-step-body open">' +
      '<div class="agent-file-op" id="' + opId + '">' +
        '<div class="agent-file-op-header"><span class="icon">' + icon + '</span><span>' + label + '</span></div>' +
        '<div class="agent-file-op-path">' + sanitizeHTML(filePath) + '</div>' +
        '<div class="agent-file-op-actions">' +
          '<button class="accept-btn" data-opid="' + opId + '" data-path="' + sanitizeHTML(filePath) + '" data-action="accept">✓ Aceptar</button>' +
          '<button class="reject-btn" data-opid="' + opId + '" data-path="' + sanitizeHTML(filePath) + '" data-action="reject">✗ Rechazar</button>' +
        '</div>' +
        '<div class="agent-file-op-status" id="' + opId + '-status"></div>' +
      '</div>' +
    '</div>';
  agentStepsContainer.appendChild(step);
  step.querySelector('.accept-btn').addEventListener('click', (e) => {
    vscode.postMessage({ type: 'acceptFileOp', opId });
  });
  step.querySelector('.reject-btn').addEventListener('click', (e) => {
    vscode.postMessage({ type: 'rejectFileOp', opId });
    updateFileOpStatus(opId, 'rejected', 'Cambios rechazados');
  });
  messagesEl.scrollTop = messagesEl.scrollHeight;
  return opId;
}

function updateFileOpStatus(opId, status, text) {
  const badge = document.querySelector('#' + opId)?.closest('.agent-step')?.querySelector('.agent-step-badge');
  const statusEl = document.getElementById(opId + '-status');
  const actionsEl = document.querySelector('#' + opId + ' .agent-file-op-actions');
  if (badge) {
    badge.textContent = status === 'accepted' ? 'aceptado' : 'rechazado';
    badge.className = 'agent-step-badge ' + (status === 'accepted' ? 'success' : 'error');
  }
  if (statusEl) { statusEl.textContent = text; statusEl.className = 'agent-file-op-status ' + status; }
  if (actionsEl) actionsEl.remove();
}

function resetHistoryUI() {
  messagesEl.innerHTML = '';
  chatHistory.length = 0;
  pending = null;
  showWelcome();
}

function hydrateHistory(messages) {
  resetHistoryUI();
  const list = Array.isArray(messages) ? messages : [];
  if (list.length === 0) { showWelcome(); return; }
  hideWelcome();
  list.forEach((m) => {
    addMessage(m.role === 'assistant' ? 'assistant' : 'user', String(m.content || ''), {
      id: String(m.id || ''),
      timestamp: Number(m.timestamp || Date.now()),
      skipSync: true,
    });
  });
}

function highlightMessageById(messageId) {
  const id = String(messageId || '').trim();
  if (!id) return;
  const el = messagesEl.querySelector('[data-msg-id="' + id + '"]');
  if (!el) return;
  messagesEl.querySelectorAll('.msg.highlighted').forEach((n) => n.classList.remove('highlighted'));
  el.classList.add('highlighted');
  el.scrollIntoView({ behavior: 'smooth', block: 'center' });
  setTimeout(() => el.classList.remove('highlighted'), 1800);
}

function sendNow(text) {
  if (!text.trim()) return;
  addMessage('user', text);
  sendBtn.disabled = true;
  sendBtn.style.display = 'none';
  stopBtn.style.display = 'flex';
  if (text.trim().toLowerCase().startsWith('/agent')) {
    const task = text.replace(/^\\/agent\\s*/i, '').trim();
    if (!task) { addMessage('assistant', 'Uso: /agent <descripción de la tarea>'); sendBtn.disabled = false; sendBtn.style.display = 'flex'; stopBtn.style.display = 'none'; return; }
    startAgentAssistant();
    vscode.postMessage({ type: 'agentChat', task, model: modelsEl.value || '', contextFiles: getContextFiles() });
  } else {
    startAssistant();
    vscode.postMessage({ type: 'chat', text, model: modelsEl.value || '' });
  }
}

function setRegressionDraft(text) { regressionPromptDraft = String(text || '').trim(); }

/* ── Compare ── */
function openCompareView() { compareViewEl.style.display = 'block'; }
function closeCompareView() { compareViewEl.style.display = 'none'; }
function setComparePending() { openCompareView(); compareMetaEl.textContent = 'comparando...'; compareLeftTitleEl.textContent = '-'; compareRightTitleEl.textContent = '-'; compareLeftStatsEl.textContent = ''; compareRightStatsEl.textContent = ''; compareLeftContentEl.textContent = ''; compareRightContentEl.textContent = ''; }
function renderCompareResult(payload) {
  if (!payload) return;
  openCompareView();
  compareMetaEl.textContent = payload.prompt ? ('Prompt: ' + String(payload.prompt.length) + ' chars') : '';
  const left = payload.left || {}, right = payload.right || {};
  compareLeftTitleEl.textContent = left.model || '-';
  compareLeftStatsEl.textContent = 'time: ' + (left.elapsedMs || 0) + 'ms | chars: ' + (left.length || 0);
  compareLeftContentEl.textContent = left.content || '';
  compareRightTitleEl.textContent = right.model || '-';
  compareRightStatsEl.textContent = 'time: ' + (right.elapsedMs || 0) + 'ms | chars: ' + (right.length || 0);
  compareRightContentEl.textContent = right.content || '';
}

/* ── Voice ── */
function setVoiceStatus(state, label) { if (voiceStatusEl) { voiceStatusEl.dataset.state = state; voiceStatusEl.textContent = label; } }
function appendTranscript(text) {
  const t = String(text || '').trim();
  if (!t) return;
  const sep = promptEl.value && !/\\s$/.test(promptEl.value) ? ' ' : '';
  promptEl.value += sep + t;
  promptEl.focus();
}
function initVoiceInput() {
  if (!voiceEnabled) { if (micBtn) micBtn.style.display = 'none'; if (voiceStatusEl) voiceStatusEl.style.display = 'none'; return; }
  const Ctor = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (typeof Ctor !== 'function') { if (micBtn) micBtn.disabled = true; setVoiceStatus('unavailable', ''); return; }
  recognition = new Ctor();
  recognition.continuous = true;
  recognition.interimResults = true;
  recognition.lang = navigator.language || 'es-ES';
  recognition.onstart = () => { listening = true; setVoiceStatus('listening', '🔴'); };
  recognition.onend = () => { listening = false; setVoiceStatus('stopped', ''); };
  recognition.onerror = () => { listening = false; setVoiceStatus('unavailable', ''); };
  recognition.onresult = (event) => {
    let finalText = '';
    for (let i = event.resultIndex; i < event.results.length; i++) {
      const r = event.results[i];
      if (r.isFinal && r[0]?.transcript) finalText += r[0].transcript + ' ';
    }
    appendTranscript(finalText);
  };
  if (micBtn) micBtn.addEventListener('click', () => { try { listening ? recognition.stop() : recognition.start(); } catch { setVoiceStatus('unavailable', ''); } });
}

/* ── Slash command popup ── */
function updateSlashPopup() {
  const val = promptEl.value;
  if (!val.startsWith('/') || val.includes(' ')) { slashPopupEl.classList.remove('show'); slashActive = -1; return; }
  const q = val.toLowerCase();
  const matches = SLASH_COMMANDS.filter(c => c.cmd.startsWith(q));
  if (matches.length === 0) { slashPopupEl.classList.remove('show'); slashActive = -1; return; }
  slashPopupEl.innerHTML = matches.map((c, i) =>
    '<div class="slash-item' + (i === slashActive ? ' active' : '') + '" data-cmd="' + c.cmd + '"><span class="slash-cmd">' + c.cmd + '</span><span class="slash-desc">' + c.desc + '</span></div>'
  ).join('');
  slashPopupEl.classList.add('show');
  slashPopupEl.querySelectorAll('.slash-item').forEach((el) => {
    el.addEventListener('click', () => {
      promptEl.value = el.dataset.cmd + ' ';
      slashPopupEl.classList.remove('show');
      slashActive = -1;
      promptEl.focus();
    });
  });
}

/* ── Send ── */
function send() {
  const text = promptEl.value.trim();
  if (!text) return;
  promptEl.value = '';
  slashPopupEl.classList.remove('show');
  sendNow(text);
}

sendBtn.addEventListener('click', send);
promptEl.addEventListener('keydown', (e) => {
  if (slashPopupEl.classList.contains('show')) {
    const items = slashPopupEl.querySelectorAll('.slash-item');
    if (e.key === 'ArrowDown') { e.preventDefault(); slashActive = Math.min(slashActive + 1, items.length - 1); updateSlashPopup(); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); slashActive = Math.max(slashActive - 1, 0); updateSlashPopup(); return; }
    if ((e.key === 'Tab' || e.key === 'Enter') && slashActive >= 0 && items[slashActive]) {
      e.preventDefault();
      promptEl.value = items[slashActive].dataset.cmd + ' ';
      slashPopupEl.classList.remove('show');
      slashActive = -1;
      return;
    }
  }
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
});
promptEl.addEventListener('input', updateSlashPopup);

exportBtn.addEventListener('click', () => { vscode.postMessage({ type: 'export', messages: chatHistory }); });
compareBtn.addEventListener('click', () => {
  const prompt = promptEl.value.trim();
  if (!prompt) return;
  setComparePending();
  vscode.postMessage({ type: 'compare', text: prompt });
});
stopBtn.addEventListener('click', () => {
  vscode.postMessage({ type: 'cancelChat' });
  stopBtn.style.display = 'none';
  sendBtn.style.display = 'flex';
  sendBtn.disabled = false;
});
modelsEl.addEventListener('change', () => { vscode.postMessage({ type: 'modelSelected', model: modelsEl.value || '' }); });
closeCompareBtn.addEventListener('click', closeCompareView);
codeOnlyToggleEl.addEventListener('click', () => {
  codeOnlyMode = !codeOnlyMode;
  codeOnlyToggleEl.classList.toggle('active', codeOnlyMode);
  rerenderChatKeepingScroll();
});
profileBadgeEl.addEventListener('click', () => { vscode.postMessage({ type: 'switchProfile' }); });
addFileBtnEl.addEventListener('click', () => { vscode.postMessage({ type: 'pickContextFile' }); });

function getContextFiles() {
  const files = [];
  contextChipsEl.querySelectorAll('.ctx-chip').forEach(chip => {
    const p = chip.dataset.path;
    if (p) files.push(p);
  });
  return files;
}

/* Welcome suggestions */
document.querySelectorAll('.suggest-btn').forEach((btn) => {
  btn.addEventListener('click', () => {
    const cmd = btn.dataset.prompt || '';
    promptEl.value = cmd + ' ';
    promptEl.focus();
  });
});

initVoiceInput();

window.addEventListener('message', (e) => {
  const msg = e.data;
  if (msg.type === 'chunk') { updatePending(msg.text || ''); return; }
  if (msg.type === 'done') { pending = null; sendBtn.disabled = false; sendBtn.style.display = 'flex'; stopBtn.style.display = 'none'; return; }
  if (msg.type === 'error') { updatePending('\\n[Error: ' + (msg.text || 'unknown') + ']'); pending = null; sendBtn.disabled = false; sendBtn.style.display = 'flex'; stopBtn.style.display = 'none'; return; }
  if (msg.type === 'agentThinking') { addAgentThinking(msg.iteration || 0, msg.content || ''); return; }
  if (msg.type === 'agentToolCall') { addAgentToolCall(msg.iteration || 0, msg.tool || '', msg.args); return; }
  if (msg.type === 'agentToolResult') { addAgentToolResult(msg.iteration || 0, msg.tool || '', !!msg.success, msg.output || ''); return; }
  if (msg.type === 'agentAnswer') { addAgentAnswer(msg.content || ''); return; }
  if (msg.type === 'agentDone') { finishAgent(); return; }
  if (msg.type === 'agentError') { addAgentAnswer('[Error: ' + (msg.text || 'unknown') + ']'); finishAgent(); return; }
  if (msg.type === 'agentFileCreate') { addAgentFileOp('create', msg.iteration || 0, msg.path || '', msg.opId || ''); return; }
  if (msg.type === 'agentFileModify') { addAgentFileOp('modify', msg.iteration || 0, msg.path || '', msg.opId || ''); return; }
  if (msg.type === 'agentFileResult') {
    const opId = msg.opId || '';
    const accepted = !!msg.accepted;
    updateFileOpStatus(opId, accepted ? 'accepted' : 'rejected', accepted ? 'Archivo guardado ✓' : 'Cambios descartados');
    return;
  }
  if (msg.type === 'prefill') { promptEl.value = msg.text || ''; promptEl.focus(); return; }
  if (msg.type === 'runPrompt') { sendNow(msg.text || ''); return; }
  if (msg.type === 'externalResult') { addMessage('assistant', msg.text || ''); return; }
  if (msg.type === 'hydrateHistory') { hydrateHistory(msg.messages || []); return; }
  if (msg.type === 'highlightHistory') { highlightMessageById(msg.id || ''); return; }
  if (msg.type === 'historyCleared') { resetHistoryUI(); return; }
  if (msg.type === 'compareStart') { setComparePending(); return; }
  if (msg.type === 'compareResult') { renderCompareResult(msg); return; }
  if (msg.type === 'openCompareMode') { openCompareView(); return; }
  if (msg.type === 'models') {
    const models = Array.isArray(msg.models) ? msg.models : [];
    const current = msg.current || '';
    modelsEl.innerHTML = '';
    const arr = models.length > 0 ? models : [current || 'local-rag'];
    arr.forEach((m) => { const opt = document.createElement('option'); opt.value = m; opt.textContent = m; modelsEl.appendChild(opt); });
    modelsEl.value = arr.includes(current) ? current : arr[0];
    return;
  }
  if (msg.type === 'profileUpdate') { updateProfileBadge(msg.profile || {}); return; }
  if (msg.type === 'modelsValidation') { updateModelsHealth(msg); return; }
  if (msg.type === 'sessionRestored') { showSessionBanner(msg.text || 'session restored'); return; }
  if (msg.type === 'regressionDraft') { setRegressionDraft(msg.text || ''); return; }
  if (msg.type === 'contextFile') {
    const file = String(msg.file || '').trim();
    if (!file) return;
    const chip = document.createElement('span');
    chip.className = 'ctx-chip';
    chip.dataset.path = file;
    chip.innerHTML = '📄 ' + sanitizeHTML(file.split('/').pop() || file) + '<span class="remove" title="Remove">×</span>';
    chip.querySelector('.remove').addEventListener('click', () => chip.remove());
    contextChipsEl.appendChild(chip);
    return;
  }
});
</script>
</body>
</html>`;
}

class CopilotLocalChatViewProvider {
  constructor(extensionUri, getState) {
    this._extensionUri = extensionUri;
    this._getState = getState;
    this._view = null;
  }

  resolveWebviewView(webviewView) {
    this._view = webviewView;
    webviewView.webview.options = { enableScripts: true, localResourceRoots: [this._extensionUri] };
    const state = this._getState();
    webviewView.webview.html = getChatSidebarHTML(state.fontSize, state.voiceInputEnabled);
    if (state.onReady) state.onReady(webviewView);
  }

  get view() { return this._view; }
}

function activate(context) {
  loadPromptRC();
  const output = vscode.window.createOutputChannel('Copilot Local');
  const metrics = new LocalMetrics(context);
  let chatPanel = null;
  let sidebarReady = false;
  let activeSessionId = '';
  let selectedModel = getConfig().model;
  let activeChatAbort = null;
  let consecutiveErrors = 0;
  const activeQualityAlerts = new Set();
  let shouldShowRestoredBanner = false;

  // ── Sidebar Chat View Provider ──
  const chatViewProvider = new CopilotLocalChatViewProvider(context.extensionUri, () => {
    const cfg = getConfig();
    return {
      fontSize: cfg.chatFontSize,
      voiceInputEnabled: cfg.voiceInputEnabled,
      onReady: (webviewView) => {
        sidebarReady = true;
        setupWebviewMessageHandler(webviewView.webview, 'sidebar');
        publishProfileToWebview(webviewView.webview);
        const restoredState = loadSessionState();
        const messages = restoredState.messages.length > 0 ? restoredState.messages : loadChatHistory();
        webviewView.webview.postMessage({ type: 'hydrateHistory', messages });
        if (shouldShowRestoredBanner) {
          webviewView.webview.postMessage({ type: 'sessionRestored', text: 'session restored' });
          shouldShowRestoredBanner = false;
        }
      },
    };
  });
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider('copilotLocal.chatView', chatViewProvider, { webviewOptions: { retainContextWhenHidden: true } })
  );

  // ── Status Bar (consolidated, Copilot-style) ──
  const mainStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 90);
  mainStatus.text = '$(sparkle) Copilot Local';
  mainStatus.tooltip = 'Copilot Local – Click to open chat';
  mainStatus.command = 'copilotLocal.chatView.focus';
  mainStatus.show();
  context.subscriptions.push(mainStatus);

  const profileStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 88);
  profileStatus.command = 'copilot-local.switchProfile';
  context.subscriptions.push(profileStatus);

  const refreshProfileStatus = () => {
    const cfg = getConfig();
    profileStatus.text = 'Profile: ' + cfg.activeProfileId;
    profileStatus.tooltip = [
      'Active profile: ' + cfg.activeProfileId,
      'Model: ' + cfg.model,
      'Lang: ' + cfg.lang,
      'Temperature: ' + Number(cfg.temperature || 0).toFixed(2),
      'Click to switch profile',
    ].join('\n');
    profileStatus.show();
  };
  refreshProfileStatus();

  const loadChatHistory = () => normalizeHistoryMessages(context.globalState.get(getChatHistoryStorageKey(), []));
  const saveChatHistory = async (messages) => {
    const normalized = normalizeHistoryMessages(messages);
    await context.globalState.update(getChatHistoryStorageKey(), normalized);
    return normalized;
  };
  const loadSessionState = () => normalizeSessionState(context.workspaceState.get(CHAT_SESSION_STATE_KEY, null));
  const saveSessionState = async (messages) => {
    const state = buildSessionState(messages, selectedModel, activeSessionId);
    await context.workspaceState.update(CHAT_SESSION_STATE_KEY, state);
    return state;
  };
  const loadFavorites = () => normalizeFavorites(context.globalState.get(getFavoritesStorageKey(), []));
  const saveFavorites = async (items) => {
    const normalized = normalizeFavorites(items);
    await context.globalState.update(getFavoritesStorageKey(), normalized);
    return normalized;
  };

  const inlineStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 89);
  inlineStatus.text = '$(sparkle)';
  inlineStatus.tooltip = 'Copilot Local – Inline completions active';
  inlineStatus.show();
  context.subscriptions.push(inlineStatus);

  const qualityStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 89);
  qualityStatus.text = 'Quality OK';
  qualityStatus.tooltip = 'Local quality alerts';
  qualityStatus.hide();
  context.subscriptions.push(qualityStatus);

  const perfStatus = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 87);
  perfStatus.text = 'Perf: --';
  perfStatus.tooltip = 'Latency and tokens/sec';
  perfStatus.show();
  context.subscriptions.push(perfStatus);

  const refreshPerfStatus = () => {
    const snap = metrics.getSnapshot();
    const avgLatency = Math.round(Number(snap.avg_response_time_ms || 0));
    const tps = metrics.tokensPerSecondApprox();
    perfStatus.text = 'Perf: ' + avgLatency + 'ms | ' + tps.toFixed(1) + ' tok/s';
    perfStatus.tooltip = [
      'Avg latency: ' + avgLatency + ' ms',
      'Approx tokens/sec: ' + tps.toFixed(2),
      'Total tokens approx: ' + Number(snap.total_tokens_approx || 0),
      'Total requests: ' + Number(snap.total_requests || 0),
    ].join('\n');
  };
  refreshPerfStatus();
  const perfPoll = setInterval(refreshPerfStatus, 5000);
  context.subscriptions.push({ dispose: () => clearInterval(perfPoll) });

  const restored = loadSessionState();
  if (restored.messages.length > 0 || restored.activeSessionId || restored.selectedModel) {
    activeSessionId = restored.activeSessionId;
    selectedModel = restored.selectedModel || selectedModel;
    shouldShowRestoredBanner = true;
    saveChatHistory(restored.messages).catch(() => {});
  }

  // ── Helpers to send to whichever webview is active (sidebar or panel) ──
  const getActiveWebview = () => {
    if (chatViewProvider.view) return chatViewProvider.view.webview;
    if (chatPanel) return chatPanel.webview;
    return null;
  };

  const focusChatAndGetWebview = async () => {
    try { await vscode.commands.executeCommand('copilotLocal.chatView.focus'); } catch {}
    let wv = getActiveWebview();
    if (!wv) {
      const panel = await openChatPanel();
      wv = panel.webview;
    }
    return wv;
  };

  const postToChat = (msg) => {
    const wv = getActiveWebview();
    if (wv) wv.postMessage(msg);
  };

  const publishProfileToWebview = async (webview) => {
    const cfg = getConfig();
    const models = await getAvailableModels();
    const currentModel = selectedModel || cfg.model;
    webview.postMessage({ type: 'models', models, current: currentModel });
    webview.postMessage({
      type: 'profileUpdate',
      profile: {
        id: cfg.activeProfileId,
        model: currentModel,
        lang: cfg.lang,
        temperature: cfg.temperature,
      },
    });
    const mStatus = await validateLocalModels();
    webview.postMessage({
      type: 'modelsValidation',
      ok: mStatus.ok,
      count: mStatus.count,
      source: mStatus.source,
      error: mStatus.error || '',
    });
  };

  const previewAndApplyCode = async (editor, code, languageHint) => {
    const hasSelection = !editor.selection.isEmpty;
    const targetRange = hasSelection
      ? editor.selection
      : new vscode.Range(editor.document.positionAt(0), editor.document.positionAt(editor.document.getText().length));
    const originalText = editor.document.getText(targetRange);
    const guessedLanguage = targetEditorLanguage(languageHint) || editor.document.languageId || 'plaintext';
    const originalDoc = await vscode.workspace.openTextDocument({ content: originalText, language: guessedLanguage });
    const proposedDoc = await vscode.workspace.openTextDocument({ content: code, language: guessedLanguage });
    const title = hasSelection ? 'Copilot Local Refactor Preview (selection)' : 'Copilot Local Refactor Preview (full file)';
    await vscode.commands.executeCommand('vscode.diff', originalDoc.uri, proposedDoc.uri, title, { preview: true, viewColumn: vscode.ViewColumn.Beside });
    const decision = await vscode.window.showInformationMessage('Diff preview abierto. ¿Aplicar cambios propuestos?', 'Apply', 'Cancel');
    if (decision !== 'Apply') return false;
    const edit = new vscode.WorkspaceEdit();
    edit.replace(editor.document.uri, targetRange, code);
    const ok = await vscode.workspace.applyEdit(edit);
    if (ok) { try { await vscode.commands.executeCommand('editor.action.formatDocument'); } catch {} }
    return ok;
  };

  // ── Workspace context gathering for agent autopilot ──
  const gatherWorkspaceContext = async (extraFiles) => {
    const ctx = { tree: [], files: [] };
    const wsFolder = vscode.workspace.workspaceFolders?.[0];
    if (!wsFolder) return ctx;

    // 1. Get workspace file tree (exclude heavy dirs)
    try {
      const uris = await vscode.workspace.findFiles('**/*', '{**/node_modules/**,**/.git/**,**/vendor/**,**/dist/**,**/build/**,**/*.exe,**/*.bin,**/*.wasm}', 500);
      ctx.tree = uris.map((u) => vscode.workspace.asRelativePath(u)).sort();
    } catch {}

    // 2. Get content of open/visible editors
    const seen = new Set();
    for (const editor of vscode.window.visibleTextEditors) {
      const rel = vscode.workspace.asRelativePath(editor.document.uri);
      if (seen.has(rel)) continue;
      seen.add(rel);
      const text = editor.document.getText();
      if (text.length < 50000) {
        ctx.files.push({ path: rel, content: text });
      }
    }

    // 3. Add explicitly selected context files
    if (Array.isArray(extraFiles)) {
      for (const filePath of extraFiles) {
        if (seen.has(filePath)) continue;
        seen.add(filePath);
        try {
          const uri = vscode.Uri.joinPath(wsFolder.uri, filePath);
          const raw = await vscode.workspace.fs.readFile(uri);
          const text = Buffer.from(raw).toString('utf8');
          if (text.length < 50000) {
            ctx.files.push({ path: filePath, content: text });
          }
        } catch {}
      }
    }

    return ctx;
  };

  // ── File operation handlers for agent autopilot ──
  const handleAgentFileCreate = async (filePath, content, webview) => {
    const wsFolder = vscode.workspace.workspaceFolders?.[0];
    if (!wsFolder) { vscode.window.showErrorMessage('No hay workspace abierto'); return; }

    const uri = vscode.Uri.joinPath(wsFolder.uri, filePath);
    // Show proposed content in a temporary document
    const doc = await vscode.workspace.openTextDocument({ content, language: guessLanguageFromPath(filePath) });
    await vscode.window.showTextDocument(doc, { preview: true, viewColumn: vscode.ViewColumn.Beside });

    const decision = await vscode.window.showInformationMessage(
      '🤖 Agent propone crear: ' + filePath, 'Crear archivo', 'Descartar'
    );
    if (decision === 'Crear archivo') {
      await vscode.workspace.fs.writeFile(uri, Buffer.from(content, 'utf8'));
      vscode.window.showInformationMessage('Archivo creado: ' + filePath);
      webview.postMessage({ type: 'agentFileResult', path: filePath, action: 'create', accepted: true });
    } else {
      webview.postMessage({ type: 'agentFileResult', path: filePath, action: 'create', accepted: false });
    }
  };

  const handleAgentFileModify = async (filePath, proposedContent, webview) => {
    const wsFolder = vscode.workspace.workspaceFolders?.[0];
    if (!wsFolder) { vscode.window.showErrorMessage('No hay workspace abierto'); return; }

    const uri = vscode.Uri.joinPath(wsFolder.uri, filePath);
    let originalContent = '';
    try {
      const raw = await vscode.workspace.fs.readFile(uri);
      originalContent = Buffer.from(raw).toString('utf8');
    } catch {
      // File doesn't exist yet, treat as create
      return handleAgentFileCreate(filePath, proposedContent, webview);
    }

    const lang = guessLanguageFromPath(filePath);
    const originalDoc = await vscode.workspace.openTextDocument({ content: originalContent, language: lang });
    const proposedDoc = await vscode.workspace.openTextDocument({ content: proposedContent, language: lang });
    await vscode.commands.executeCommand('vscode.diff', originalDoc.uri, proposedDoc.uri, '🤖 Agent: ' + filePath + ' (cambios propuestos)', { preview: true, viewColumn: vscode.ViewColumn.Beside });

    const decision = await vscode.window.showInformationMessage(
      '🤖 Agent propone modificar: ' + filePath, 'Aplicar cambios', 'Descartar'
    );
    if (decision === 'Aplicar cambios') {
      await vscode.workspace.fs.writeFile(uri, Buffer.from(proposedContent, 'utf8'));
      vscode.window.showInformationMessage('Archivo modificado: ' + filePath);
      webview.postMessage({ type: 'agentFileResult', path: filePath, action: 'modify', accepted: true });
    } else {
      webview.postMessage({ type: 'agentFileResult', path: filePath, action: 'modify', accepted: false });
    }
  };

  const guessLanguageFromPath = (p) => {
    const ext = String(p || '').split('.').pop()?.toLowerCase() || '';
    const map = { js: 'javascript', ts: 'typescript', go: 'go', py: 'python', rs: 'rust', java: 'java', rb: 'ruby', md: 'markdown', json: 'json', yaml: 'yaml', yml: 'yaml', html: 'html', css: 'css', sh: 'shellscript', bash: 'shellscript', sql: 'sql', xml: 'xml', toml: 'toml' };
    return map[ext] || 'plaintext';
  };

  const pendingFileOps = new Map();

  const setupWebviewMessageHandler = (webview, source) => {
    webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'cancelChat') {
        if (activeChatAbort) { try { activeChatAbort.abort(); } catch {} activeChatAbort = null; }
        webview.postMessage({ type: 'done' });
        return;
      }
      if (msg.type === 'switchProfile') {
        vscode.commands.executeCommand('copilot-local.switchProfile');
        return;
      }
      if (msg.type === 'chat') {
        const model = String(msg.model || getConfig().chatModel);
        const resolvedText = resolveSlashChatPrompt(msg.text);
        selectedModel = model;
        if (activeSessionId) {
          const endpoint = '/api/sessions/' + encodeURIComponent(activeSessionId) + '/chat';
          try {
            const res = await postJSON(endpoint, { message: resolvedText });
            if (res?.response) webview.postMessage({ type: 'chunk', text: String(res.response) });
            webview.postMessage({ type: 'done' });
            await saveSessionState(loadChatHistory());
          } catch (err) {
            webview.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
          }
          return;
        }
        streamWithFallback(resolvedText, model,
          (chunk) => webview.postMessage({ type: 'chunk', text: chunk }),
          (err) => {
            evaluateQualityAlerts('chat', !!err);
            if (err) webview.postMessage({ type: 'error', text: err.message });
            else webview.postMessage({ type: 'done' });
          },
        );
        return;
      }
      if (msg.type === 'agentChat') {
        const task = String(msg.task || '').trim();
        const model = String(msg.model || getConfig().chatModel);
        const extraFiles = Array.isArray(msg.contextFiles) ? msg.contextFiles : [];
        if (!task) { webview.postMessage({ type: 'agentError', text: 'Tarea vacía' }); return; }
        const wsContext = await gatherWorkspaceContext(extraFiles);
        let fileOpSeq = 0;
        streamAgentAutopilot(task, model, wsContext,
          (ev) => {
            if (ev.event === 'thinking') webview.postMessage({ type: 'agentThinking', iteration: ev.iteration, content: ev.content });
            else if (ev.event === 'tool_call') webview.postMessage({ type: 'agentToolCall', iteration: ev.iteration, tool: ev.tool, args: ev.args });
            else if (ev.event === 'tool_result') webview.postMessage({ type: 'agentToolResult', iteration: ev.iteration, tool: ev.tool, success: ev.success, output: ev.output });
            else if (ev.event === 'answer') webview.postMessage({ type: 'agentAnswer', content: ev.content });
            else if (ev.event === 'error') webview.postMessage({ type: 'agentError', text: ev.content });
            else if (ev.event === 'file_create') {
              const opId = 'fileop-' + (++fileOpSeq);
              pendingFileOps.set(opId, { type: 'create', path: ev.file_path, content: ev.file_content });
              webview.postMessage({ type: 'agentFileCreate', iteration: ev.iteration, path: ev.file_path, opId });
            }
            else if (ev.event === 'file_modify') {
              const opId = 'fileop-' + (++fileOpSeq);
              pendingFileOps.set(opId, { type: 'modify', path: ev.file_path, content: ev.file_content });
              webview.postMessage({ type: 'agentFileModify', iteration: ev.iteration, path: ev.file_path, opId });
            }
          },
          (err) => {
            if (err) webview.postMessage({ type: 'agentError', text: err.message || String(err) });
            else webview.postMessage({ type: 'agentDone' });
          },
        );
        return;
      }
      if (msg.type === 'pickContextFile') {
        const wsFolder = vscode.workspace.workspaceFolders?.[0];
        if (!wsFolder) return;
        const uris = await vscode.window.showOpenDialog({
          canSelectMany: true,
          defaultUri: wsFolder.uri,
          openLabel: 'Agregar al contexto',
        });
        if (!uris || uris.length === 0) return;
        for (const uri of uris) {
          const rel = vscode.workspace.asRelativePath(uri);
          webview.postMessage({ type: 'contextFile', file: rel });
        }
        return;
      }
      if (msg.type === 'acceptFileOp') {
        const opId = msg.opId;
        const op = pendingFileOps.get(opId);
        if (!op) return;
        pendingFileOps.delete(opId);
        if (op.type === 'create') {
          await handleAgentFileCreate(op.path, op.content, webview);
        } else {
          await handleAgentFileModify(op.path, op.content, webview);
        }
        webview.postMessage({ type: 'agentFileResult', opId, accepted: true });
        return;
      }
      if (msg.type === 'rejectFileOp') {
        const opId = msg.opId;
        pendingFileOps.delete(opId);
        webview.postMessage({ type: 'agentFileResult', opId, accepted: false });
        return;
      }
      if (msg.type === 'compare') {
        const prompt = String(msg.text || '').trim();
        if (!prompt) { webview.postMessage({ type: 'error', text: 'Prompt vacío' }); return; }
        const selected = await chooseTwoModels();
        if (!selected) return;
        webview.postMessage({ type: 'compareStart' });
        try {
          const [left, right] = await Promise.all([
            requestChatCompletion(prompt, selected[0], metrics),
            requestChatCompletion(prompt, selected[1], metrics),
          ]);
          await evaluateQualityAlerts('compare', false);
          webview.postMessage({ type: 'compareResult', prompt, left, right });
        } catch (err) {
          await evaluateQualityAlerts('compare', true);
          webview.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
        }
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
        let content = '', language = 'plaintext';
        if (format.value === 'md') { language = 'markdown'; content = messages.map((m) => '## ' + (m.role === 'user' ? 'User' : 'Assistant') + '\\n\\n' + (m.content || '')).join('\\n\\n'); }
        else if (format.value === 'json') { language = 'json'; content = JSON.stringify(messages, null, 2); }
        else { content = messages.map((m) => (m.role === 'user' ? 'You: ' : 'AI: ') + (m.content || '')).join('\\n\\n'); }
        const doc = await vscode.workspace.openTextDocument({ content, language });
        await vscode.window.showTextDocument(doc, { preview: false });
        return;
      }
      if (msg.type === 'historyUpdate') {
        const normalized = await saveChatHistory(msg.messages);
        await saveSessionState(normalized);
        return;
      }
      if (msg.type === 'modelSelected') {
        selectedModel = String(msg.model || '').trim() || getConfig().model;
        await saveSessionState(loadChatHistory());
        return;
      }
      if (msg.type === 'clearHistoryRequest') {
        const confirmed = await vscode.window.showWarningMessage('¿Borrar historial de chat?', 'Clear', 'Cancel');
        if (confirmed !== 'Clear') return;
        await saveChatHistory([]);
        await saveSessionState([]);
        webview.postMessage({ type: 'historyCleared' });
        vscode.window.showInformationMessage('Historial de chat limpiado');
        return;
      }
      if (msg.type === 'favoriteAdd') {
        const content = String(msg.content || '').trim();
        if (!content) return;
        const favorites = loadFavorites();
        if (favorites.some((f) => f.content.trim() === content)) { vscode.window.showInformationMessage('Ya está en favoritos'); return; }
        favorites.push({ id: Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8), title: buildFavoriteTitle(content), content, timestamp: Date.now() });
        await saveFavorites(favorites);
        vscode.window.showInformationMessage('Favorito guardado');
        return;
      }
      if (msg.type === 'apply') {
        const code = String(msg.code || '');
        if (!code.trim()) return;
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
          const ok = await vscode.window.showInformationMessage('Apply code to editor?', 'Yes', 'No');
          if (ok !== 'Yes') return;
          const doc = await vscode.workspace.openTextDocument({ content: code, language: targetEditorLanguage(msg.lang) });
          await vscode.window.showTextDocument(doc, { preview: false });
          return;
        }
        const isDiff = code.startsWith('---') || code.startsWith('@@');
        if (isDiff) {
          const ok = await vscode.window.showInformationMessage('Apply code to editor?', 'Yes', 'No');
          if (ok !== 'Yes') return;
          const edit = new vscode.WorkspaceEdit();
          const fullRange = new vscode.Range(editor.document.positionAt(0), editor.document.positionAt(editor.document.getText().length));
          edit.replace(editor.document.uri, fullRange, code);
          await vscode.workspace.applyEdit(edit);
          try { await vscode.commands.executeCommand('editor.action.formatDocument'); } catch {}
        } else {
          await previewAndApplyCode(editor, code, msg.lang);
        }
        return;
      }
    });
  };

  const evaluateQualityAlerts = async (source, hadError) => {
    const cfg = getConfig();
    if (!cfg.qualityAlertsEnabled) {
      activeQualityAlerts.clear();
      qualityStatus.hide();
      return;
    }

    if (hadError) {
      consecutiveErrors += 1;
    } else {
      consecutiveErrors = 0;
    }

    const snap = metrics.getSnapshot();
    const nextAlerts = new Set();
    if ((snap.avg_response_time_ms || 0) >= Math.max(1, Number(cfg.qualityLatencyThresholdMs) || 8000)) {
      nextAlerts.add('latency');
    }
    if (consecutiveErrors >= Math.max(1, Number(cfg.qualityConsecutiveErrorsThreshold) || 3)) {
      nextAlerts.add('errors');
    }

    for (const a of nextAlerts) {
      if (!activeQualityAlerts.has(a)) {
        if (a === 'latency') {
          output.appendLine('[quality][warn] high latency detected in ' + source + ': avg=' + Math.round(snap.avg_response_time_ms || 0) + 'ms');
        } else if (a === 'errors') {
          output.appendLine('[quality][warn] consecutive errors detected in ' + source + ': count=' + consecutiveErrors);
        }
      }
    }
    for (const a of activeQualityAlerts) {
      if (!nextAlerts.has(a)) {
        output.appendLine('[quality][clear] alert resolved: ' + a);
      }
    }

    activeQualityAlerts.clear();
    for (const a of nextAlerts) activeQualityAlerts.add(a);

    if (activeQualityAlerts.size === 0) {
      qualityStatus.hide();
      return;
    }

    qualityStatus.text = 'Quality ⚠ ' + Array.from(activeQualityAlerts).join('+');
    qualityStatus.tooltip = [
      'Avg latency: ' + Math.round(snap.avg_response_time_ms || 0) + ' ms (threshold: ' + Math.max(1, Number(cfg.qualityLatencyThresholdMs) || 8000) + ' ms)',
      'Consecutive errors: ' + consecutiveErrors + ' (threshold: ' + Math.max(1, Number(cfg.qualityConsecutiveErrorsThreshold) || 3) + ')',
    ].join('\n');
    qualityStatus.show();
  };

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
  const inlineAbort = new Map();
  const inlineProvider = {
    provideInlineCompletionItems: async (document, position) => {
      const cfg = getConfig();
      if (!cfg.inlineCompletions) return [];
      if (document.lineCount < 3) return [];

      const key = document.uri.toString();
      if (completionDebounce.has(key)) clearTimeout(completionDebounce.get(key));
      await new Promise((resolve) => {
        const t = setTimeout(resolve, 300);
        completionDebounce.set(key, t);
      });

      if (inlineAbort.has(key)) {
        try { inlineAbort.get(key).abort(); } catch {}
      }
      const abortCtl = new AbortController();
      inlineAbort.set(key, abortCtl);

      const startLine = Math.max(0, position.line - 49);
      const endLine = Math.min(document.lineCount - 1, position.line + 49);
      const prefixRange = new vscode.Range(new vscode.Position(startLine, 0), position);
      const suffixRange = new vscode.Range(position, new vscode.Position(endLine, document.lineAt(endLine).text.length));
      const prefix = applyPromptRC(document.getText(prefixRange));
      const suffix = document.getText(suffixRange);
      inlineStatus.text = '$(loading~spin) Completing...';
      mainStatus.text = '$(loading~spin) Copilot Local';
      try {
        const res = await postJSONAbort('/complete', {
          model: cfg.fimModel,
          prefix,
          suffix,
          language: normalizeLanguageId(document.languageId),
          num_predict: 100,
          temperature: cfg.temperature,
        }, abortCtl.signal);
        const text = String(res?.completion || '').trim();
        if (!text) return [];
        return [new vscode.InlineCompletionItem(text, new vscode.Range(position, position))];
      } catch {
        return [];
      } finally {
        if (inlineAbort.get(key) === abortCtl) inlineAbort.delete(key);
        inlineStatus.text = '$(sparkle)';
        mainStatus.text = '$(sparkle) Copilot Local';
      }
    },
  };
  if (vscode.languages && typeof vscode.languages.registerInlineCompletionItemProvider === 'function') {
    context.subscriptions.push(vscode.languages.registerInlineCompletionItemProvider({ pattern: '**' }, inlineProvider));
  } else {
    output.appendLine('[warn] Inline completions API is not available in this VS Code version.');
  }

  const streamWithFallback = (prompt, model, onChunk, onDone) => {
    const abortCtl = new AbortController();
    activeChatAbort = abortCtl;

    const done = (err) => {
      if (activeChatAbort === abortCtl) activeChatAbort = null;
      onDone(err);
    };

    streamWS(prompt, model, onChunk, (wsErr) => {
      if (abortCtl.signal.aborted) {
        done();
        return;
      }
      if (!wsErr) {
        done();
        return;
      }
      streamHTTP(prompt, model, onChunk, (httpErr) => {
        if (abortCtl.signal.aborted) {
          done();
          return;
        }
        if (!httpErr) {
          done();
          return;
        }
        streamCLI(prompt, model, onChunk, done);
      }, abortCtl, metrics);
    }, abortCtl, metrics);
  };

  const validateLocalModels = async () => {
    const cfg = getConfig();
    try {
      const res = await getJSON('/api/models');
      const models = Array.isArray(res?.models) ? res.models.filter((m) => String(m || '').trim()) : [];
      if (models.length > 0) {
        return { ok: true, count: models.length, source: 'api', models };
      }
      return { ok: false, count: 0, source: 'api', models: [], error: 'La API no devolvio modelos' };
    } catch (err) {
      return {
        ok: false,
        count: 0,
        source: 'fallback',
        models: [cfg.model || 'local-rag'],
        error: err instanceof Error ? err.message : String(err),
      };
    }
  };

  const publishModelsValidationToPanel = async () => {
    const status = await validateLocalModels();
    const msg = { type: 'modelsValidation', ok: status.ok, count: status.count, source: status.source, error: status.error || '' };
    if (chatPanel) chatPanel.webview.postMessage(msg);
    if (chatViewProvider.view) chatViewProvider.view.webview.postMessage(msg);
  };

  const getAvailableModels = async () => {
    const cfg = getConfig();
    try {
      const res = await getJSON('/api/models');
      const models = Array.isArray(res?.models) ? res.models.filter((m) => String(m || '').trim()) : [];
      if (models.length > 0) return models;
    } catch {}
    return [cfg.model || 'local-rag'];
  };

  const publishProfileToPanel = async () => {
    const cfg = getConfig();
    const models = await getAvailableModels();
    const currentModel = selectedModel || cfg.model;
    const profileMsg = { type: 'profileUpdate', profile: { id: cfg.activeProfileId, model: currentModel, lang: cfg.lang, temperature: cfg.temperature } };
    const modelsMsg = { type: 'models', models, current: currentModel };
    if (chatPanel) { chatPanel.webview.postMessage(modelsMsg); chatPanel.webview.postMessage(profileMsg); }
    if (chatViewProvider.view) { chatViewProvider.view.webview.postMessage(modelsMsg); chatViewProvider.view.webview.postMessage(profileMsg); }
    await publishModelsValidationToPanel();
  };

  const chooseTwoModels = async () => {
    const models = await getAvailableModels();
    if (models.length < 2) {
      vscode.window.showErrorMessage('Se requieren al menos 2 modelos para comparar');
      return null;
    }

    const first = await vscode.window.showQuickPick(models.map((m) => ({ label: m, value: m })), {
      title: 'Compare Models',
      placeHolder: 'Selecciona el primer modelo',
    });
    if (!first) return null;

    const secondCandidates = models.filter((m) => m !== first.value);
    const second = await vscode.window.showQuickPick(secondCandidates.map((m) => ({ label: m, value: m })), {
      title: 'Compare Models',
      placeHolder: 'Selecciona el segundo modelo',
    });
    if (!second) return null;

    return [first.value, second.value];
  };

  const resolveSlashChatPrompt = (rawText) => {
    const text = String(rawText || '').trim();
    if (!text.startsWith('/')) return text;

    const editor = vscode.window.activeTextEditor;
    const selected = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection)?.trim() || '';
    const arg = text.replace(/^\/\w+\s*/, '').trim();
    const body = arg || selected;

    if (text.toLowerCase().startsWith('/explain')) {
      return 'Explain this code:\n' + body;
    }
    if (text.toLowerCase().startsWith('/refactor')) {
      return [
        'Refactor this code for clarity and maintainability.',
        'Return two sections:',
        '1) Refactored code',
        '2) What changed and why (diff-style explanation)',
        '',
        body,
      ].join('\n');
    }
    if (text.toLowerCase().startsWith('/test')) {
      return 'Create robust tests for this code:\n' + body;
    }
    if (text.toLowerCase().startsWith('/fix')) {
      return 'Fix the errors in this code:\n' + body;
    }
    if (text.toLowerCase().startsWith('/docstring')) {
      return 'Add high quality docstrings/comments to this code:\n' + body;
    }
    if (text.toLowerCase().startsWith('/doc')) {
      return 'Add high quality docstrings/comments to this code:\n' + body;
    }
    return text;
  };

  async function openChatPanel() {
    if (chatPanel) {
      chatPanel.reveal();
      return chatPanel;
    }

    try {
      chatPanel = vscode.window.createWebviewPanel('copilotLocalChat', 'Copilot Local Chat', vscode.ViewColumn.Beside, {
        enableScripts: true,
        retainContextWhenHidden: true,
      });

      const cfg = getConfig();
      chatPanel.webview.html = getChatPanelHTML(cfg.chatFontSize, cfg.voiceInputEnabled);
    } catch (err) {
      try { chatPanel?.dispose(); } catch {}
      chatPanel = null;
      throw err;
    }

    const publishHistory = () => {
      const restoredState = loadSessionState();
      const messages = restoredState.messages.length > 0 ? restoredState.messages : loadChatHistory();
      chatPanel?.webview.postMessage({ type: 'hydrateHistory', messages });
      if (shouldShowRestoredBanner) {
        chatPanel?.webview.postMessage({ type: 'sessionRestored', text: 'session restored' });
        shouldShowRestoredBanner = false;
      }
    };

    await publishProfileToPanel();
    publishHistory();

    const previewAndApplyCode = async (editor, code, languageHint) => {
      const hasSelection = !editor.selection.isEmpty;
      const targetRange = hasSelection
        ? editor.selection
        : new vscode.Range(
          editor.document.positionAt(0),
          editor.document.positionAt(editor.document.getText().length),
        );
      const originalText = editor.document.getText(targetRange);
      const guessedLanguage = targetEditorLanguage(languageHint) || editor.document.languageId || 'plaintext';

      const originalDoc = await vscode.workspace.openTextDocument({ content: originalText, language: guessedLanguage });
      const proposedDoc = await vscode.workspace.openTextDocument({ content: code, language: guessedLanguage });
      const title = hasSelection
        ? 'Copilot Local Refactor Preview (selection)'
        : 'Copilot Local Refactor Preview (full file)';

      await vscode.commands.executeCommand('vscode.diff', originalDoc.uri, proposedDoc.uri, title, {
        preview: true,
        viewColumn: vscode.ViewColumn.Beside,
      });

      const decision = await vscode.window.showInformationMessage(
        'Diff preview abierto. ¿Aplicar cambios propuestos?',
        'Apply',
        'Cancel',
      );
      if (decision !== 'Apply') return false;

      const edit = new vscode.WorkspaceEdit();
      edit.replace(editor.document.uri, targetRange, code);
      const ok = await vscode.workspace.applyEdit(edit);
      if (ok) {
        try { await vscode.commands.executeCommand('editor.action.formatDocument'); } catch {}
      }
      return ok;
    };

    chatPanel.webview.onDidReceiveMessage(async (msg) => {
      if (msg.type === 'cancelChat') {
        if (activeChatAbort) {
          try { activeChatAbort.abort(); } catch {}
          activeChatAbort = null;
        }
        chatPanel?.webview.postMessage({ type: 'done' });
        return;
      }

      if (msg.type === 'chat') {
        const model = String(msg.model || getConfig().chatModel);
        const resolvedText = resolveSlashChatPrompt(msg.text);
        selectedModel = model;
        if (activeSessionId) {
          const endpoint = '/api/sessions/' + encodeURIComponent(activeSessionId) + '/chat';
          try {
            const res = await postJSON(endpoint, { message: resolvedText });
            if (res?.response) chatPanel?.webview.postMessage({ type: 'chunk', text: String(res.response) });
            chatPanel?.webview.postMessage({ type: 'done' });
            await saveSessionState(loadChatHistory());
          } catch (err) {
            chatPanel?.webview.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
          }
          return;
        }

        streamWithFallback(resolvedText, model,
          (chunk) => chatPanel?.webview.postMessage({ type: 'chunk', text: chunk }),
          (err) => {
            evaluateQualityAlerts('chat', !!err);
            if (err) chatPanel?.webview.postMessage({ type: 'error', text: err.message });
            else chatPanel?.webview.postMessage({ type: 'done' });
          },
        );
        return;
      }

      if (msg.type === 'compare') {
        const prompt = String(msg.text || '').trim();
        if (!prompt) {
          chatPanel?.webview.postMessage({ type: 'error', text: 'Prompt vacío para comparación' });
          return;
        }

        const selected = await chooseTwoModels();
        if (!selected) return;
        const [leftModel, rightModel] = selected;

        chatPanel?.webview.postMessage({ type: 'compareStart' });
        try {
          const [left, right] = await Promise.all([
            requestChatCompletion(prompt, leftModel, metrics),
            requestChatCompletion(prompt, rightModel, metrics),
          ]);
          await evaluateQualityAlerts('compare', false);
          chatPanel?.webview.postMessage({ type: 'compareResult', prompt, left, right });
        } catch (err) {
          await evaluateQualityAlerts('compare', true);
          chatPanel?.webview.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
        }
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

      if (msg.type === 'historyUpdate') {
        const normalized = await saveChatHistory(msg.messages);
        await saveSessionState(normalized);
        return;
      }

      if (msg.type === 'modelSelected') {
        selectedModel = String(msg.model || '').trim() || getConfig().model;
        await saveSessionState(loadChatHistory());
        return;
      }

      if (msg.type === 'clearHistoryRequest') {
        const confirmed = await vscode.window.showWarningMessage('¿Borrar historial de chat del workspace actual?', 'Clear', 'Cancel');
        if (confirmed !== 'Clear') return;
        await saveChatHistory([]);
        await saveSessionState([]);
        chatPanel?.webview.postMessage({ type: 'historyCleared' });
        vscode.window.showInformationMessage('Historial de chat limpiado');
        return;
      }

      if (msg.type === 'favoriteAdd') {
        const content = String(msg.content || '').trim();
        if (!content) return;

        const favorites = loadFavorites();
        const exists = favorites.some((f) => f.content.trim() === content);
        if (exists) {
          vscode.window.showInformationMessage('Este mensaje ya está en favoritos');
          return;
        }

        favorites.push({
          id: Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8),
          title: buildFavoriteTitle(content),
          content,
          timestamp: Date.now(),
        });
        await saveFavorites(favorites);
        vscode.window.showInformationMessage('Favorito guardado');
        return;
      }

      if (msg.type === 'apply') {
        const code = String(msg.code || '');
        if (!code.trim()) return;

        const editor = vscode.window.activeTextEditor;
        if (!editor) {
          const ok = await vscode.window.showInformationMessage('Apply code to editor?', 'Yes', 'No');
          if (ok !== 'Yes') return;
          const doc = await vscode.workspace.openTextDocument({ content: code, language: targetEditorLanguage(msg.lang) });
          await vscode.window.showTextDocument(doc, { preview: false });
          return;
        }

        const isDiff = code.startsWith('---') || code.startsWith('@@');
        if (isDiff) {
          const ok = await vscode.window.showInformationMessage('Apply code to editor?', 'Yes', 'No');
          if (ok !== 'Yes') return;
          const edit = new vscode.WorkspaceEdit();
          const fullRange = new vscode.Range(editor.document.positionAt(0), editor.document.positionAt(editor.document.getText().length));
          edit.replace(editor.document.uri, fullRange, code);
          await vscode.workspace.applyEdit(edit);
          try { await vscode.commands.executeCommand('editor.action.formatDocument'); } catch {}
        } else {
          await previewAndApplyCode(editor, code, msg.lang);
        }
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
    try { await vscode.commands.executeCommand('copilotLocal.chatView.focus'); } catch { await openChatPanel(); }
    const wv = getActiveWebview();
    if (wv) {
      wv.postMessage({ type: 'contextFile', file: editor.document.fileName });
      wv.postMessage({ type: 'runPrompt', text: prefix + '\n' + selected });
    }
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
        evaluateQualityAlerts('sendSelection', !!err);
        if (err) output.appendLine('\n[Error] ' + err.message);
        output.appendLine('\n--- Done ---');
      },
    );
  }));

  // ── Open Chat: Focus sidebar view ──
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openChat', async () => {
    try {
      await vscode.commands.executeCommand('copilotLocal.chatView.focus');
    } catch {
      // Fallback to legacy panel
      try { await openChatPanel(); } catch (err) {
        output.appendLine('[openChat][error] ' + (err instanceof Error ? err.message : String(err)));
      }
    }
  }));

  // ── New Chat: Clear and focus sidebar ──
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.newChat', async () => {
    await saveChatHistory([]);
    await saveSessionState([]);
    const wv = getActiveWebview();
    if (wv) wv.postMessage({ type: 'historyCleared' });
    try { await vscode.commands.executeCommand('copilotLocal.chatView.focus'); } catch {}
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openChatLegacy', async () => {
    try {
      await openChatPanel();
    } catch (err) {
      const message = err instanceof Error ? (err.stack || err.message) : String(err);
      output.appendLine('[openChat][error] ' + message);
      vscode.window.showErrorMessage('Open Chat Panel failed. Revisa Output > Copilot Local para detalles.');
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.semanticSearch', async () => {
    const query = await vscode.window.showInputBox({
      title: 'Semantic Search',
      prompt: 'Consulta semantica (ej: auth middleware)',
      ignoreFocusOut: true,
    });
    const cleanQuery = String(query || '').trim();
    if (!cleanQuery) return;

    const topRaw = await vscode.window.showInputBox({
      title: 'Semantic Search',
      prompt: 'Top resultados (1-20)',
      value: '8',
      ignoreFocusOut: true,
    });
    const top = Math.max(1, Math.min(20, Number.parseInt(String(topRaw || '8'), 10) || 8));

    try {
      const res = await postJSON('/api/v2/search', { query: cleanQuery, top });
      const hits = Array.isArray(res?.result) ? res.result : [];
      if (hits.length === 0) {
        vscode.window.showInformationMessage('Semantic search: sin resultados');
        return;
      }

      const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || '';
      const toAbsolutePath = (candidate) => {
        const c = String(candidate || '').trim();
        if (!c) return '';
        if (path.isAbsolute(c) && fs.existsSync(c)) return c;
        if (!root) return '';
        const normalized = c.replace(/^\.?\//, '');
        const direct = path.join(root, normalized);
        if (fs.existsSync(direct)) return direct;
        const apiScoped = path.join(root, 'api', normalized);
        if (fs.existsSync(apiScoped)) return apiScoped;
        return '';
      };

      const picks = hits.slice(0, 80).map((hit, idx) => {
        const score = Number(hit?.score || 0);
        const payload = (hit && typeof hit.payload === 'object' && hit.payload) ? hit.payload : {};
        const pathCandidate = String(payload.path || payload.file_path || payload.file || payload.filename || '');
        const snippet = String(payload.code || payload.text || payload.content || payload.summary || '').replace(/\s+/g, ' ').trim();
        const labelBase = pathCandidate ? path.basename(pathCandidate) : ('resultado-' + String(idx + 1));
        return {
          label: labelBase + ' | score=' + score.toFixed(3),
          description: pathCandidate || '(sin ruta)',
          detail: snippet.length > 180 ? snippet.slice(0, 180) + '…' : (snippet || 'sin snippet'),
          value: { pathCandidate, snippet, hit },
        };
      });

      const picked = await vscode.window.showQuickPick(picks, {
        title: 'Semantic Search Results',
        placeHolder: 'Selecciona un resultado para abrirlo',
        matchOnDescription: true,
        matchOnDetail: true,
      });
      if (!picked) return;

      const absPath = toAbsolutePath(picked.value.pathCandidate);
      if (absPath) {
        const doc = await vscode.workspace.openTextDocument(absPath);
        const editor = await vscode.window.showTextDocument(doc, { preview: false });
        const line = Number(picked.value.hit?.payload?.line || picked.value.hit?.payload?.line_number || 0);
        if (line > 0 && line <= doc.lineCount) {
          const pos = new vscode.Position(line - 1, 0);
          editor.selection = new vscode.Selection(pos, pos);
          editor.revealRange(new vscode.Range(pos, pos), vscode.TextEditorRevealType.InCenter);
        }
        return;
      }

      const fallback = [
        'No se pudo resolver una ruta local para este resultado.',
        '',
        'Path reportado: ' + (picked.value.pathCandidate || '(none)'),
        'Snippet:',
        picked.value.snippet || '(none)',
        '',
        'Raw payload:',
        JSON.stringify(picked.value.hit || {}, null, 2),
      ].join('\n');
      const doc = await vscode.workspace.openTextDocument({ content: fallback, language: 'json' });
      await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });
    } catch (err) {
      vscode.window.showErrorMessage('Semantic search failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.validateLocalModels', async () => {
    const status = await validateLocalModels();
    if (chatPanel) {
      chatPanel.webview.postMessage({
        type: 'modelsValidation',
        ok: status.ok,
        count: status.count,
        source: status.source,
        error: status.error || '',
      });
    }

    if (status.ok) {
      vscode.window.showInformationMessage('Modelos locales validados: ' + status.count + ' disponibles');
      return;
    }
    vscode.window.showWarningMessage('No se pudieron validar modelos locales: ' + (status.error || 'error desconocido'));
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.switchProfile', async () => {
    const cfg = getConfig();
    const profiles = cfg.profiles || {};
    const picks = Object.entries(profiles).map(([id, p]) => ({
      label: id,
      description: id === cfg.activeProfileId ? 'activo' : '',
      detail: 'model=' + p.model + ' | lang=' + p.lang + ' | temperature=' + Number(p.temperature || 0).toFixed(2),
      value: { id, profile: p },
    }));

    const picked = await vscode.window.showQuickPick(picks, {
      title: 'Switch Workspace Profile',
      placeHolder: 'Selecciona perfil para este workspace',
      matchOnDescription: true,
      matchOnDetail: true,
    });
    if (!picked) return;

    const wsCfg = vscode.workspace.getConfiguration('copilotLocal');
    await wsCfg.update('workspaceActiveProfile', picked.value.id, vscode.ConfigurationTarget.Workspace);
    await wsCfg.update('workspaceModel', picked.value.profile.model, vscode.ConfigurationTarget.Workspace);
    await wsCfg.update('workspaceLang', picked.value.profile.lang, vscode.ConfigurationTarget.Workspace);
    await wsCfg.update('workspaceTemperature', picked.value.profile.temperature, vscode.ConfigurationTarget.Workspace);
    selectedModel = picked.value.profile.model;

    refreshProfileStatus();
    await publishProfileToPanel();
    await saveSessionState(loadChatHistory());
    vscode.window.showInformationMessage('Perfil cambiado a: ' + picked.value.id);
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.clearSessionState', async () => {
    await context.workspaceState.update(CHAT_SESSION_STATE_KEY, null);
    activeSessionId = '';
    selectedModel = getConfig().model;
    shouldShowRestoredBanner = false;
    vscode.window.showInformationMessage('Session state cleared');
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.sendSelectionToChat', async () => {
    const editor = vscode.window.activeTextEditor;
    const text = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection) || '';
    try { await vscode.commands.executeCommand('copilotLocal.chatView.focus'); } catch { await openChatPanel(); }
    const wv = getActiveWebview();
    if (wv && editor) wv.postMessage({ type: 'contextFile', file: editor.document.fileName });
    if (wv && text.trim()) wv.postMessage({ type: 'prefill', text });
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.quickPrompt', async () => {
    const editor = vscode.window.activeTextEditor;
    const selected = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection) || '';

    const templates = getQuickPromptTemplates();
    if (templates.length === 0) {
      vscode.window.showInformationMessage('No hay plantillas configuradas');
      return;
    }

    const lastId = context.workspaceState.get(QUICK_PROMPT_LAST_KEY, '');
    const picks = templates.map((t) => ({
      label: t.label,
      description: t.source === 'custom' ? 'custom' : 'built-in',
      detail: t.template,
      value: t,
    }));
    const activeItem = picks.find((p) => p.value.id === lastId);

    const picked = await vscode.window.showQuickPick(picks, {
      title: 'Quick Prompt Templates',
      placeHolder: 'Selecciona una plantilla para insertar en el chat',
      matchOnDescription: true,
      matchOnDetail: true,
      activeItem,
    });
    if (!picked) return;

    const finalPrompt = applyTemplateToSelection(picked.value.template, selected);
    const wv = await focusChatAndGetWebview();
    wv.postMessage({ type: 'prefill', text: finalPrompt });
    await context.workspaceState.update(QUICK_PROMPT_LAST_KEY, picked.value.id);
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.searchHistory', async () => {
    const history = loadChatHistory();
    if (history.length === 0) {
      vscode.window.showInformationMessage('No hay historial de chat para este workspace');
      return;
    }

    const query = (await vscode.window.showInputBox({
      title: 'Search Chat History',
      prompt: 'Texto a buscar en mensajes previos (vacío para listar todo)',
      ignoreFocusOut: true,
    })) || '';

    const q = query.trim().toLowerCase();
    const filtered = q
      ? history.filter((m) => String(m.content || '').toLowerCase().includes(q))
      : history;

    if (filtered.length === 0) {
      vscode.window.showInformationMessage('No se encontraron coincidencias en el historial');
      return;
    }

    const picks = filtered.slice(-200).reverse().map((m) => {
      const text = String(m.content || '').replace(/\s+/g, ' ').trim();
      const short = text.length > 120 ? text.slice(0, 120) + '…' : text;
      return {
        label: (m.role === 'assistant' ? 'AI' : 'You') + ': ' + short,
        description: new Date(Number(m.timestamp || Date.now())).toLocaleString(),
        detail: 'id: ' + m.id,
        value: m,
      };
    });

    const picked = await vscode.window.showQuickPick(picks, {
      title: 'Search Chat History',
      placeHolder: 'Selecciona un mensaje para abrir y resaltar en el chat',
      matchOnDescription: true,
      matchOnDetail: true,
    });
    if (!picked) return;

    const wv = await focusChatAndGetWebview();
    wv.postMessage({ type: 'hydrateHistory', messages: history });
    wv.postMessage({ type: 'highlightHistory', id: picked.value.id });
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.compareModels', async () => {
    const wv = await focusChatAndGetWebview();
    wv.postMessage({ type: 'openCompareMode' });

    const editor = vscode.window.activeTextEditor;
    const selected = editor?.document.getText(editor.selection.isEmpty ? undefined : editor.selection)?.trim() || '';
    if (!selected) {
      vscode.window.showInformationMessage('Selecciona texto o escribe un prompt en el chat para comparar modelos');
      return;
    }

    wv.postMessage({ type: 'prefill', text: selected });
    const selectedModels = await chooseTwoModels();
    if (!selectedModels) return;
    const [leftModel, rightModel] = selectedModels;

    wv.postMessage({ type: 'compareStart' });
    try {
      const [left, right] = await Promise.all([
        requestChatCompletion(selected, leftModel, metrics),
        requestChatCompletion(selected, rightModel, metrics),
      ]);
      await evaluateQualityAlerts('compare-command', false);
      wv.postMessage({ type: 'compareResult', prompt: selected, left, right });
    } catch (err) {
      await evaluateQualityAlerts('compare-command', true);
      wv.postMessage({ type: 'error', text: err instanceof Error ? err.message : String(err) });
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.resetQualityAlerts', async () => {
    consecutiveErrors = 0;
    activeQualityAlerts.clear();
    qualityStatus.hide();
    output.appendLine('[quality][reset] quality alerts reset by user');
    vscode.window.showInformationMessage('Quality alerts reset');
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.openFavorites', async () => {
    let favorites = loadFavorites();
    if (favorites.length === 0) {
      vscode.window.showInformationMessage('No hay favoritos guardados en este workspace');
      return;
    }

    while (favorites.length > 0) {
      const picks = favorites.slice().reverse().map((f) => ({
        label: f.title,
        description: new Date(Number(f.timestamp || Date.now())).toLocaleString(),
        detail: f.content.length > 140 ? f.content.slice(0, 140) + '…' : f.content,
        value: f,
      }));

      const picked = await vscode.window.showQuickPick(picks, {
        title: 'Favorites',
        placeHolder: 'Selecciona un favorito',
        matchOnDescription: true,
        matchOnDetail: true,
      });
      if (!picked) return;

      const action = await vscode.window.showQuickPick([
        { label: 'Copy', value: 'copy' },
        { label: 'Apply', value: 'apply' },
        { label: 'Delete', value: 'delete' },
      ], {
        title: 'Favorites: ' + picked.value.title,
        placeHolder: 'Elige una acción',
      });
      if (!action) return;

      if (action.value === 'copy') {
        await vscode.env.clipboard.writeText(picked.value.content);
        vscode.window.showInformationMessage('Favorito copiado al portapapeles');
        continue;
      }

      if (action.value === 'apply') {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
          const doc = await vscode.workspace.openTextDocument({ content: picked.value.content, language: 'markdown' });
          await vscode.window.showTextDocument(doc, { preview: false });
        } else {
          await editor.edit((editBuilder) => editBuilder.insert(editor.selection.active, picked.value.content));
        }
        continue;
      }

      if (action.value === 'delete') {
        const confirmed = await vscode.window.showWarningMessage('¿Eliminar favorito seleccionado?', 'Delete', 'Cancel');
        if (confirmed !== 'Delete') {
          continue;
        }
        favorites = favorites.filter((f) => f.id !== picked.value.id);
        await saveFavorites(favorites);
        if (favorites.length === 0) {
          vscode.window.showInformationMessage('No quedan favoritos en este workspace');
          return;
        }
      }
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.explainSelection', async () => {
    await sendSelectionPrompt('Explain this code:');
  }));
  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.refactorSelection', async () => {
    await sendSelectionPrompt('Refactor this code for clarity. Also include a concise diff-style explanation of what changed and why.');
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

      const wv = await focusChatAndGetWebview();
      wv.postMessage({ type: 'externalResult', text });
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

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.explainTestFailure', async () => {
    const ctx = await collectExplainTestFailureContext();
    if (!ctx) {
      vscode.window.showInformationMessage('No hay salida de test para analizar');
      return;
    }

    const wv = await focusChatAndGetWebview();
    wv.postMessage({ type: 'regressionDraft', text: ctx.regressionPrompt });
    wv.postMessage({ type: 'runPrompt', text: ctx.prompt });
    vscode.window.showInformationMessage('Analisis de test failure enviado al chat (' + ctx.source + ')');
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.joinSession', async () => {
    const sessionId = await vscode.window.showInputBox({ title: 'Join Shared Session', prompt: 'Ingresa session_id', ignoreFocusOut: true });
    if (!sessionId || !sessionId.trim()) return;

    const cleanID = sessionId.trim();
    try {
      const endpoint = '/api/sessions/' + encodeURIComponent(cleanID) + '/join';
      await postJSON(endpoint, {});
      activeSessionId = cleanID;
      await saveSessionState(loadChatHistory());
      const wv = await focusChatAndGetWebview();
      wv.postMessage({ type: 'externalResult', text: 'Connected to shared session: ' + cleanID });
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

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.architectReview', async () => {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showInformationMessage('No active editor');
      return;
    }

    try {
      const result = await postJSON('/api/architect/patterns', { path: editor.document.fileName });
      const suggestions = Array.isArray(result?.suggestions) ? result.suggestions : [];
      const lines = [
        'Architect Review',
        'Path: ' + String(result?.path || editor.document.fileName),
        'Risk: ' + String(result?.risk_level || 'unknown') + ' (' + String(result?.risk_score || 0) + ')',
        '',
        'Suggestions:',
      ];
      if (suggestions.length === 0) {
        lines.push('- No suggestions');
      } else {
        for (const s of suggestions) {
          lines.push('- [' + String(s?.severity || 'low') + '] ' + String(s?.pattern || 'pattern') + ': ' + String(s?.suggestion || ''));
        }
      }

      const wv = await focusChatAndGetWebview();
      wv.postMessage({ type: 'externalResult', text: lines.join('\n') });
    } catch (err) {
      vscode.window.showErrorMessage('Architect review failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.securityScanCurrentFile', async () => {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showInformationMessage('No active editor');
      return;
    }

    try {
      const result = await postJSON('/api/security/scan/file', { path: editor.document.fileName });
      const findings = Array.isArray(result?.findings) ? result.findings : [];
      const lines = [
        'Security Scan - Current File',
        'Path: ' + String(result?.path || editor.document.fileName),
        'Findings: ' + String(result?.count || findings.length),
        '',
      ];
      if (findings.length === 0) {
        lines.push('No findings');
      } else {
        for (const f of findings.slice(0, 40)) {
          lines.push('- [' + String(f?.severity || 'medium') + '] line ' + String(f?.line || 0) + ': ' + String(f?.description || '')); 
        }
      }
      const wv = await focusChatAndGetWebview();
      wv.postMessage({ type: 'externalResult', text: lines.join('\n') });
    } catch (err) {
      vscode.window.showErrorMessage('Security scan failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.generatePipelineAndOpen', async () => {
    const platformPick = await vscode.window.showQuickPick([
      { label: 'GitHub Actions', value: 'github-actions' },
      { label: 'GitLab CI', value: 'gitlab-ci' },
      { label: 'Jenkins', value: 'jenkins' },
    ], { title: 'Generate CI/CD Pipeline', placeHolder: 'Select platform' });
    if (!platformPick) return;

    try {
      const result = await postJSON('/api/cicd/generate', { platform: platformPick.value, apply: false });
      const pipeline = String(result?.pipeline || '').trim();
      if (!pipeline) {
        vscode.window.showErrorMessage('No pipeline generated');
        return;
      }
      const doc = await vscode.workspace.openTextDocument({ content: pipeline, language: 'yaml' });
      await vscode.window.showTextDocument(doc, { preview: false, viewColumn: vscode.ViewColumn.Beside });
      vscode.window.showInformationMessage('Pipeline generado para ' + platformPick.value);
    } catch (err) {
      vscode.window.showErrorMessage('Pipeline generation failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand('copilot-local.createPRSummary', async () => {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!workspaceFolder) {
      vscode.window.showErrorMessage('No workspace folder abierto');
      return;
    }

    let diff = '';
    try {
      diff = await runGitDiffCached(workspaceFolder);
    } catch (err) {
      vscode.window.showErrorMessage('No se pudo leer staged diff: ' + (err instanceof Error ? err.message : String(err)));
      return;
    }
    if (!diff.trim()) {
      vscode.window.showInformationMessage('No hay cambios staged para resumir');
      return;
    }

    try {
      const result = await postJSON('/api/pr/summary', { diff });
      const text = String(result?.summary || result?.executive_summary || JSON.stringify(result, null, 2)).trim();
      const wv = await focusChatAndGetWebview();
      wv.postMessage({ type: 'externalResult', text: 'PR Summary\n\n' + text });
      await vscode.env.clipboard.writeText(text);
      vscode.window.showInformationMessage('PR summary generado y copiado al portapapeles');
    } catch (err) {
      vscode.window.showErrorMessage('PR summary failed: ' + (err instanceof Error ? err.message : String(err)));
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

  context.subscriptions.push(vscode.workspace.onDidChangeConfiguration(async (event) => {
    if (!event.affectsConfiguration('copilotLocal')) return;
    loadPromptRC();
    selectedModel = getConfig().model;
    refreshProfileStatus();
    await publishProfileToPanel();
    await saveSessionState(loadChatHistory());
  }));
}

function deactivate() {}

module.exports = { activate, deactivate };
