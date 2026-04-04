class LocalMetrics {
  constructor(context) {
    this.context = context;
    this.key = 'copilotLocal.metrics';
    this.data = context.globalState.get(this.key, {
      total_requests: 0,
      total_tokens_approx: 0,
      requests_per_day: {},
      avg_response_time_ms: 0,
      avg_time_to_first_token_ms: 0,
      errors_count: 0,
      started_at_ms: Date.now(),
      last_request_at_ms: 0,
    });
    if (!this.data.started_at_ms) this.data.started_at_ms = Date.now();
  }

  _todayKey() {
    return new Date().toISOString().slice(0, 10);
  }

  _persist() {
    return this.context.globalState.update(this.key, this.data);
  }

  async trackRequest(totalTimeMs, chars, timeToFirstTokenMs, hadError) {
    const prev = this.data.total_requests;
    this.data.total_requests += 1;
    this.data.total_tokens_approx += Math.ceil((chars || 0) / 4);
    const today = this._todayKey();
    this.data.requests_per_day[today] = (this.data.requests_per_day[today] || 0) + 1;

    const n = this.data.total_requests;
    const safeTotal = Math.max(0, totalTimeMs || 0);
    const safeFirst = Math.max(0, timeToFirstTokenMs || 0);
    this.data.avg_response_time_ms = ((this.data.avg_response_time_ms * prev) + safeTotal) / n;
    this.data.avg_time_to_first_token_ms = ((this.data.avg_time_to_first_token_ms * prev) + safeFirst) / n;

    if (hadError) {
      this.data.errors_count += 1;
    }
    this.data.last_request_at_ms = Date.now();

    await this._persist();
  }

  getSnapshot() {
    return { ...this.data, requests_per_day: { ...this.data.requests_per_day } };
  }

  tokensPerSecondApprox() {
    const snap = this.getSnapshot();
    const started = Number(snap.started_at_ms || Date.now());
    const elapsedSec = Math.max(1, (Date.now() - started) / 1000);
    return Number(snap.total_tokens_approx || 0) / elapsedSec;
  }

  graphLastDays(days = 7) {
    const now = new Date();
    const lines = [];
    for (let i = days - 1; i >= 0; i--) {
      const d = new Date(now);
      d.setDate(now.getDate() - i);
      const key = d.toISOString().slice(0, 10);
      const v = this.data.requests_per_day[key] || 0;
      lines.push(key + ' | ' + '#'.repeat(Math.min(v, 40)) + ' (' + v + ')');
    }
    return lines.join('\n');
  }

  summaryText() {
    const snap = this.getSnapshot();
    const today = this._todayKey();
    let week = 0;
    const now = new Date();
    for (let i = 0; i < 7; i++) {
      const d = new Date(now);
      d.setDate(now.getDate() - i);
      const key = d.toISOString().slice(0, 10);
      week += snap.requests_per_day[key] || 0;
    }

    return [
      'Requests hoy: ' + (snap.requests_per_day[today] || 0),
      'Requests semana: ' + week,
      'Requests total: ' + snap.total_requests,
      'Tiempo promedio respuesta (ms): ' + Math.round(snap.avg_response_time_ms),
      'Tiempo promedio primer token (ms): ' + Math.round(snap.avg_time_to_first_token_ms),
      'Tokens aproximados: ' + snap.total_tokens_approx,
      'Errores: ' + snap.errors_count,
      '',
      'Requests por dia (ultimos 7):',
      this.graphLastDays(7),
    ].join('\n');
  }
}

module.exports = { LocalMetrics };
