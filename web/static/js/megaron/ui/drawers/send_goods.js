// Economy → Send goods: ONE form for sending goods between cities, whoever owns
// the destination. Transfer, gift and standing order are one control set; the
// source city always crews. Pure helpers first (tested), DOM glue last.
import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { serverNow } from '../../clock.js';
import { esc, byGoodName, formatApiError, playerError } from '../format.js';
import { fmtNum, fmtDays } from '../fmt_num.js';
import { fmtTickTime } from '../fmt_clock.js';
import { routeCargoHTML, renderStandingOrdersHTML } from './economy.js';

// Own cities first, then every known foreign city as "city — ruler".
export function destinationButtons(rows) {
  const list = (rows || []).map(d => ({ id: d.settlement_id, own: !!d.own,
    label: d.own ? d.name : `${d.name} — ${d.owner_name}` }));
  return [...list.filter(d => d.own), ...list.filter(d => !d.own)];
}

export function routeChoices(preview) {
  return (preview?.options || []).filter(o => o.available);
}

const selectedOption = s => routeChoices(s.preview).find(o => o.by === s.by) || null;

export function routeFacts(o) {
  return [`By ${o.by}`, o.arrives_at ? `arrives ${fmtTickTime(null, o.arrives_at)}` : '', fmtDays(o.travel_ticks)]
    .filter(Boolean).join(' · ');
}

const amountOf = (s, key) => { const n = Number(s.amounts[key]); return Number.isFinite(n) && n > 0 ? n : 0; };

export function formItems(s) {
  return s.stock.map(g => ({ good_key: g.key, quantity: amountOf(s, g.key) })).filter(i => i.quantity > 0);
}

// What the chosen route can still carry of one good (a ship has a hull limit).
export function allAmount(s, key) {
  const have = Math.floor(s.stock.find(g => g.key === key)?.amount || 0);
  const o = selectedOption(s);
  if (!o || !Number.isFinite(o.cargo_capacity)) return have;
  const others = s.stock.filter(g => g.key !== key).reduce((sum, g) => sum + amountOf(s, g.key), 0);
  return Math.max(0, Math.min(have, o.cargo_capacity - others));
}

function overload(s) {
  const o = selectedOption(s);
  if (!o || !Number.isFinite(o.cargo_capacity)) return null;
  const total = formItems(s).reduce((sum, i) => sum + i.quantity, 0);
  return total > o.cargo_capacity ? o : null;
}

export function transferBody({ to, items, by }) {
  return { destination_id: to, items, ...(by ? { by } : {}) };
}

// Source city crews; "Bring home" rows are the return cargo.
export function stockedBody({ fromSettlementId, toSettlementId, items, home, by }) {
  return { from_settlement_id: fromSettlementId, to_settlement_id: toSettlementId, crewed_by_settlement_id: fromSettlementId,
    outbound: items.map(i => ({ good_key: i.good_key, threshold: i.quantity })), return: home, ...(by ? { by } : {}) };
}

function overText(s) {
  const over = overload(s);
  return over ? `Too much: the ${over.ship_name || 'ship'} carries ${fmtNum(over.cargo_capacity)}` : '';
}
export const canSend = s => !!destOf(s) && !!selectedOption(s) && !overload(s) && formItems(s).length > 0;

const destOf = s => s.dests.find(d => d.id === s.to) || null;

