import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { serverNow } from '../../clock.js';
import { esc, fmtSilver } from '../format.js';
import { renderLockedActions } from '../misc.js';
import { sitosGranaryState } from './sitos_view.js';

// A good at its storage ceiling is a silent off-switch: everything produced
// past the cap is discarded, so labour sitting on it earns nothing. Mirror
// keryx's atStorageCeiling (cmd_goods.go) exactly — >=99% of cap, since a
// delivery can land a hair over cap and 99% is full within the hour anyway.
export function atStorageCeiling(amount, cap) {
  return cap > 0 && amount >= cap * 0.99;
}

// The Rate cell for one goods-mini row. Normally green + growth, but a good at
// its storage ceiling reads "full" (dimmed via .goods-atcap) instead — a
// positive rate there looks like growth when every unit past the cap is being
// discarded (the spill the storage-ceiling decision makes visible; keryx shows
// the same via its `*` footnote). The wasted rate rides along so the player
// sees exactly how much labour is earning nothing.
export function goodsRateCell(g) {
  const rate = g.rate_per_tick || 0;
  if (atStorageCeiling(g.amount || 0, g.cap || 0)) {
    const lost = rate > 0 ? ` · +${rate.toFixed(1)} lost` : '';
    const title = `Storage full (${Math.floor(g.amount || 0)}/${Math.floor(g.cap || 0)}). `
      + 'Everything produced past the cap is discarded — move this labour or trade it away.';
    return `<td class="goods-atcap" title="${esc(title)}">full${lost}</td>`;
  }
  return rate > 0 ? `<td style="color:var(--safe)">+${rate.toFixed(1)}/tick</td>` : '<td></td>';
}

// ── Settlements overview (S1, megaron_plan_stad_vs_ekonomi.md §3) ──────────
// Answers "vilken av mina städer?" for food — the one question this table
// exists for (§1: "Ekonomi äger jämförelsen"). `sitosGranaryState` is the
// exact same pure function city.js's per-settlement Tillstånd row (and
// keryx's `status`) derive their five states from — reused here, not
// re-derived, so this table and the city drawer can never disagree about
// which state a settlement is in.
const FOOD_RANK = { 'empty-shrinking': 0, 'release': 1, 'empty-growing': 2, 'rest': 3, 'store': 4 };
const FOOD_LABEL = {
  'empty-shrinking': 'starving',
  'release': 'drawing down reserve',
  'empty-growing': 'empty, refilling',
  'rest': 'stable',
  'store': 'surplus',
};

// One settlement's food-survival summary. `pd` is the settlement object from
// GET .../provinces/{id} (the same fields city.js already reads: population,
// grain_prod_rate/grain_consum_rate, food_self_sufficient, sitos.*) — no new
// server data (plan §4: "Ingen ny data på servern"). `pd` may be null if the
// fetch failed; that renders as an honest "no data" row rather than a guess.
export function settlementFoodRow(s, pd) {
  if (!pd) {
    return { id: s.id, name: s.name, isCapital: !!s.is_capital, population: 0,
      grainRate: 0, coverage: 0, severity: 'rest', label: 'no data', rank: 5 };
  }
  const netTick = (pd.grain_prod_rate || 0) - (pd.grain_consum_rate || 0);
  const sitos = pd.sitos || {};
  const cov   = sitos.coverage_ticks    || 0;
  const low   = sitos.low_ticks         || 0;
  const high  = sitos.high_ticks        || 0;
  const total = sitos.granary_total     || 0;
  const net   = sitos.food_net_per_tick != null ? sitos.food_net_per_tick : netTick;
  // food_self_sufficient===false means the catchment cannot feed the
  // population even with every citizen on food — worse than any coverage
  // number can say (it will never recover on its own), so it overrides the
  // granary state and always sorts to the very top.
  const cannotFeed = pd.food_self_sufficient === false;
  const { severity } = sitosGranaryState(cov, low, high, total, net);
  return {
    id: s.id,
    name: s.name,
    isCapital: !!s.is_capital,
    population: pd.population || 0,
    grainRate: netTick,
    coverage: cov,
    severity: cannotFeed ? 'empty-shrinking' : severity,
    label: cannotFeed ? 'cannot feed itself' : (FOOD_LABEL[severity] || severity),
    rank: cannotFeed ? -1 : (FOOD_RANK[severity] ?? 3),
  };
}

