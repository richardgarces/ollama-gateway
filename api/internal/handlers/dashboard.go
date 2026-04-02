package handlers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/observability"
	"ollama-gateway/pkg/httputil"
)

type dashboardMetricsCollector interface {
	Snapshot() observability.MetricsSnapshot
}

type dashboardIndexerStatus interface {
	Status() map[string]interface{}
}

type DashboardHandler struct {
	cfg     *config.Config
	metrics dashboardMetricsCollector
	indexer dashboardIndexerStatus
	logs    *observability.LogStream
	version string
}

func NewDashboardHandler(cfg *config.Config, metrics dashboardMetricsCollector, indexer dashboardIndexerStatus, logs *observability.LogStream) *DashboardHandler {
	version := strings.TrimSpace(os.Getenv("APP_VERSION"))
	if version == "" {
		version = "dev"
	}
	return &DashboardHandler{cfg: cfg, metrics: metrics, indexer: indexer, logs: logs, version: version}
}

func (h *DashboardHandler) Handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

func (h *DashboardHandler) Status(w http.ResponseWriter, r *http.Request) {
	snap := observability.MetricsSnapshot{}
	if h.metrics != nil {
		snap = h.metrics.Snapshot()
	}

	indexStatus := map[string]interface{}{}
	if h.indexer != nil {
		indexStatus = h.indexer.Status()
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"server": map[string]interface{}{
			"started_at":     snap.StartedAt,
			"uptime_seconds": snap.UptimeSeconds,
			"version":        h.version,
			"config": map[string]interface{}{
				"port":                      h.cfg.Port,
				"log_level":                 h.cfg.LogLevel,
				"repo_root":                 h.cfg.RepoRoot,
				"repo_roots":                h.cfg.RepoRoots,
				"qdrant_url":                h.cfg.QdrantURL,
				"cache_backend":             h.cfg.CacheBackend,
				"vector_store_prefer_local": h.cfg.VectorStorePreferLocal,
			},
		},
		"indexer": indexStatus,
	})
}