export function sendFormHTML(s) {
  const dest = destOf(s), foreign = !!dest && !dest.own, keep = s.keep && !!dest && dest.own;
  const fromHTML = s.froms.length > 1
    ? '<div class="sg-group"><span class="sg-label">From</span>' + s.froms.map(f =>
        `<button type="button" class="btn-small sg-btn${f.id === s.from.id ? ' active' : ''}" data-act="from" data-id="${esc(f.id)}">${esc(f.name)}</button>`).join('') + '</div>'
    : `<div class="sg-line">From ${esc(s.from.name)}</div>`;
  const toHTML = '<div class="sg-group"><span class="sg-label">To</span>' + (s.dests.length
    ? s.dests.map(d => `<button type="button" class="btn-small sg-btn${d.id === s.to ? ' active' : ''}" data-act="to" data-id="${esc(d.id)}">${esc(d.label)}</button>`).join('')
    : '<span class="sg-line">No other city known yet</span>') + '</div>';
  const rows = s.stock.length ? s.stock.map(g => `<div class="sg-row">
      <span class="sg-good">${esc(g.name || g.key)}</span><span class="sg-have">${fmtNum(Math.floor(g.amount))} in store</span>
      <input type="number" inputmode="decimal" min="0" step="any" placeholder="0" aria-label="${esc(g.name || g.key)}" data-amt="${esc(g.key)}" value="${esc(s.amounts[g.key] ?? '')}">
      ${keep ? '' : `<button type="button" class="btn-small" data-act="all" data-good="${esc(g.key)}">All</button>`}
    </div>`).join('') : '<div class="sg-line">Nothing to send from this city</div>';
  let route = '';
  if (dest) {
    const ch = routeChoices(s.preview);
    if (!s.preview) route = '<div class="sg-line">Checking the route…</div>';
    else if (!ch.length) route = `<div class="sg-line">${esc((s.preview.options || []).map(x => x.reason).filter(Boolean)[0] || 'No route to this city')}</div>`;
    else if (ch.length === 1) route = `<div class="sg-line">${esc(routeFacts(ch[0]))}</div>`;
    else route = '<div class="sg-group">' + ch.map(x => `<button type="button" class="btn-small sg-btn${x.by === s.by ? ' active' : ''}" data-act="by" data-by="${esc(x.by)}">${esc(routeFacts(x))}</button>`).join('') + '</div>';
    route += `<div class="sg-line sg-over" id="sg-over">${esc(overText(s))}</div>`;
  }
  const giftLine = foreign ? '<div class="sg-line">A gift: nothing comes back.</div>' : '';
  const keepHTML = dest && dest.own ? `<label class="sg-keep"><input type="checkbox" data-act="keep"${s.keep ? ' checked' : ''}> Keep it stocked</label>` : '';
  const moreHTML = keep ? `<details class="sg-more" data-act="more"${s.homeOpen ? ' open' : ''}><summary>More</summary>
      <div class="sg-line">Bring home — leave this much there</div>
      ${s.homeStock.length ? s.homeStock.map(g => `<div class="sg-row"><span class="sg-good">${esc(g.name || g.key)}</span><span class="sg-have">${fmtNum(Math.floor(g.amount))} there</span>
        <input type="number" inputmode="decimal" min="0" step="any" placeholder="–" aria-label="${esc(g.name || g.key)} to leave" data-home="${esc(g.key)}" value="${esc(s.home[g.key] ?? '')}"></div>`).join('') : '<div class="sg-line">Nothing there to bring home</div>'}
    </details>` : '';
  const label = keep ? 'Keep it stocked →' : foreign ? 'Send as a gift →' : 'Send →';
  const can = canSend(s);
  return `<div class="dsec">${fromHTML}${toHTML}${giftLine}
    <div class="sg-rows">${rows}</div>${route}${keepHTML}${moreHTML}
    <button type="button" id="sg-send" class="btn-primary btn-small" data-act="send"${can ? '' : ' disabled'}>${label}</button>
    <div id="sg-result" class="action-result${s.result?.ok ? ' sg-ok' : ''}">${s.result ? esc(s.result.text) : ''}</div>
    <div id="sg-fresh">${s.result?.rowHTML || ''}</div></div>`;
}

// ── DOM glue ────────────────────────────────────────────────────────────────
let S = null, seq = 0;
const host = () => document.getElementById('ectab-send');
const base = () => `/api/v1/worlds/${State.WORLD_ID}`;

function render() {
  const el = host();
  if (el && S) el.innerHTML = sendFormHTML(S);
}
// Typing must not rebuild the form (focus): update only the two live parts.
function refreshSend() {
  const el = host();
  if (!el || !S) return;
  const btn = el.querySelector('#sg-send'), over = el.querySelector('#sg-over');
  if (btn) btn.disabled = !canSend(S);
  if (over) over.textContent = overText(S);
}