const SETTLEMENT_SORT_KEYS = {
  name:       r => r.name.toLowerCase(),
  population: r => r.population,
  grainRate:  r => r.grainRate,
  coverage:   r => r.coverage,
  brist:      r => r.rank,
};

// Sortable on every column, but the table exists to answer one question —
// so the caller (loadEconomyGoods) defaults to key='brist', dir='asc'
// (most urgent first). Pure (array in, new sorted array out) for testing.
export function sortSettlementRows(rows, key, dir) {
  const getter = SETTLEMENT_SORT_KEYS[key] || SETTLEMENT_SORT_KEYS.brist;
  const sign = dir === 'desc' ? -1 : 1;
  return [...rows].sort((a, b) => {
    const av = getter(a), bv = getter(b);
    if (av < bv) return -1 * sign;
    if (av > bv) return 1 * sign;
    return 0;
  });
}

// renderSettlementsOverviewHTML builds the sortable comparison table —
// clicking a header re-sorts (sortEconomySettlements), clicking a row opens
// that settlement's City drawer (openCitySettlement) rather than exposing
// any per-settlement control here (plan §1 point 3: Economy only links to
// Stad, it never edits a single settlement).
export function renderSettlementsOverviewHTML(rows, sortKey, sortDir) {
  if (!rows.length) return '';
  const arrow = key => key === sortKey ? (sortDir === 'desc' ? ' ▼' : ' ▲') : '';
  const th = (key, label, align) =>
    `<td onclick="sortEconomySettlements('${key}')" style="cursor:pointer${align ? `;text-align:${align}` : ''}">${esc(label)}${arrow(key)}</td>`;
  return `<table class="goods-mini">
    <tr style="color:var(--text-dim);font-size:.7rem">
      ${th('name', 'Settlement')}
      ${th('population', 'Pop', 'right')}
      ${th('grainRate', 'Grain', 'right')}
      ${th('coverage', 'Coverage', 'right')}
      ${th('brist', 'Status')}
    </tr>
    ${rows.map(r => {
      const rateColor = r.grainRate < 0 ? 'var(--danger)' : (r.grainRate > 0 ? 'var(--safe)' : 'var(--text-dim)');
      const rateSign = r.grainRate > 0 ? '+' : '';
      return `<tr onclick="openCitySettlement('${r.id}')" style="cursor:pointer">
        <td>${esc(r.name)}${r.isCapital ? ' ★' : ''}</td>
        <td style="text-align:right">${Math.floor(r.population)}</td>
        <td style="text-align:right;color:${rateColor}">${rateSign}${r.grainRate.toFixed(1)}/tick</td>
        <td style="text-align:right">${r.coverage.toFixed(1)} ticks</td>
        <td><span class="sitos-state-${r.severity}">${esc(r.label)}</span></td>
      </tr>`;
    }).join('')}
  </table>`;
}

// Sort state + row cache survive only for the current Goods tab render — a
// fresh loadEconomyGoods() call (tab switch, drawer reopen) resets both, same
// lifetime as the rest of the tab's DOM.
let _settlementRows = [];
let _settlementSort = { key: 'brist', dir: 'asc' };

export function sortEconomySettlements(key) {
  _settlementSort = _settlementSort.key === key
    ? { key, dir: _settlementSort.dir === 'asc' ? 'desc' : 'asc' }
    : { key, dir: 'asc' };
  const el = document.getElementById('ec-settlements-overview');
  if (!el) return;
  el.innerHTML = renderSettlementsOverviewHTML(
    sortSettlementRows(_settlementRows, _settlementSort.key, _settlementSort.dir),
    _settlementSort.key, _settlementSort.dir);
}

