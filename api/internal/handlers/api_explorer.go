package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type APIExplorerHandler struct {
	routes []domain.RouteDefinition
}

func NewAPIExplorerHandler(routes []domain.RouteDefinition) *APIExplorerHandler {
	copyRoutes := append([]domain.RouteDefinition(nil), routes...)
	return &APIExplorerHandler{routes: copyRoutes}
}

func (h *APIExplorerHandler) Handle(w http.ResponseWriter, r *http.Request) {
	routesJSON, err := json.Marshal(h.routes)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "no se pudo renderizar rutas")
		return
	}

	html := strings.Replace(apiExplorerHTML, "__ROUTES_JSON__", string(routesJSON), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (h *APIExplorerHandler) Routes(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"count":  len(h.routes),
		"routes": h.routes,
	})
}

const apiExplorerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Gateway API Explorer</title>
  <style>
    :root {
      --bg: #0a0d12;
      --bg-soft: #0f1724;
      --panel: #111723;
      --panel-soft: #182133;
      --text: #d8e2f0;
      --muted: #8ca0bc;
      --accent: #34d399;
      --accent-2: #22d3ee;
      --danger: #f87171;
      --warning: #f59e0b;
      --border: rgba(255,255,255,0.08);
    }

    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: radial-gradient(circle at 8% 0%, #1a2b45 0%, var(--bg) 40%), var(--bg);
      color: var(--text);
      font-family: "JetBrains Mono", "SF Mono", Menlo, Consolas, monospace;
      min-height: 100vh;
      padding: 24px;
    }
    h1 {
      margin: 0;
      font-size: 28px;
      letter-spacing: 0.5px;
    }
    .subtitle {
      margin-top: 8px;
      color: var(--muted);
      font-size: 13px;
      max-width: 920px;
    }
    .toolbar {
      margin-top: 18px;
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 12px;
      background: linear-gradient(180deg, var(--panel-soft), var(--panel));
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 14px;
    }
    .toolbar label {
      display: block;
      color: var(--muted);
      font-size: 12px;
      margin-bottom: 6px;
      text-transform: uppercase;
      letter-spacing: 0.8px;
    }
    input[type="text"] {
      width: 100%;
      background: #0b1320;
      border: 1px solid var(--border);
      color: var(--text);
      border-radius: 10px;
      padding: 10px 12px;
      font-family: inherit;
      font-size: 13px;
    }
    button {
      background: #1f2a3a;
      color: var(--text);
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 9px 12px;
      cursor: pointer;
      font-family: inherit;
      font-size: 12px;
      letter-spacing: 0.5px;
      transition: 120ms ease;
    }
    button:hover {
      border-color: var(--accent);
      transform: translateY(-1px);
    }
    .count {
      margin-top: 10px;
      color: var(--muted);
      font-size: 12px;
    }
    .list {
      margin-top: 16px;
      display: grid;
      gap: 14px;
    }
    .route-card {
      background: linear-gradient(180deg, var(--panel-soft), var(--panel));
      border: 1px solid var(--border);
      border-radius: 12px;
      overflow: hidden;
    }
    .route-head {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 10px;
      align-items: center;
      padding: 12px 14px;
      border-bottom: 1px solid var(--border);
    }
    .method {
      display: inline-block;
      min-width: 62px;
      text-align: center;
      font-weight: 700;
      font-size: 11px;
      border-radius: 999px;
      padding: 5px 8px;
      border: 1px solid transparent;
    }
    .method.GET { color: #7dd3fc; border-color: #0e7490; }
    .method.POST { color: #86efac; border-color: #166534; }
    .method.PUT { color: #fcd34d; border-color: #92400e; }
    .method.DELETE { color: #fda4af; border-color: #991b1b; }
    .path {
      font-size: 13px;
      font-weight: 700;
      word-break: break-all;
    }
    .tags {
      display: flex;
      gap: 8px;
      align-items: center;
      justify-content: flex-end;
      flex-wrap: wrap;
    }
    .tag {
      font-size: 10px;
      border: 1px solid var(--border);
      border-radius: 999px;
      padding: 4px 7px;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.8px;
    }
    .tag.protected { color: #fbbf24; border-color: #92400e; }
    .tag.local { color: #93c5fd; border-color: #1e3a8a; }
    .tag.sse { color: var(--accent-2); border-color: #155e75; }
    .route-body {
      padding: 12px 14px;
      display: grid;
      gap: 10px;
    }
    .desc { color: var(--muted); font-size: 12px; line-height: 1.4; }
    textarea {
      width: 100%;
      min-height: 104px;
      resize: vertical;
      background: #0b1320;
      border: 1px solid var(--border);
      color: var(--text);
      border-radius: 10px;
      padding: 10px;
      font-family: inherit;
      font-size: 12px;
      line-height: 1.35;
    }
    .actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
    }
    .result {
      background: rgba(0,0,0,0.28);
      border-radius: 10px;
      border: 1px solid var(--border);
      padding: 10px;
      max-height: 280px;
      overflow: auto;
      font-size: 12px;
      line-height: 1.35;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .status-line {
      font-size: 11px;
      color: var(--muted);
      margin-bottom: 6px;
    }
    .err { color: var(--danger); }
    .json-key { color: #93c5fd; }
    .json-string { color: #86efac; }
    .json-number { color: #fcd34d; }
    .json-bool { color: #f9a8d4; }

    @media (max-width: 880px) {
      body { padding: 12px; }
      .toolbar { grid-template-columns: 1fr; }
      .route-head { grid-template-columns: 1fr; }
      .tags { justify-content: flex-start; }
    }
  </style>
</head>
<body>
  <h1>Embedded API Explorer</h1>
  <div class="subtitle">
    Explora y prueba endpoints del gateway desde localhost. Para rutas protegidas, ingresa JWT y se enviara automaticamente como Authorization: Bearer token.
  </div>

  <section class="toolbar">
    <div>
      <label for="jwtToken">JWT Token</label>
      <input id="jwtToken" type="text" placeholder="eyJhbGciOi..." />
    </div>
    <div style="display:flex;align-items:flex-end;">
      <button id="clearAll">Clear Responses</button>
    </div>
  </section>

  <div class="count" id="count"></div>
  <section class="list" id="routeList"></section>

  <script>
    const ROUTES = __ROUTES_JSON__;

    const routeListEl = document.getElementById('routeList');
    const countEl = document.getElementById('count');
    const jwtInput = document.getElementById('jwtToken');
    const sseConnections = new Map();

    countEl.textContent = ROUTES.length + ' endpoints disponibles';

    function createEl(tag, className, text) {
      const el = document.createElement(tag);
      if (className) el.className = className;
      if (typeof text === 'string') el.textContent = text;
      return el;
    }

    function safeJSONStringify(v) {
      try { return JSON.stringify(v, null, 2); } catch (_) { return String(v); }
    }

    function syntaxHighlightJSON(value) {
      const escaped = value
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;');
      return escaped.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"\s*:?)|(\btrue\b|\bfalse\b)|(\bnull\b)|(-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (m) {
        if (/^".*":$/.test(m)) return '<span class="json-key">' + m + '</span>';
        if (/^"/.test(m)) return '<span class="json-string">' + m + '</span>';
        if (/true|false|null/.test(m)) return '<span class="json-bool">' + m + '</span>';
        return '<span class="json-number">' + m + '</span>';
      });
    }

    function setResult(resultEl, statusLineEl, statusText, payload, isError) {
      statusLineEl.textContent = statusText;
      statusLineEl.className = 'status-line' + (isError ? ' err' : '');
      const text = typeof payload === 'string' ? payload : safeJSONStringify(payload);
      const isLikelyJSON = text.trim().startsWith('{') || text.trim().startsWith('[');
      resultEl.innerHTML = isLikelyJSON ? syntaxHighlightJSON(text) : text;
    }

    async function runRequest(route, bodyText, resultEl, statusLineEl) {
      if (route.sse) {
        if (sseConnections.has(route.path)) {
          const existing = sseConnections.get(route.path);
          existing.close();
          sseConnections.delete(route.path);
          setResult(resultEl, statusLineEl, 'SSE desconectado', '', false);
          return;
        }

        setResult(resultEl, statusLineEl, 'Conectando SSE...', '', false);
        const es = new EventSource(route.path);
        sseConnections.set(route.path, es);
        let lines = [];
        es.onmessage = (ev) => {
          let line = ev.data;
          try {
            line = safeJSONStringify(JSON.parse(ev.data));
          } catch (_) {}
          lines.push(line);
          if (lines.length > 200) lines = lines.slice(lines.length - 200);
          setResult(resultEl, statusLineEl, 'SSE conectado', lines.join('\n'), false);
        };
        es.onerror = () => {
          setResult(resultEl, statusLineEl, 'SSE error/desconectado', 'La conexion SSE fallo o fue cerrada.', true);
          es.close();
          sseConnections.delete(route.path);
        };
        return;
      }

      const headers = { 'Accept': 'application/json' };
      if (route.method !== 'GET') {
        headers['Content-Type'] = 'application/json';
      }
      const token = jwtInput.value.trim();
      if (token && route.protected) {
        headers['Authorization'] = 'Bearer ' + token;
      }

      let bodyValue;
      if (route.method !== 'GET') {
        const trimmed = bodyText.trim();
        if (trimmed !== '') {
          try {
            bodyValue = JSON.stringify(JSON.parse(trimmed));
          } catch (e) {
            setResult(resultEl, statusLineEl, 'Body invalido', String(e), true);
            return;
          }
        } else {
          bodyValue = '{}';
        }
      }

      const init = { method: route.method, headers };
      if (bodyValue !== undefined) init.body = bodyValue;

      try {
        const res = await fetch(route.path, init);
        const text = await res.text();
        let payload = text;
        try { payload = JSON.parse(text); } catch (_) {}
        setResult(resultEl, statusLineEl, res.status + ' ' + res.statusText, payload, !res.ok);
      } catch (e) {
        setResult(resultEl, statusLineEl, 'Request error', String(e), true);
      }
    }

    function renderRoutes() {
      for (const route of ROUTES) {
        const card = createEl('article', 'route-card');
        const head = createEl('header', 'route-head');

        const method = createEl('span', 'method ' + route.method, route.method);
        const path = createEl('div', 'path', route.path);
        const tags = createEl('div', 'tags');
        if (route.protected) tags.appendChild(createEl('span', 'tag protected', 'JWT'));
        if (route.localhost_only) tags.appendChild(createEl('span', 'tag local', 'localhost'));
        if (route.sse) tags.appendChild(createEl('span', 'tag sse', 'SSE'));

        head.appendChild(method);
        head.appendChild(path);
        head.appendChild(tags);

        const body = createEl('div', 'route-body');
        body.appendChild(createEl('div', 'desc', route.description || ''));

        const textarea = createEl('textarea');
        textarea.value = route.example_body || '{}';
        if (route.method === 'GET' || route.sse) {
          textarea.value = route.example_body || '';
          textarea.placeholder = 'GET/SSE sin body';
        }

        const actions = createEl('div', 'actions');
        const tryBtn = createEl('button', '', route.sse ? 'Connect/Disconnect SSE' : 'Try it');
        const clearBtn = createEl('button', '', 'Clear');
        actions.appendChild(tryBtn);
        actions.appendChild(clearBtn);

        const statusLine = createEl('div', 'status-line', 'Listo');
        const result = createEl('pre', 'result', '');

        tryBtn.addEventListener('click', () => runRequest(route, textarea.value, result, statusLine));
        clearBtn.addEventListener('click', () => {
          statusLine.textContent = 'Listo';
          statusLine.className = 'status-line';
          result.textContent = '';
        });

        body.appendChild(textarea);
        body.appendChild(actions);
        body.appendChild(statusLine);
        body.appendChild(result);

        card.appendChild(head);
        card.appendChild(body);
        routeListEl.appendChild(card);
      }
    }

    document.getElementById('clearAll').addEventListener('click', () => {
      document.querySelectorAll('.result').forEach((x) => { x.textContent = ''; });
      document.querySelectorAll('.status-line').forEach((x) => {
        x.textContent = 'Listo';
        x.className = 'status-line';
      });
    });

    renderRoutes();
  </script>
</body>
</html>`
