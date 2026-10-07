import { esc } from './format.js';

export function recallBatchBody(target = '', intent = '') {
  const body = { all: true };
  if (target.trim()) {
    const parts = target.trim().split(',');
    if (parts.length !== 2 || parts.some(p => !/^-?\d+$/.test(p.trim()))) {
      throw new Error('Destination must be q,r.');
    }
    [body.target_q, body.target_r] = parts.map(Number);
    if (![body.target_q, body.target_r].every(Number.isSafeInteger)) throw new Error('Destination must use whole coordinates.');
  }
  if (intent.trim()) body.intent = intent.trim();
  return body;
}

export function recallBatchControlsHTML() {
  return `<div class="dsec"><div class="dsec-title">Recall units</div>
    <p>Send all selected units home. Units sharing a meeting place and game day share a Runner; other units get their own. Every refusal is listed.</p>
    <label>Destination <input id="recall-batch-target" placeholder="q,r (optional)"></label>
    <label>Intent <input id="recall-batch-intent" placeholder="e.g. explore (optional)"></label>
    <button id="recall-batch-send" onclick="unitRecallAll()">Recall all matching units</button>
    <div id="recall-batch-result" role="status" aria-live="polite"></div></div>`;
}

export function recallBatchReportHTML(report) {
  if (!report.results?.length) return '<p>No units match the recall selection.</p>';
  const groups = (report.groups || []).map(g => {
    const label = g.unit_ids.length === 1 ? 'Single' : 'Group';
    const eta = g.passage_status ? esc(g.passage_status) : 'delivery on game day ' + esc(String(g.courier_due_tick));
    return `<li>${label}: Runner for ${g.unit_ids.length} unit(s) at (${esc(String(g.q))},${esc(String(g.r))}) — ${eta}; they turn home on delivery.</li>`;
  }).join('');
  const results = report.results.map(r => {
    const outcome = r.status === 'order_dispatched' ? 'recall sent by Runner.'
      : r.status === 'order_applied' ? 'recall given in person.' : 'rejected — ' + (r.reason || 'Recall failed');
    return '<li>' + esc(r.name || r.unit_id) + ': ' + esc(outcome) + '</li>';
  }).join('');
  return (groups ? '<ul>' + groups + '</ul>' : '') + '<ul>' + results + '</ul>';
}

export async function sendRecallBatch(fetcher, worldID, body) {
  const response = await fetcher(`/api/v1/worlds/${worldID}/units/recall`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  });
  const report = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(report.error || 'Recall failed');
  return report;
}