// Opens the City drawer on a specific settlement — the only way Economy ever
// reaches a single settlement's controls (plan §1 point 3). Same pattern
// render/map.js already uses when a settlement marker is clicked on the map.
export function openCitySettlement(provinceId) {
  State.cityViewID = provinceId;
  window.openDrawer('city');
}

// ── Economy drawer ────────────────────────────────────────────────────────
export async function loadEconomyDrawer() {
  const body = document.getElementById('economy-body');
  const mySettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
  if (!mySettlements.length) { body.innerHTML = '<p class="empty-state" style="padding:1rem">No settlements.</p>'; return; }

  body.innerHTML =
    '<div class="drawer-tabs">' +
      '<button class="dtab active" data-tab="goods">Goods</button>' +
      '<button class="dtab" data-tab="transfer">Transfer</button>' +
      '<button class="dtab" data-tab="automation">Automation</button>' +
      '<button class="dtab" data-tab="wants">Wants</button>' +
    '</div>' +
    '<div id="ectab-goods" class="city-tab"></div>' +
    '<div id="ectab-transfer" class="city-tab" style="display:none"></div>' +
    '<div id="ectab-automation" class="city-tab" style="display:none"></div>' +
    '<div id="ectab-wants" class="city-tab" style="display:none"></div>';

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('ectab-' + this.dataset.tab);
      if (el) el.style.display = '';
      if (this.dataset.tab === 'goods') loadEconomyGoods(mySettlements);
      else if (this.dataset.tab === 'transfer') loadEconomyTransfer(mySettlements);
      else if (this.dataset.tab === 'automation') loadEconomyAutomation(mySettlements);
      else if (this.dataset.tab === 'wants') loadEconomyWants();
    });
  });

  loadEconomyGoods(mySettlements);
}

async function loadEconomyGoods(mySettlements) {
  const el = document.getElementById('ectab-goods');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const [goodsResults, provResults] = await Promise.all([
      Promise.all(mySettlements.map(s => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${s.id}/goods`))),
      // The list endpoint (/provinces) carries only size_tier for a settlement,
      // never exact population or grain rates — the per-settlement detail
      // (already fetched by city.js) is where those live, so the overview
      // fetches it too rather than inventing a new endpoint.
      Promise.all(mySettlements.map(s => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${s.id}`))),
    ]);

    _settlementRows = await Promise.all(mySettlements.map(async (s, i) => {
      const pr = provResults[i];
      const pd = pr.ok ? (await pr.json()).settlement : null;
      return settlementFoodRow(s, pd);
    }));
    const overviewHtml =
      `<div class="dsec-title">Settlements</div>` +
      `<div id="ec-settlements-overview">${renderSettlementsOverviewHTML(
        sortSettlementRows(_settlementRows, _settlementSort.key, _settlementSort.dir),
        _settlementSort.key, _settlementSort.dir)}</div>`;

    let html = '';
    for (let i = 0; i < mySettlements.length; i++) {
      const s = mySettlements[i];
      const r = goodsResults[i];
      if (!r.ok) continue;
      const goods = await r.json();
      const silver = goods.find(g => g.key === 'silver');
      const others = goods.filter(g => g.key !== 'silver' && (g.amount > 0 || g.producible));
      html += `<div class="dsec-title" style="margin-top:.8rem">${s.name}${s.is_capital?' ★':''}</div>`;
      if (silver) {
        html += `<div class="silver-balance">
          <div>
            <div class="sb-label">Silver</div>
            <div class="sb-val">${fmtSilver(silver.amount)}</div>
          </div>
          ${silver.rate_per_tick ? `<div class="sb-rate">+${silver.rate_per_tick.toFixed(1)}/tick</div>` : ''}
        </div>`;
      }
      if (others.length) {
        html += `<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem"><td>Good</td><td>Amount</td><td>Rate</td></tr>${others.map(g =>
          `<tr><td>${g.name||g.key}</td><td>${Math.floor(g.amount||0)}</td>${goodsRateCell(g)}</tr>`
        ).join('')}</table>`;
      }
    }
    el.innerHTML = overviewHtml + (html || '<p class="empty-state" style="padding:1rem">No goods data.</p>') + await renderLockedActions('trade');
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load goods.</p>';
  }
}

