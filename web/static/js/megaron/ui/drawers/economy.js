import { fmtNum, fmtDays } from '../fmt_num.js';
import { fmtTickTime } from '../fmt_clock.js';
import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { serverNow } from '../../clock.js';
import { esc, byGoodName } from '../format.js';
import { renderLockedActions } from '../misc.js';
import { loadSendGoods } from './send_goods.js';

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
    const lost = rate > 0 ? ` · +${fmtNum(Number(rate.toFixed(1)))} lost per game day` : '';
    const title = `Storage full (${fmtNum(Math.floor(g.amount || 0))} of ${fmtNum(Math.floor(g.cap || 0))}). `
      + 'Everything produced past the cap is discarded — move this labour or trade it away.';
    return `<td class="goods-atcap" title="${esc(title)}">full${lost}</td>`;
  }
  return rate > 0 ? `<td style="color:var(--safe-text)">+${fmtNum(Number(rate.toFixed(1)))} per game day</td>` : '<td></td>';
}

// Compare the single city food stocks and authoritative food net, including army
// upkeep when reported. Hunger follows the server growth state, never a local
// stock threshold: low coverage alone does not mean a city is starving.
const FOOD_RANK = { starving: 0, declining: 1, growing: 2, stable: 3, unknown: 5 };
const FOOD_LABEL = { starving: 'starving', declining: 'stocks falling', growing: 'stocks growing', stable: 'stable', unknown: 'no data' };

