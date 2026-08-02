import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { serverNow } from '../../clock.js';
import { esc, fmtSilver } from '../format.js';
import { renderLockedActions } from '../misc.js';

// ── Economy drawer ────────────────────────────────────────────────────────
export async function loadEconomyDrawer() {
  const body = document.getElementById('economy-body');
  const mySettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
  if (!mySettlements.length) { body.innerHTML = '<p class="empty-state" style="padding:1rem">No settlements.</p>'; return; }

  body.innerHTML =
    '<div class="drawer-tabs">' +
      '<button class="dtab active" data-tab="goods">Goods</button>' +
      '<button class="dtab" data-tab="transfer">Transfer</button>' +
      '<button class="dtab" data-tab="wants">Wants</button>' +
    '</div>' +
    '<div id="ectab-goods" class="city-tab"></div>' +
    '<div id="ectab-transfer" class="city-tab" style="display:none"></div>' +
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
      else if (this.dataset.tab === 'wants') loadEconomyWants();
    });
  });

  loadEconomyGoods(mySettlements);
}

async function loadEconomyGoods(mySettlements) {
  const el = document.getElementById('ectab-goods');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const results = await Promise.all(
      mySettlements.map(s => fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${s.id}/goods`))
    );
    let html = '';
    for (let i = 0; i < mySettlements.length; i++) {
      const s = mySettlements[i];
      const r = results[i];
      if (!r.ok) continue;
      const goods = await r.json();
      const silver = goods.find(g => g.key === 'silver');
      const others = goods.filter(g => g.key !== 'silver' && (g.amount > 0 || g.producible));
      html += `<div class="dsec-title" style="margin-top:${i>0?'.8rem':'0'}">${s.name}${s.is_capital?' ★':''}</div>`;
      if (silver) {
        html += `<div class="silver-balance">
          <div>
            <div class="sb-label">Silver</div>
            <div class="sb-val">${fmtSilver(silver.amount)}</div>
          </div>
          ${silver.rate_per_tick ? `<div class="sb-rate">+${(silver.rate_per_tick*24).toFixed(1)}/day</div>` : ''}
        </div>`;
      }
      if (others.length) {
        html += `<table class="goods-mini"><tr style="color:var(--text-dim);font-size:.7rem"><td>Good</td><td>Amount</td><td>Rate</td><td style="text-align:right">Price</td></tr>${others.map(g =>
          `<tr><td>${g.name||g.key}</td><td>${Math.floor(g.amount||0)}</td>${g.rate_per_tick > 0 ? `<td style="color:var(--safe)">+${(g.rate_per_tick*24).toFixed(1)}/day</td>` : '<td></td>'}<td style="text-align:right;color:var(--text-dim)">${(g.price||0).toFixed(2)}</td></tr>`
        ).join('')}</table>`;
      }
    }
    el.innerHTML = (html || '<p class="empty-state" style="padding:1rem">No goods data.</p>') + await renderLockedActions('trade');
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
// GET /trades already filtered to `trade.mine` by the caller.
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
    '<td>Good</td><td>Qty</td><td>From → To</td><td style="text-align:right">ETA</td></tr>' +
    rows.map(row =>
      `<tr><td>${esc(row.good)}</td><td>${row.qty}</td><td>${esc(row.from)} → ${esc(row.to)}</td>` +
      `<td style="text-align:right">${esc(row.eta)}</td></tr>`
    ).join('') +
    '</table>' +
    '<p style="color:var(--text-dim);font-size:.68rem;margin-top:.3rem">' +
    'Physical cargo — it can be intercepted and seized while in transit.</p>';
}

// Fetches GET /trades and renders the caller's own in-transit cargo (the
// `mine` flag added server-side does the ownership filtering; this only
// keeps the rows where it's true). Called on tab load and again right after
// a successful transfer, so a freshly dispatched caravan appears without
// having to leave and reopen the tab.
async function refreshCargoInTransit() {
  const el = document.getElementById('ec-tr-cargo');
  if (!el) return;
  el.innerHTML = '<div class="loading" style="padding:.4rem 0">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/trades`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state" style="padding:.4rem 0">Could not load.</p>'; return; }
    const trades = (await r.json()) || [];
    el.innerHTML = renderCargoHTML(trades.filter(t => t.mine), serverNow());
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

async function loadEconomyWants() {
  const el = document.getElementById('ectab-wants');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/market/wants`);
    if (!r.ok) { el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>'; return; }
    const data = await r.json();
    const wants = data.wants || [], surplus = data.surplus || [];
    if (!wants.length && !surplus.length) { el.innerHTML = '<p class="empty-state" style="padding:1rem">No known market intel yet — visit settlements to learn their market.</p>'; return; }
    const LEVEL_COLOR = { high: 'var(--danger)', medium: 'var(--border)', low: 'var(--text-dim)' };
    let html = '';
    if (wants.length) {
      html += '<div class="dsec-title">Wants — buy here at a premium</div>' + wants.map(sw => `
        <div class="dsec">
          <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
          <table class="goods-mini">${sw.goods.map(g =>
            `<tr><td>${g.good}</td><td style="color:${LEVEL_COLOR[g.want_level]||'inherit'}">${g.want_level}</td><td style="text-align:right">${g.price.toFixed(2)} <span style="color:var(--text-dim)">(base ${g.base_value.toFixed(2)})</span></td></tr>`
          ).join('')}</table>
        </div>`).join('');
    }
    if (surplus.length) {
      html += '<div class="dsec-title" style="margin-top:.6rem">Surplus — sell cheap here</div>' + surplus.map(sw => `
        <div class="dsec">
          <div class="dsec-title" style="font-size:.75rem">${esc(sw.name)}${sw.secondhand ? ' <span style="color:var(--text-dim);font-size:.68rem">(rumour)</span>' : ''}</div>
          <table class="goods-mini">${sw.goods.map(g =>
            `<tr><td>${g.good}</td><td style="text-align:right">${g.price.toFixed(2)} <span style="color:var(--text-dim)">(base ${g.base_value.toFixed(2)})</span></td></tr>`
          ).join('')}</table>
        </div>`).join('');
    }
    el.innerHTML = html;
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>';
  }
}