async function loadEconomyTransfer(mySettlements) {
  const el = document.getElementById('ectab-transfer');
  if (mySettlements.length < 2) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Need at least two of your own settlements to transfer between.</p>';
    return;
  }
  // From is addressed by PROVINCE id (the /provinces/{id}/trade URL); To is the
  // DESTINATION SETTLEMENT id (the handler resolves the destination by settlement
  // id — sending a province id here was the "destination settlement not found" bug).
  const fromOpts = mySettlements.map(s => `<option value="${s.id}">${esc(s.name)}${s.is_capital?' ★':''}</option>`).join('');
  const toOpts   = mySettlements.map(s => `<option value="${s.settlement_id||s.id}">${esc(s.name)}${s.is_capital?' ★':''}</option>`).join('');
  const inputStyle = 'width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.2rem .3rem';
  el.innerHTML = `
    <div class="dsec">
      <div class="dsec-title">Internal transfer</div>
      <div style="display:flex;flex-direction:column;gap:.35rem;font-size:.78rem">
        <label>From <select id="ec-tr-from" onchange="loadTransferGoods(this.value)" style="${inputStyle}">${fromOpts}</select></label>
        <label>To <select id="ec-tr-to" style="${inputStyle}">${toOpts}</select></label>
        <label>Good <select id="ec-tr-good" style="${inputStyle}"><option value="">Loading…</option></select></label>
        <label>Quantity <input type="number" id="ec-tr-qty" min="1" style="${inputStyle}"></label>
        <button class="btn-primary btn-small" onclick="startTransfer()">Transfer →</button>
        <div id="ec-tr-result" class="action-result"></div>
      </div>
    </div>
    <div class="dsec">
      <div class="dsec-title">Your cargo in transit</div>
      <div id="ec-tr-cargo" class="loading" style="padding:.4rem 0">Loading…</div>
    </div>`;
  const toSel = document.getElementById('ec-tr-to');
  if (toSel && mySettlements.length > 1) toSel.selectedIndex = 1;
  loadTransferGoods(document.getElementById('ec-tr-from').value);
  refreshCargoInTransit();
}

// formatCargoRows is the pure part of the "your cargo in transit" list — no
// DOM, no fetch, no Date.now()/serverNow() read internally. `nowMs` is passed
// in explicitly (the caller supplies serverNow()) so this can be unit-tested
// the same way render/camera.test.mjs tests zoomStep: canned inputs in,
// checked output out, no stubbing required. Each `trade` is one row of
// GET /trades already filtered to the caller's own by the caller.
export function formatCargoRows(trades, nowMs) {
  return trades.map(t => {
    const etaMs = new Date(t.arrives_at).getTime() - nowMs;
    let eta;
    if (etaMs <= 0) eta = 'arrived';
    else if (etaMs < 3600000) eta = `${Math.floor(etaMs / 60000)}m`;
    else eta = `${Math.floor(etaMs / 3600000)}h ${Math.floor((etaMs % 3600000) / 60000)}m`;
    return {
      good: t.good_key || '?',
      qty: Math.floor(t.quantity || 0),
      from: `(${t.origin_q},${t.origin_r})`,
      to: `(${t.dest_q},${t.dest_r})`,
      eta,
      direction: t.role === 'recipient' ? 'incoming' : 'outgoing',
    };
  });
}