export function settlementFoodRow(s, pd) {
  if (!pd) {
    return { id: s.id, name: s.name, isCapital: !!s.is_capital, population: 0,
      grainRate: null, coverage: null, severity: 'unknown', label: 'no data', rank: 5 };
  }
  const food = pd.food;
  const cov = food?.coverage_ticks;
  const net = pd.food_net_per_tick_after_upkeep ?? food?.food_net_per_tick;
  const coverageKnown = cov != null && Number.isFinite(cov);
  const netKnown = net != null && Number.isFinite(net);
  const known = coverageKnown && netKnown;
  const cannotFeed = pd.food_self_sufficient === false;
  const severity = !known ? 'unknown' : food.growth_state === 'shrinking' ? 'starving'
    : net < 0 ? 'declining' : net > 0 ? 'growing' : 'stable';
  return {
    id: s.id,
    name: s.name,
    isCapital: !!s.is_capital,
    population: pd.population || 0,
    grainRate: netKnown ? net : null,
    coverage: coverageKnown ? cov : null,
    severity: cannotFeed ? 'starving' : severity,
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
      ${th('grainRate', 'Food after army', 'right')}
      ${th('coverage', 'Food lasts', 'right')}
      ${th('brist', 'Status')}
    </tr>
    ${rows.map(r => {
      const rateColor = r.grainRate < 0 ? 'var(--danger)' : (r.grainRate > 0 ? 'var(--safe-text)' : 'var(--text-dim)');
      const rateSign = r.grainRate > 0 ? '+' : '';
      return `<tr onclick="openCitySettlement('${r.id}')" style="cursor:pointer">
        <td>${esc(r.name)}${r.isCapital ? ' ★' : ''}</td>
        <td style="text-align:right">${fmtNum(Math.floor(r.population))}</td>
        <td style="text-align:right;color:${rateColor}">${r.grainRate == null ? 'not reported' : rateSign + fmtNum(Number(r.grainRate.toFixed(1))) + ' per game day'}</td>
        <td style="text-align:right">${r.coverage == null ? 'not reported' : fmtDays(Number(r.coverage.toFixed(1)))}</td>
        <td><span class="food-state-${r.severity}">${esc(r.label)}</span></td>
      </tr>`;
    }).join('')}
  </table>`;
}

// Row cache is rebuilt by every loadEconomyGoods() call (tab switch, drawer
// reopen) — it only ever holds the most recent fetch. Sort choice is kept
// across those reloads on purpose: re-opening the tab after acting on the
// worst-off city shouldn't silently drop the Wanax back to the default order.
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
  window.closeDrawer('economy');
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
      '<button class="dtab" data-tab="send">Send goods</button>' +
      '<button class="dtab" data-tab="road">On the road</button>' +
    '</div>' +
    '<div id="ectab-goods" class="city-tab"></div>' +
    '<div id="ectab-send" class="city-tab" style="display:none"></div>' +
    '<div id="ectab-road" class="city-tab" style="display:none"></div>';

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('ectab-' + this.dataset.tab);
      if (el) el.style.display = '';
      if (this.dataset.tab === 'goods') loadEconomyGoods(mySettlements);
      else if (this.dataset.tab === 'send') loadSendGoods(mySettlements);
      else if (this.dataset.tab === 'road') loadEconomyRoad();
    });
  });


  await loadEconomyGoods(mySettlements);
}

async function loadEconomyGoods(mySettlements) {
  const el = document.getElementById('ectab-goods');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const [goodsResults, overviewResult] = await Promise.all([
      Promise.all(mySettlements.map(s => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${s.id}/goods`))),
      // One call for every owned settlement's food fields, instead of
      // the old N /provinces/{id} fetches (megaron_plan_oversiktsendpoint.md,
      // 2026-09-06). Same economy.* functions as /provinces/{id} under the
      // hood, so the rendered rows are unchanged.
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/overview`),
    ]);

    const overviewByID = new Map();
    if (overviewResult.ok) {
      for (const pd of await overviewResult.json()) overviewByID.set(pd.id, pd);
    }
    // Province rows carry both ids: overview is settlement-scoped; links and goods stay province-scoped.
    _settlementRows = mySettlements.map(s => settlementFoodRow(s, overviewByID.get(s.settlement_id) || null));
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
      const others = goods.filter(g => g.key !== 'silver' && (g.amount > 0 || g.producible)).sort(byGoodName);
      html += `<div class="dsec-title" id="economy-city-${esc(s.id)}" style="margin-top:.8rem">${esc(s.name)}${s.is_capital?' ★':''}</div>`;
      if (silver) {
        html += `<div class="silver-balance">
          <div>
            <div class="sb-label">Silver</div>
            <div class="sb-val">${fmtNum(Math.floor(silver.amount))}</div>
          </div>
          ${silver.rate_per_tick ? `<div class="sb-rate">+${fmtNum(Number(silver.rate_per_tick.toFixed(1)))} per game day</div>` : ''}
        </div>`;
      }
      if (others.length) {
        html += `<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem"><td>Good</td><td>Amount</td><td>Rate</td></tr>${others.map(g =>
          `<tr><td>${g.name||g.key}</td><td>${fmtNum(Math.floor(g.amount||0))}</td>${goodsRateCell(g)}</tr>`
        ).join('')}</table>`;
      }
    }
    el.innerHTML = overviewHtml + (html || '<p class="empty-state" style="padding:1rem">No goods data.</p>') + await renderLockedActions('trade');
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load goods.</p>';
  }
}


// formatCargoRows is the pure part of the "your cargo in transit" list — no
// DOM, no fetch, no Date.now()/serverNow() read internally. `nowMs` is passed
// in explicitly (the caller supplies serverNow()) so this can be unit-tested
// the same way render/camera.test.mjs tests zoomStep: canned inputs in,
// checked output out, no stubbing required. Each `trade` is one row of
// GET /trades already filtered to the caller's own by the caller.
export function formatCargoRows(trades, nowMs) {
  return trades.flatMap(t => (t.items?.length ? t.items.map(item => ({...t,...item,items:null})) : [t])).map(t => {
    const etaMs = new Date(t.arrives_at).getTime() - nowMs;
    let eta;
    if (t.route_version===2) eta=fmtTickTime(null,t.arrives_at);
    else if (Number.isInteger(t.arrival_tick)) eta = `${fmtTickTime(t.arrival_tick)}`;
    else if (etaMs <= 0) eta = 'arrived';
    else if (etaMs < 3600000) eta = `${Math.floor(etaMs / 60000)}m`;
    else eta = `${Math.floor(etaMs / 3600000)}h ${Math.floor((etaMs % 3600000) / 60000)}m`;
    return {
      id:t.id,
      good: t.good_key || '?',
      qty: Math.floor(t.quantity || 0),
      from: t.origin_name || `(${t.origin_q},${t.origin_r})`,
      to: t.dest_name || `(${t.dest_q},${t.dest_r})`,
      eta,
      direction: t.role === 'recipient' ? 'incoming' : 'outgoing',
      blessings: t.blessings || [],
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
      `<tr data-cargo-id="${esc(row.id||'')}"><td>${esc(row.good)}</td><td>${row.qty}</td><td>${esc(row.from)} → ${esc(row.to)}</td>` +
      `<td>${esc(row.direction)}${row.blessings.map(b => ` · ${esc(b)}`).join('')}</td><td style="text-align:right">${esc(row.eta)}</td></tr>`
    ).join('') +
    '</table>' +
    '<p style="color:var(--text-dim);font-size:.68rem;margin-top:.3rem">' +
    'Physical cargo — it can be intercepted and seized while in transit.</p>';
}

// ── Automation tab (standing orders) ────────────────────────────────────────
// megaron_plan_staende_leverans.md: a caravan that keeps a destination's
// stock topped up on its own (PULL — "hold grain at Colony ≥ 200" — not a
// fixed quantity/interval). Home per megaron_plan_stad_vs_ekonomi.md §1: a
// flow between two settlements belongs to neither city's own drawer.

// The former text-input parser remains a reference for request compatibility
// with keryx's --out/--home flags. The web form reads structured rows below.
export function parseGoodAmountPairs(spec) {
  if (!spec || !spec.trim()) return [];
  return spec.split(',').map(part => {
    const [good, amt] = part.split(':').map(s => (s || '').trim());
    return { good_key: good, amount: parseFloat(amt) };
  }).filter(p => p.good_key && !isNaN(p.amount));
}
// renderStandingOrdersHTML is pure (list in, markup out) for the same
// testability reason as renderCargoHTML above.
export function routeCargoHTML(trades, nowMs) {
 if(!trades.length)return '<p class="empty-state">No cargo travelling.</p>';
 return trades.map(t=>'<div class="standing-order-config" data-cargo-id="'+esc(t.id)+'"><strong>'+ (t.category==='naval'?'Ship':'Caravan')+'</strong> · '+esc(t.origin_name||'Origin')+' → '+esc(t.dest_name||'Destination')+'<br>'+formatCargoRows([t],nowMs).map(row=>esc(String(row.qty))+' '+esc(row.good)+' · '+esc(row.eta)+row.blessings.map(b=>' · '+esc(b)).join('')).join('<br>')+'</div>').join('');
}

export function renderStandingOrdersHTML(orders, trades, nowMs = 0) {
  if (!orders.length) return '<p class="empty-state" style="padding:.4rem 0">No standing orders yet.</p>';
  return '<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem">' +
    '<td>Route</td><td>Status</td><td></td></tr>' +
    orders.map(o => {
      const statusText = o.status === 'paused'
        ? `paused${o.pause_reason ? ' — ' + esc(o.pause_reason) : ''}`
        : 'active';
      const toggleLabel = o.status === 'paused' ? 'Resume' : 'Pause';
      const toggleFn = o.status === 'paused' ? 'resumeStandingOrder' : 'pauseStandingOrder';
      // Sjöhandel kräver skepp (megaron_plan_sjohandel_kraver_skepp.md R4): a
      // naval route locks a real ship for its whole lifetime — name it.
      const shipNote = o.ship_name ? ` <span style="color:var(--text-dim)">⛵ ${esc(o.ship_name)}</span>` : '';
      const stockAmount = value => Number.isFinite(Number(value))
        ? Number(value).toLocaleString('en-US', { maximumFractionDigits: 20 }) : 'unknown';
      const outbound = (o.outbound || []).map(g => `${esc(g.good_key)} ${stockAmount(g.threshold)}`).join(', ');
      const returning = (o.return || []).map(g => `${esc(g.good_key)} ${stockAmount(g.floor)}`).join(', ');
      const travelling = trades === undefined ? '' : trades === null ? '<p class="empty-state">Could not load cargo.</p>' : routeCargoHTML(trades.filter(t => t.standing_order_id === o.id), nowMs);
      const settings = (o.by?`<div class="standing-order-config">By ${esc(o.by)}</div>`: "") + `<div class="standing-order-config">Keep at destination: ${outbound || 'not available'}</div>`
        + `<div class="standing-order-config">${returning ? 'Bring home above: ' + returning : 'Return cargo: none'}</div>` + travelling;
      return `<tr id="standing-order-${esc(o.id)}">
        <td>${esc(o.from_name)} → ${esc(o.to_name)}${shipNote}${settings}</td>
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
    const world=State.WORLD_ID;
    const [r,cargoResponse]=await Promise.all([fetchAuth(`/api/v1/worlds/${world}/standing-orders`),fetchAuth(`/api/v1/worlds/${world}/trades`)]);
    if(State.WORLD_ID!==world||document.getElementById('ec-so-list')!==el)return;
    if (!r.ok) { el.innerHTML = '<p class="empty-state">Could not load routes.</p>'; return; }
    const orders=(await r.json())||[];
    const trades=cargoResponse.ok?(await cargoResponse.json()).filter(t=>t.role==='sender'||t.role==='recipient'):null;
    el.innerHTML=renderStandingOrdersHTML(orders,trades,serverNow());
    const other=document.getElementById('ec-tr-cargo');
    if(other)other.innerHTML=trades===null?'<p class="empty-state">Could not load cargo.</p>':routeCargoHTML(trades.filter(t=>!orders.some(o=>o.id===t.standing_order_id)),serverNow());
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>';
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