async function loadFrom() {
  const mine = ++seq, id = S.from.id;
  S.stock = []; S.amounts = {}; S.dests = []; S.to = null; S.preview = null; S.by = ''; S.result = null;
  render();
  const [g, d] = await Promise.all([
    fetchAuth(`${base()}/provinces/${id}/goods`).then(r => r.ok ? r.json() : []).catch(() => []),
    fetchAuth(`${base()}/provinces/${id}/trade/destinations`).then(r => r.ok ? r.json() : []).catch(() => []),
  ]);
  if (mine !== seq || !S) return;
  S.stock = (g || []).filter(x => (x.amount || 0) > 0 && x.shippable === true).sort(byGoodName);
  S.dests = destinationButtons(d || []);
  const want = S.presetTo && S.dests.find(x => x.id === S.presetTo);
  S.presetTo = null;
  render();
  if (want) await pickTo(want.id);
}

async function pickTo(id) {
  S.to = id; S.preview = null; S.by = ''; S.result = null; S.homeStock = []; S.home = {};
  if (!destOf(S)?.own) S.keep = false;
  render();
  const mine = ++seq, from = S.from.id;
  const pv = fetchAuth(`${base()}/provinces/${from}/trade/preview?destination_id=${encodeURIComponent(id)}`)
    .then(async r => ({ ok: r.ok, status: r.status, data: await r.json().catch(() => ({})) })).catch(() => ({ ok: false, data: {} }));
  const [res] = await Promise.all([pv, S.keep ? loadHome() : null]);
  if (mine !== seq || !S || S.to !== id) return;
  S.preview = res.ok ? res.data : { options: [{ by: 'land', available: false, reason: playerError(res, res.data, 'Could not check the route') }] };
  const ch = routeChoices(S.preview);
  S.by = ch.find(o => o.by === res.data.selected_by)?.by || ch[0]?.by || '';
  render();
}

async function loadHome() {
  const prov = State.provinceData.find(p => p.own && p.settlement_id === S.to);
  if (!prov) return;
  const to = S.to;
  const goods = await fetchAuth(`${base()}/provinces/${prov.id}/goods`).then(r => r.ok ? r.json() : []).catch(() => []);
  if (S && S.to === to) S.homeStock = (goods || []).filter(g => (g.amount || 0) > 0 && g.shippable === true).sort(byGoodName);
}

async function send() {
  const dest = destOf(S), items = formItems(S), by = S.by;
  const keep = S.keep && dest.own;
  const cargo = items.map(i => `${fmtNum(i.quantity)} ${i.good_key}`).join(', ');
  const fail = (text) => { S.result = { ok: false, text }; render(); };
  let r, d;
  if (keep) {
    const home = S.homeStock.map(g => ({ good_key: g.key, floor: Number(S.home[g.key]) }))
      .filter(h => S.home[h.good_key] !== undefined && S.home[h.good_key] !== '' && Number.isFinite(h.floor) && h.floor >= 0);
    r = await fetchAuth(`${base()}/standing-orders`, { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(stockedBody({ fromSettlementId: S.from.settlementId, toSettlementId: S.to, items, home, by })) });
  } else {
    r = await fetchAuth(`${base()}/provinces/${S.from.id}/trade`, { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(transferBody({ to: S.to, items, by })) });
  }
  d = await r.json().catch(() => ({}));
  if (!S) return;
  if (!r.ok) return fail(formatApiError(d, keep ? 'Could not create the standing order.' : 'Transfer failed.'));
  let text;
  if (keep) text = `Standing order created: ${cargo} kept at ${dest.label}.`;
  else {
    text = d.ship_name ? `${cargo} sent aboard ${d.ship_name} — can be intercepted en route, and sails home empty afterward.`
      : `${cargo} sent — can be intercepted en route.`;
    if (d.kind === 'gift') text = 'Gift: ' + text;
    if (Number.isInteger(d.arrival_tick)) text += ` Arrives ${fmtTickTime(d.route_version === 2 ? null : d.arrival_tick, d.arrives_at)} (journey: ${fmtDays(d.travel_ticks)}).`;
  }
  S.result = { ok: true, text, rowHTML: '' };
  S.amounts = {};
  loadFrom_keepResult(keep ? d.id : null, keep ? null : d.transport_id);
}