// renderCargoHTML builds the markup for formatCargoRows' output. Kept pure
// (string in, string out) for the same testability reason as above — no
// document access. Reuses the existing goods-mini/empty-state classes
// (megaron.css is owned elsewhere this round; no new classes here).
export function renderCargoHTML(trades, nowMs) {
  if (!trades.length) {
    return '<p class="empty-state" style="padding:.4rem 0">Nothing of yours in transit.</p>';
  }
  const rows = formatCargoRows(trades, nowMs);
  return '<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem">' +
    '<td>Good</td><td>Qty</td><td>From → To</td><td>Dir</td><td style="text-align:right">ETA</td></tr>' +
    rows.map(row =>
      `<tr><td>${esc(row.good)}</td><td>${row.qty}</td><td>${esc(row.from)} → ${esc(row.to)}</td>` +
      `<td>${esc(row.direction)}</td><td style="text-align:right">${esc(row.eta)}</td></tr>`
    ).join('') +
    '</table>' +
    '<p style="color:var(--text-dim);font-size:.68rem;margin-top:.3rem">' +
    'Physical cargo — it can be intercepted and seized while in transit.</p>';
}

// Fetches GET /trades and renders the in-transit cargo the caller is party to.
// Filters on `role` rather than `mine` (2026-08-24): `mine` marks only the side
// that DISPATCHED a shipment, so a Wanax who accepted a trade offer and paid
// escrow saw an empty list while the goods they had bought crossed the map.
// Called on tab load and again right after a successful transfer, so a freshly
// dispatched caravan appears without having to leave and reopen the tab.
async function refreshCargoInTransit() {
  const el = document.getElementById('ec-tr-cargo');
  if (!el) return;
  el.innerHTML = '<div class="loading" style="padding:.4rem 0">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>'; return; }
    const trades = (await r.json()) || [];
    el.innerHTML = renderCargoHTML(
      trades.filter(t => t.role === 'sender' || t.role === 'recipient'), serverNow());
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>';
  }
}

// Populate the Good dropdown with the From settlement's goods in stock, so the
// player picks a real good and sees how much is available (no more free-text).
export async function loadTransferGoods(fromProvId) {
  const sel = document.getElementById('ec-tr-good');
  if (!sel || !fromProvId) return;
  sel.innerHTML = '<option value="">Loading…</option>';
  const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${fromProvId}/goods`);
  if (!r.ok) { sel.innerHTML = '<option value="">Could not load goods</option>'; return; }
  const goods = ((await r.json()) || []).filter(g => (g.amount || 0) > 0);
  sel.innerHTML = goods.length
    ? goods.map(g => `<option value="${g.key}">${esc(g.name || g.key)} — ${Math.floor(g.amount || 0)} in stock</option>`).join('')
    : '<option value="">No goods in stock</option>';
}

export async function startTransfer() {
  const from = document.getElementById('ec-tr-from')?.value;
  const to = document.getElementById('ec-tr-to')?.value;
  const good = document.getElementById('ec-tr-good')?.value.trim();
  const qty = parseFloat(document.getElementById('ec-tr-qty')?.value || '0');
  const resultEl = document.getElementById('ec-tr-result');
  if (!resultEl) return;
  if (!from || !to || from === to) { resultEl.style.color = 'var(--accent)'; resultEl.textContent = 'Pick two different settlements.'; return; }
  if (!good || qty <= 0) { resultEl.style.color = 'var(--accent)'; resultEl.textContent = 'Good and quantity required.'; return; }
  resultEl.textContent = '';
  const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${from}/trade`, {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ destination_id: to, good_key: good, quantity: qty }),
  });
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    resultEl.style.color = 'var(--safe)';
    resultEl.textContent = `${qty} ${good} sent — physical cargo, can be intercepted en route.`;
    refreshCargoInTransit();
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Transfer failed.';
  }
}

// ── Automation tab (standing orders) ────────────────────────────────────────
// megaron_plan_staende_leverans.md: a caravan that keeps a destination's
// stock topped up on its own (PULL — "hold grain at Colony ≥ 200" — not a
// fixed quantity/interval). Home per megaron_plan_stad_vs_ekonomi.md §1: a
// flow between two settlements belongs to neither city's own drawer.

