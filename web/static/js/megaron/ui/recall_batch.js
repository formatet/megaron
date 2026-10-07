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
  return `<div class="dsec"><div class="dsec-title">Recall all matching units</div>
    <p class="empty-state">Send every matching marching unit home. Leave both filters blank to recall them all, or narrow the order by destination or task. A Runner carries the order: units turn home when it reaches them. Units meeting in the same place on the same game day can share a Runner.</p>
    <div class="field"><label for="recall-batch-target">Current destination (optional)</label><input id="recall-batch-target" placeholder="q,r"></div>
    <div class="field"><label for="recall-batch-intent">Current task (optional)</label><input id="recall-batch-intent" placeholder="e.g. explore"></div>
    <button class="btn-small btn-primary" id="recall-batch-send" onclick="unitRecallAll()">Recall all matching units</button>
    <div id="recall-batch-result" role="status" aria-live="polite"></div></div>`;
}

// Player-facing counts use words; map coordinates and game-day indices stay
// numeric because the player uses those values to find the place and time.
function unitCount(count) {
  if (count < 20) return ['zero','one','two','three','four','five','six','seven','eight','nine','ten','eleven','twelve','thirteen','fourteen','fifteen','sixteen','seventeen','eighteen','nineteen'][count];
  if (count < 100) {
    const tens = ['','','twenty','thirty','forty','fifty','sixty','seventy','eighty','ninety'];
    return tens[Math.floor(count / 10)] + (count % 10 ? ' ' + unitCount(count % 10) : '');
  }
  if (count < 1000) return unitCount(Math.floor(count / 100)) + ' hundred' + (count % 100 ? ' and ' + unitCount(count % 100) : '');
  return unitCount(Math.floor(count / 1000)) + ' thousand' + (count % 1000 ? ' ' + unitCount(count % 1000) : '');
}

export function recallBatchReportHTML(report) {
  if (!report.results?.length) return '<p class="empty-state">No units match these filters.</p>';
  const groups = (report.groups || []).map(g => {
    const single = g.unit_ids.length === 1;
    const eta = g.passage_status === 'awaiting_passage' ? 'waiting for a ship'
      : g.passage_status === 'aboard' ? 'aboard a ship'
      : g.passage_status === 'returning_sealed' ? 'coming back to port after its ship was lost'
      : g.passage_status ? esc(g.passage_status) : 'delivery on game day ' + esc(String(g.courier_due_tick));
    return `<li><strong>${single ? 'Single unit' : 'Group of ' + unitCount(g.unit_ids.length) + ' units'}</strong>: ${single ? 'one unit receives its own Runner' : 'these units share a Runner'} at (${esc(String(g.q))},${esc(String(g.r))}) — ${eta}. ${single ? 'It turns' : 'They turn'} home when the order arrives.</li>`;
  }).join('');
  const results = report.results.map(r => {
    const outcome = r.status === 'order_dispatched' ? 'recall sent by Runner.'
      : r.status === 'order_applied' ? 'recall given in person.' : 'rejected — ' + (r.reason || 'Recall failed');
    return '<li><strong>' + esc(r.name || r.unit_id) + '</strong>: ' + esc(outcome) + '</li>';
  }).join('');
  return (groups ? '<div class="incoming-header">Runners carrying the orders</div><ul class="march-list">' + groups + '</ul>' : '')
    + '<div class="incoming-header">Each unit’s order</div><ul class="march-list">' + results + '</ul>';
}

export async function sendRecallBatch(fetcher, worldID, body) {
  const response = await fetcher(`/api/v1/worlds/${worldID}/units/recall`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  });
  const report = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(report.error || 'Recall failed');
  return report;
}