// Refresh stock after a send, keep the chosen destination and show the new row.
async function loadFrom_keepResult(orderId, transportId) {
  const result = S.result, to = S.to, by = S.by, preview = S.preview, keepFlag = S.keep, home = S.homeStock;
  const id = S.from.id;
  const g = await fetchAuth(`${base()}/provinces/${id}/goods`).then(r => r.ok ? r.json() : []).catch(() => []);
  let rowHTML = '';
  try {
    const [orders, trades] = await Promise.all([
      orderId ? fetchAuth(`${base()}/standing-orders`).then(r => r.ok ? r.json() : []) : [],
      fetchAuth(`${base()}/trades`).then(r => r.ok ? r.json() : null),
    ]);
    const mine = (trades || []).filter(t => t.role === 'sender' || t.role === 'recipient');
    if (orderId) rowHTML = renderStandingOrdersHTML((orders || []).filter(o => o.id === orderId), mine, serverNow());
    else if (transportId) rowHTML = routeCargoHTML(mine.filter(t => t.id === transportId), serverNow());
  } catch (_) { /* the result line stands alone */ }
  if (!S || S.to !== to) return;
  S.stock = (g || []).filter(x => (x.amount || 0) > 0 && x.shippable === true).sort(byGoodName);
  S.result = { ...result, rowHTML }; S.by = by; S.preview = preview; S.keep = keepFlag; S.homeStock = home;
  render();
}

function bind(el) {
  if (el._sgBound) return;
  el._sgBound = true;
  el.addEventListener('click', e => {
    const t = e.target.closest?.('[data-act]');
    if (!t || !S) return;
    const act = t.dataset.act;
    if (act === 'from') { S.from = S.froms.find(f => f.id === t.dataset.id) || S.from; loadFrom(); }
    else if (act === 'to') pickTo(t.dataset.id);
    else if (act === 'by') { S.by = t.dataset.by; render(); }
    else if (act === 'all') { S.amounts[t.dataset.good] = String(allAmount(S, t.dataset.good)); render(); }
    else if (act === 'send') send();
  });
  el.addEventListener('change', async e => {
    if (!S || e.target.dataset?.act !== 'keep') return;
    S.keep = e.target.checked;
    if (S.keep) await loadHome();
    render();
  });
  el.addEventListener('input', e => {
    if (!S) return;
    const a = e.target.dataset?.amt, h = e.target.dataset?.home;
    if (a !== undefined) { S.amounts[a] = e.target.value; refreshSend(); }
    else if (h !== undefined) S.home[h] = e.target.value;
  });
  el.addEventListener('toggle', e => { if (S && e.target.dataset?.act === 'more') S.homeOpen = e.target.open; }, true);
}

// cities: the player's own provinces (State.provinceData rows); presetTo: a destination settlement id.
export async function loadSendGoods(cities, presetTo = null) {
  const el = host();
  if (!el) return;
  if (!cities.length) { el.innerHTML = '<p class="empty-state" style="padding:1rem">Found a city before sending goods.</p>'; return; }
  const froms = [...cities].sort((a, b) => (b.is_capital ? 1 : 0) - (a.is_capital ? 1 : 0))
    .map(c => ({ id: c.id, settlementId: c.settlement_id || c.id, name: c.name }));
  const keepFrom = S && S.world === State.WORLD_ID && froms.find(f => f.id === S.from?.id);
  S = { world: State.WORLD_ID, froms, from: keepFrom || froms[0], presetTo, dests: [], to: null, stock: [], amounts: {}, preview: null, by: '',
    keep: false, homeOpen: false, homeStock: [], home: {}, result: null };
  bind(el);
  await loadFrom();
}