// parseGoodAmountPairs parses "grain:200,fish:50" into [{good_key,amount}] —
// same shape and separator as keryx's --out/--home flags (cmd_route.go), so
// a Wanax moving between the two surfaces sees one format, not two.
export function parseGoodAmountPairs(spec) {
  if (!spec || !spec.trim()) return [];
  return spec.split(',').map(part => {
    const [good, amt] = part.split(':').map(s => (s || '').trim());
    return { good_key: good, amount: parseFloat(amt) };
  }).filter(p => p.good_key && !isNaN(p.amount));
}

async function loadEconomyAutomation(mySettlements) {
  const el = document.getElementById('ectab-automation');
  if (mySettlements.length < 2) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Need at least two of your own settlements for a standing order.</p>';
    return;
  }
  const inputStyle = 'width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.2rem .3rem';
  const opts = mySettlements.map(s => `<option value="${s.settlement_id||s.id}">${esc(s.name)}${s.is_capital?' ★':''}</option>`).join('');
  el.innerHTML = `
    <div class="dsec">
      <div class="dsec-title">New standing order</div>
      <div style="display:flex;flex-direction:column;gap:.35rem;font-size:.78rem">
        <label>From <select id="ec-so-from" style="${inputStyle}">${opts}</select></label>
        <label>To <select id="ec-so-to" style="${inputStyle}">${opts}</select></label>
        <label>Crewed by (which end supplies the gubbe)
          <select id="ec-so-crew" style="${inputStyle}">
            <option value="from">From (source)</option>
            <option value="to">To (destination)</option>
          </select>
        </label>
        <label>Keep at destination — good:threshold, comma-separated
          <input id="ec-so-out" placeholder="grain:200,fish:50" style="${inputStyle}">
        </label>
        <label>Bring home — good:floor, comma-separated (optional)
          <input id="ec-so-home" placeholder="silver:0,stone:20" style="${inputStyle}">
        </label>
        <button class="btn-primary btn-small" onclick="createStandingOrder()">Create route</button>
        <div id="ec-so-result" class="action-result"></div>
      </div>
    </div>
    <div class="dsec">
      <div class="dsec-title">Standing orders</div>
      <div id="ec-so-list" class="loading" style="padding:.4rem 0">Loading…</div>
    </div>`;
  const toSel = document.getElementById('ec-so-to');
  if (toSel && mySettlements.length > 1) toSel.selectedIndex = 1;
  refreshStandingOrders();
}

// renderStandingOrdersHTML is pure (list in, markup out) for the same
// testability reason as renderCargoHTML above.
export function renderStandingOrdersHTML(orders) {
  if (!orders.length) return '<p class="empty-state" style="padding:.4rem 0">No standing orders yet.</p>';
  return '<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem">' +
    '<td>Route</td><td>Status</td><td></td></tr>' +
    orders.map(o => {
      const statusText = o.status === 'paused'
        ? `paused${o.pause_reason ? ' — ' + esc(o.pause_reason) : ''}`
        : 'active';
      const toggleLabel = o.status === 'paused' ? 'Resume' : 'Pause';
      const toggleFn = o.status === 'paused' ? 'resumeStandingOrder' : 'pauseStandingOrder';
      return `<tr>
        <td>${esc(o.from_name)} → ${esc(o.to_name)}</td>
        <td>${esc(statusText)}</td>
        <td style="text-align:right;white-space:nowrap">
          <button class="btn-small" onclick="${toggleFn}('${o.id}')">${toggleLabel}</button>
          <button class="btn-small" onclick="deleteStandingOrder('${o.id}')">Delete</button>
        </td>
      </tr>`;
    }).join('') +
    '</table>';
}