func (h *DashboardHandler) LogsStream(w http.ResponseWriter, r *http.Request) {
	if h.logs == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "log stream no disponible")
		return
	}
	if err := httputil.WriteSSEHeaders(w); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, event := range h.logs.Recent(100) {
		_ = httputil.WriteSSEData(w, map[string]interface{}{"type": "log", "event": event})
	}

	ch, unsubscribe := h.logs.Subscribe()
	defer unsubscribe()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := httputil.WriteSSEData(w, map[string]interface{}{"type": "log", "event": event}); err != nil {
				return
			}
		case <-ticker.C:
			if err := httputil.WriteSSEData(w, map[string]interface{}{"type": "heartbeat", "ts": time.Now().UTC()}); err != nil {
				return
			}
		}
	}
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Gateway Dashboard</title>
  <style>
    :root {
      --bg: #0a0d12;
      --panel: #111723;
      --panel-soft: #182133;
      --text: #d8e2f0;
      --muted: #8ca0bc;
      --accent: #34d399;
      --warning: #f59e0b;
      --danger: #f87171;
      --border: rgba(255,255,255,0.08);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "JetBrains Mono", "SF Mono", Menlo, Consolas, monospace;
      color: var(--text);
      background: radial-gradient(circle at 10% 0%, #1b2b40 0%, var(--bg) 40%), var(--bg);
      min-height: 100vh;
      padding: 24px;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(12, 1fr);
      gap: 16px;
    }
    .card {
      background: linear-gradient(180deg, var(--panel-soft), var(--panel));
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 16px;
      box-shadow: 0 12px 30px rgba(0,0,0,0.25);
    }
    .card h3 {
      margin: 0 0 10px;
      font-size: 13px;
      letter-spacing: 1px;
      color: var(--muted);
      text-transform: uppercase;
    }
    .span-4 { grid-column: span 4; }
    .span-8 { grid-column: span 8; }
    .span-12 { grid-column: span 12; }
    .kpi {
      font-size: 30px;
      font-weight: 700;
      color: var(--accent);
      margin-top: 8px;
    }
    .sub { color: var(--muted); font-size: 12px; }
    .row { display: flex; gap: 10px; flex-wrap: wrap; }
    button {
      background: #1f2a3a;
      color: var(--text);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 8px 12px;
      cursor: pointer;
    }
    button:hover { border-color: var(--accent); }
    pre {
      margin: 0;
      max-height: 240px;
      overflow: auto;
      background: rgba(0,0,0,0.25);
      border-radius: 8px;
      padding: 12px;
      font-size: 12px;
      line-height: 1.35;
    }
    .logs {
      height: 320px;
      overflow: auto;
      font-size: 12px;
      background: rgba(0,0,0,0.25);
      border-radius: 8px;
      padding: 8px;
    }
    .log { padding: 4px 0; border-bottom: 1px dashed rgba(255,255,255,0.08); }
    .log.err { color: var(--danger); }
    .log.warn { color: var(--warning); }
    @media (max-width: 1000px) {
      .span-4, .span-8 { grid-column: span 12; }
      body { padding: 12px; }
    }
  </style>
</head>
<body>
  <div class="grid">
    <section class="card span-4">
      <h3>Server</h3>
      <div class="kpi" id="uptime">0s</div>
      <div class="sub" id="version">version: -</div>
      <pre id="serverCfg">{}</pre>
    </section>

    <section class="card span-4">
      <h3>Traffic</h3>
      <div class="kpi" id="rpm">0</div>
      <div class="sub">requests/min</div>
      <div class="sub">p50: <span id="p50">0</span> ms</div>
      <div class="sub">p95: <span id="p95">0</span> ms</div>
      <div class="sub">errors: <span id="errors">0</span></div>
    </section>

    <section class="card span-4">
      <h3>Indexer</h3>
      <div class="sub">indexed files: <span id="indexedFiles">0</span></div>
      <div class="sub">watcher active: <span id="watcherActive">false</span></div>
      <div class="sub">reindexing: <span id="reindexing">false</span></div>
      <div class="sub">last reindex: <span id="lastReindex">-</span></div>
      <div class="row" style="margin-top: 10px;">
        <button data-action="reindex">Reindex</button>
        <button data-action="start">Start Watcher</button>
        <button data-action="stop">Stop Watcher</button>
        <button data-action="reset">Reset State</button>
      </div>
    </section>

    <section class="card span-8">
      <h3>Routes Metrics</h3>
      <pre id="routes">[]</pre>
    </section>

    <section class="card span-4">
      <h3>Logs (SSE)</h3>
      <div class="logs" id="logs"></div>
    </section>
  </div>

  <script>
    const state = { lastTotal: 0, lastAt: Date.now() };
    const logsEl = document.getElementById('logs');

    function percentile(values, p) {
      if (!values.length) return 0;
      const sorted = [...values].sort((a, b) => a - b);
      const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length));
      return sorted[idx] || 0;
    }

    function addLog(line, level) {
      const div = document.createElement('div');
      div.className = 'log ' + ((level || '').toLowerCase());
      div.textContent = line;
      logsEl.appendChild(div);
      while (logsEl.children.length > 300) logsEl.removeChild(logsEl.firstChild);
      logsEl.scrollTop = logsEl.scrollHeight;
    }

    async function refreshStatus() {
      const [statusRes, metricsRes, indexRes] = await Promise.all([
        fetch('/internal/dashboard/status'),
        fetch('/metrics'),
        fetch('/internal/index/status'),
      ]);

      if (statusRes.ok) {
        const status = await statusRes.json();
        document.getElementById('uptime').textContent = (status.server?.uptime_seconds || 0) + 's';
        document.getElementById('version').textContent = 'version: ' + (status.server?.version || '-');
        document.getElementById('serverCfg').textContent = JSON.stringify(status.server?.config || {}, null, 2);
      }

      if (metricsRes.ok) {
        const m = await metricsRes.json();
        const now = Date.now();
        const dtMin = Math.max((now - state.lastAt) / 60000, 1 / 60);
        const rpm = Math.max(0, Math.round(((m.total_requests || 0) - state.lastTotal) / dtMin));
        state.lastTotal = m.total_requests || 0;
        state.lastAt = now;
        document.getElementById('rpm').textContent = String(rpm);

        const latencies = (m.routes || []).map(x => x.average_latency_ms || 0).filter(x => x > 0);
        document.getElementById('p50').textContent = String(Math.round(percentile(latencies, 50)));
        document.getElementById('p95').textContent = String(Math.round(percentile(latencies, 95)));
        const errors = (m.routes || []).reduce((acc, r) => acc + (r.errors || 0), 0);
        document.getElementById('errors').textContent = String(errors);
        document.getElementById('routes').textContent = JSON.stringify(m.routes || [], null, 2);
      }

      if (indexRes.ok) {
        const idx = await indexRes.json();
        document.getElementById('indexedFiles').textContent = String(idx.indexed_files || 0);
        document.getElementById('watcherActive').textContent = String(!!idx.watcher_active);
        document.getElementById('reindexing').textContent = String(!!idx.reindexing);
        document.getElementById('lastReindex').textContent = idx.last_reindex_at || '-';
      }
    }

    async function indexAction(action) {
      const path = {
        reindex: '/internal/index/reindex',
        start: '/internal/index/start',
        stop: '/internal/index/stop',
        reset: '/internal/index/reset',
      }[action];
      if (!path) return;
      const res = await fetch(path, { method: 'POST' });
      const body = await res.text();
      addLog('[index] ' + action + ' => ' + body, res.ok ? 'info' : 'err');
      await refreshStatus();
    }

    document.querySelectorAll('[data-action]').forEach(btn => {
      btn.addEventListener('click', () => indexAction(btn.getAttribute('data-action')));
    });

    const es = new EventSource('/internal/logs/stream');
    es.onmessage = (ev) => {
      try {
        const payload = JSON.parse(ev.data);
        if (payload.type === 'log' && payload.event) {
          const e = payload.event;
          addLog('[' + e.timestamp + '] ' + e.message + ' ' + JSON.stringify(e.fields || {}), e.level);
        }
      } catch (_) {}
    };
    es.onerror = () => addLog('[sse] disconnected, browser will retry', 'warn');

    refreshStatus();
    setInterval(refreshStatus, 5000);
  </script>
</body>
</html>`