// ── On the road: standing orders and other cargo in transit ─────────────────
async function loadEconomyRoad() {
  const el = document.getElementById('ectab-road');
  if (!el) return;
  el.innerHTML = `<div class="dsec">
      <div class="dsec-title">Standing orders</div>
      <div id="ec-so-list" class="loading" style="padding:.4rem 0">Loading…</div>
    </div><div class="dsec"><div class="dsec-title">Other cargo in transit</div><div id="ec-tr-cargo"></div></div>`;
  await refreshStandingOrders();
}

// Activate one Economy tab without a click (used by deep links and the map).
function showEconomyTab(name) {
  document.querySelectorAll('#economy-body .dtab').forEach(t => t.classList.toggle('active', t.dataset.tab === name));
  document.querySelectorAll('#economy-body .city-tab').forEach(t => { t.style.display = t.id === 'ectab-' + name ? '' : 'none'; });
}


export async function focusStandingOrder(id){
 window.openDrawer?.('economy');
 await loadEconomyDrawer();
 showEconomyTab('road');
 await loadEconomyRoad();
 const row=document.getElementById('standing-order-'+id);
 if(row){row.classList.add('ucard-focus');row.scrollIntoView({block:'center'});}
}

// The map's "Send goods →" button: open the form with the destination chosen.
export async function openSendGoods(toSettlementId){
 window.openDrawer?.('economy');
 await loadEconomyDrawer();
 const cities=State.provinceData.filter(p=>p.own&&!p.is_outpost);
 showEconomyTab('send');
 await loadSendGoods(cities,toSettlementId);
}

export async function focusEconomyCity(id){
 window.openDrawer?.('economy');
 await loadEconomyDrawer();
 const title=document.getElementById('economy-city-'+id);
 if(title){title.classList.add('ucard-focus');title.scrollIntoView({block:'center'});}
}
export async function focusCargo(id){
 window.openDrawer?.('economy');await loadEconomyDrawer();
 showEconomyTab('road');
 await loadEconomyRoad();
 const row=[...document.querySelectorAll('[data-cargo-id]')].find(r=>r.dataset.cargoId===id);
 if(row){row.classList.add('ucard-focus');row.scrollIntoView({block:'center'});}
}