async function refreshStandingOrders() {
  const el = document.getElementById('ec-so-list');
  if (!el) return;
  el.innerHTML = '<div class="loading" style="padding:.4rem 0">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/standing-orders`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>'; return; }
    const orders = (await r.json()) || [];
    el.innerHTML = renderStandingOrdersHTML(orders);
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>';
  }
}

export async function createStandingOrder() {
  const from = document.getElementById('ec-so-from')?.value;
  const to = document.getElementById('ec-so-to')?.value;
  const crew = document.getElementById('ec-so-crew')?.value;
  const outSpec = document.getElementById('ec-so-out')?.value || '';
  const homeSpec = document.getElementById('ec-so-home')?.value || '';
  const resultEl = document.getElementById('ec-so-result');
  if (!resultEl) return;
  if (!from || !to || from === to) { resultEl.style.color = 'var(--accent)'; resultEl.textContent = 'Pick two different settlements.'; return; }
  const outbound = parseGoodAmountPairs(outSpec).map(p => ({ good_key: p.good_key, threshold: p.amount }));
  if (!outbound.length) { resultEl.style.color = 'var(--accent)'; resultEl.textContent = 'Name at least one good:threshold to keep topped up.'; return; }
  const ret = parseGoodAmountPairs(homeSpec).map(p => ({ good_key: p.good_key, floor: p.amount }));
  const crewedBy = crew === 'to' ? to : from;
  resultEl.textContent = '';
  const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/standing-orders`, {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ from_settlement_id: from, to_settlement_id: to, crewed_by_settlement_id: crewedBy, outbound, return: ret }),
  });
  const d = await r.json().catch(() => ({}));
  if (r.ok) {
    resultEl.style.color = 'var(--safe)';
    resultEl.textContent = 'Standing order created.';
    refreshStandingOrders();
  } else {
    resultEl.style.color = 'var(--accent)';
    resultEl.textContent = d.error || 'Could not create standing order.';
  }
}

export async function pauseStandingOrder(id) {
  await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/standing-orders/${id}/pause`, { method: 'POST' });
  refreshStandingOrders();
}

export async function resumeStandingOrder(id) {
  await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/standing-orders/${id}/resume`, { method: 'POST' });
  refreshStandingOrders();
}

export async function deleteStandingOrder(id) {
  await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/standing-orders/${id}`, { method: 'DELETE' });
  refreshStandingOrders();
}

async function loadEconomyWants() {
  const el = document.getElementById('ectab-wants');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/market/wants`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>'; return; }
    const data = await r.json();
    const wants = data.wants || [], surplus = data.surplus || [];
    if (!wants.length && !surplus.length) { el.innerHTML = '<p class="empty-state" style="padding:1rem">No known market intel yet — visit settlements to learn their market.</p>'; return; }
    const rateCell = g => g.rate < 0
      ? `<span style="color:var(--danger)">▼ ${g.rate.toFixed(1)}/tick</span>`
      : `<span style="color:var(--safe)">▲ +${g.rate.toFixed(1)}/tick</span>`;
    let html = '';
    if (wants.length) {
      html += '<div class="dsec-title">Wants — buy here at a premium</div>' + wants.map(sw => `
        <div class="dsec">
          <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
          <table class="goods-mini">${sw.goods.map(g =>
            `<tr><td>${g.good}</td><td>${Math.floor(g.stock)}</td><td style="text-align:right">${rateCell(g)}</td></tr>`
          ).join('')}</table>
        </div>`).join('');
    }
    if (surplus.length) {
      html += '<div class="dsec-title" style="margin-top:.6rem">Surplus — sell cheap here</div>' + surplus.map(sw => `
        <div class="dsec">
          <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
          <table class="goods-mini">${sw.goods.map(g =>
            `<tr><td>${g.good}</td><td>${Math.floor(g.stock)}</td><td style="text-align:right">${rateCell(g)}</td></tr>`
          ).join('')}</table>
        </div>`).join('');
    }
    el.innerHTML = html;
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>';
  }
}
