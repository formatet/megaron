import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc, fmtAgo } from '../format.js';
import { fmtEta, fmtArrival, arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';

// ── Diplomacy drawer ──────────────────────────────────────────────────────
export async function loadDiplomacyDrawer() {
  const body = document.getElementById('diplomacy-body');
  body.innerHTML = `
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="threads">Correspondence</button>
      <button class="dtab" data-tab="compose">Compose</button>
      <button class="dtab" data-tab="cities">Cities</button>
      <button class="dtab" data-tab="rulers">Rulers</button>
    </div>
    <div id="dtab-threads" class="city-tab"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="dtab-compose" class="city-tab" style="display:none"></div>
    <div id="dtab-cities" class="city-tab" style="display:none"></div>
    <div id="dtab-rulers" class="city-tab" style="display:none"></div>`;

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('dtab-' + this.dataset.tab);
      if (el) el.style.display = '';
      if (this.dataset.tab === 'compose') loadDipCompose();
      else if (this.dataset.tab === 'cities') loadDipCities();
      else if (this.dataset.tab === 'rulers') loadDipRulers();
    });
  });

  await loadDipThreads();
}

async function loadDipCities() {
  const el = document.getElementById('dtab-cities');
  if (el.dataset.loaded) return;
  el.dataset.loaded = '1';
  el.innerHTML = '<div class="loading" style="font-size:.8rem">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/cities`);
    if (!r.ok) throw new Error();
    const cities = await r.json();
    if (!cities.length) { el.innerHTML = '<p class="empty-state" style="padding:1rem">No cities known yet — explore the map.</p>'; return; }
    el.innerHTML = `<table class="goods-mini">
      <tr style="color:var(--text-dim);font-size:.7rem"><td>City</td><td>Owner</td><td>Loc</td><td>Deposits</td></tr>
      ${cities.map(c => {
        const deps = [c.copper_deposit?'Cu':null, c.tin_deposit?'Sn':null, c.silver_deposit?'Ag':null, c.cedar_deposit?'Cedar':null].filter(Boolean).join(' ');
        const loc = c.knowledge === 'known' ? (c.q!=null ? `(${c.q},${c.r})` : '—') : (c.bearing || 'rumour');
        return `<tr><td>${esc(c.name)}${c.own?' ★':''}</td><td>${esc(c.owner||'—')}</td><td style="color:var(--text-dim)">${loc}</td><td style="color:var(--text-dim)">${deps||c.industry_hint||''}</td></tr>`;
      }).join('')}
    </table>`;
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load cities.</p>';
  }
}

async function loadDipRulers() {
  const el = document.getElementById('dtab-rulers');
  if (el.dataset.loaded) return;
  el.dataset.loaded = '1';
  el.innerHTML = '<div class="loading" style="font-size:.8rem">Loading…</div>';
  try {
    const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/diplomacy`);
    if (!r.ok) throw new Error();
    const rulers = await r.json();
    if (!rulers.length) { el.innerHTML = '<p class="empty-state" style="padding:1rem">No rulers known yet.</p>'; return; }
    el.innerHTML = `<table class="goods-mini">
      <tr style="color:var(--text-dim);font-size:.7rem"><td>Wanax</td><td>Known</td><td>Rumour</td></tr>
      ${rulers.map(d => `<tr><td>${esc(d.owner)}${d.own?' ★':''}${d.rumour_only?' <span style="color:var(--text-dim);font-size:.68rem">(rumour only)</span>':''}</td><td>${d.known_cities}</td><td style="color:var(--text-dim)">${d.rumour_cities}</td></tr>`).join('')}
    </table>`;
  } catch (_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load rulers.</p>';
  }
}

async function loadDipThreads() {
  const el = document.getElementById('dtab-threads');
  if (!el) return;
  let myGoods = {};
  const capital = ownCapital();
  if (capital) {
    const gr = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + capital.id + '/goods');
    if (gr.ok) { const list = await gr.json().catch(() => []); list.forEach(g => { myGoods[g.key] = g.amount || 0; }); }
  }
  try {
    const [inR, outR] = await Promise.all([
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/inbox'),
      State.MY_SETTLEMENT_ID ? fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/messengers') : Promise.resolve(null),
    ]);
    const inbox  = (inR && inR.ok)  ? await inR.json().catch(() => [])  : [];
    const sent   = (outR && outR.ok) ? await outR.json().catch(() => []) : [];

    // Normalise both sides into a common shape and group by counterpart name.
    // Inbox: from_name/from_id is the counterpart; Sent: destination_name/destination_id is counterpart.
    const threads = {}; // key → { name, settlement_id, messages[] }

    for (const m of inbox) {
      const key = m.from_name || 'Unknown';
      if (!threads[key]) threads[key] = { name: key, settlement_id: m.from_id || null, messages: [] };
      threads[key].messages.push({ ...m, _dir: 'in' });
    }
    for (const m of sent) {
      const key = m.destination_name || 'unknown';
      if (!threads[key]) threads[key] = { name: key, settlement_id: m.destination_id || null, messages: [] };
      else if (!threads[key].settlement_id) threads[key].settlement_id = m.destination_id || null;
      threads[key].messages.push({ ...m, _dir: 'out' });
    }

    const keys = Object.keys(threads);
    if (!keys.length) { el.innerHTML = '<p class="empty-state" style="padding:.5rem">No correspondence yet.</p>'; return; }

    const msgTime = m => new Date(m.arrived_at || m.sent_at || m.created_at || 0).getTime();
    keys.sort((a,b) => {
      const ta = Math.max(...threads[a].messages.map(msgTime));
      const tb = Math.max(...threads[b].messages.map(msgTime));
      return tb - ta;
    });

    // Remember which threads were open so re-render keeps them open
    const openSet = new Set();
    el.querySelectorAll('.dip-thread[data-open]').forEach(t => openSet.add(t.id));

    el.innerHTML = keys.map(key => {
      const thread = threads[key];
      const msgs = [...thread.messages].sort((a,b) => msgTime(a) - msgTime(b));
      const latest = msgs[msgs.length - 1];
      const hasPendingTrade = msgs.some(m => m._dir === 'in' && m.trade_offer && m.trade_offer.status === 'pending');
      const hasUnread = msgs.some(m => m._dir === 'in' && m.status === 'delivered');

      const threadId = 'dip-thread-' + key.replace(/[^a-z0-9]/gi,'_');
      const isOpen = openSet.has(threadId);
      const safeDestId = esc(thread.settlement_id || '');
      const safeName   = esc(thread.name);

      let badges = '';
      if (hasPendingTrade) badges += ' <span style="color:var(--accent-trade);font-size:.65rem">⚖</span>';
      if (hasUnread)       badges += ' <span style="color:var(--accent);font-size:.65rem">●</span>';
      const latestPreview = latest.message || latest.message_text || '';

      // Thread wrapper: data-open attribute controls CSS visibility of body
      let html = '<div class="dip-thread" id="' + threadId + '"' + (isOpen ? ' data-open' : '') + '>'
        + '<div class="dip-thread-header" onclick="dipToggleThread(\'' + threadId + '\')">'
        + '<span style="font-size:.82rem;font-weight:bold;color:var(--accent-diplomacy)">'
        +   safeName + badges
        + '</span>'
        + '<span style="font-size:.68rem;color:var(--text-dim)">'
        +   fmtAgo(latest.arrived_at || latest.sent_at || latest.created_at)
        +   ' <span class="dip-thread-expand-hint">' + (isOpen ? '▲' : '▼') + '</span>'
        + '</span>'
        + '</div>';

      if (latestPreview && !isOpen) {
        html += '<div class="dip-thread-preview" onclick="dipToggleThread(\'' + threadId + '\')">'
          + '"' + esc(latestPreview.slice(0,80)) + (latestPreview.length > 80 ? '…' : '') + '"'
          + '</div>';
      }

      // Thread body (CSS shows/hides via data-open on parent)
      html += '<div class="dip-thread-body">';

      // Messages as chat bubbles
      for (const m of msgs) {
        if (m._dir === 'in') {
          const offer = m.trade_offer;
          let tradeHtml = '';
          if (offer && offer.status === 'pending') {
            const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim)">expires ' + fmtEta(m.expires_at) + '</div>' : '';
            if (offer.kind === 'sell') {
              tradeHtml = '<div id="dip-trade-' + m.id + '" style="background:var(--warm-white);border:1px solid var(--border);padding:.3rem .5rem;margin:.3rem 0;font-size:.78rem">'
                + '<div>Sells <strong>' + offer.offer_qty + ' ' + esc(offer.offer_good) + '</strong> for <strong>' + offer.want_silver + ' silver</strong></div>' + countdown
                + '<div style="display:flex;gap:.4rem;margin-top:.3rem">'
                + '<button class="btn-small btn-primary" onclick="dipAccept(\'' + m.id + '\',this)">Accept ✓</button>'
                + '<button class="btn-small btn-danger"  onclick="dipDecline(\'' + m.id + '\',this)">Decline ✗</button>'
                + '</div></div>';
            } else {
              const have = myGoods[offer.want_good] || 0;
              const canAccept = have >= offer.want_qty;
              const dis = canAccept ? '' : 'disabled title="Need ' + offer.want_qty + ' ' + esc(offer.want_good) + ', have ' + Math.floor(have) + '"';
              tradeHtml = '<div id="dip-trade-' + m.id + '" style="background:var(--warm-white);border:1px solid var(--border);padding:.3rem .5rem;margin:.3rem 0;font-size:.78rem">'
                + '<div>Wants <strong>' + offer.want_qty + ' ' + esc(offer.want_good) + '</strong> · Offers <strong>' + offer.offer_silver + ' silver</strong></div>' + countdown
                + '<div style="display:flex;gap:.4rem;margin-top:.3rem">'
                + '<button class="btn-small btn-primary" onclick="dipAccept(\'' + m.id + '\',this)" ' + dis + '>Accept ✓</button>'
                + '<button class="btn-small btn-danger"  onclick="dipDecline(\'' + m.id + '\',this)">Decline ✗</button>'
                + '</div></div>';
            }
          } else if (offer) {
            tradeHtml = '<div style="font-size:.72rem;color:var(--text-dim);font-style:italic;margin:.2rem 0">Trade: ' + esc(offer.status) + '</div>';
          }
          const replyRow = !offer
            ? '<div id="dip-reply-row-' + m.id + '" style="display:flex;gap:.3rem;margin-top:.3rem">'
              + '<input id="dip-reply-' + m.id + '" type="text" placeholder="Reply…" maxlength="1000" style="flex:1;background:var(--warm-white);border:1px solid var(--border);padding:.2rem .4rem;font-size:.75rem;font-family:var(--mono)">'
              + '<button onclick="dipReply(\'' + m.id + '\')" style="padding:.2rem .5rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer">Reply</button>'
              + '</div>'
            : '';
          html += '<div id="dip-msg-' + m.id + '" class="dip-msg-row dip-msg-row-in">'
            + '<div class="dip-bubble dip-bubble-in">'
            + '<div class="dip-bubble-meta">← ' + esc(m.from_name || 'Unknown') + ' · ' + fmtAgo(m.arrived_at) + '</div>'
            + (m.message ? '<div class="dip-msg-text">' + esc(m.message) + '</div>' : '')
            + tradeHtml + replyRow
            + '</div></div>';
        } else {
          // Sent message
          const offerStatus = m.trade_offer && m.trade_offer.status;
          const statusBit = offerStatus === 'accepted' ? '<span style="color:var(--safe)">✓ accepted</span>'
                          : offerStatus === 'declined' ? '<span style="color:var(--text-dim)">✗ declined</span>'
                          : offerStatus === 'expired'  ? '<span style="color:var(--text-dim)">⏳ expired</span>'
                          : m.status === 'returned'   ? '<span style="color:var(--safe)">↩ returned</span>'
                          : m.status === 'delivering' ? '<span style="color:var(--text-dim)">en route · arrives ' + arrivalHTML(m.arrives_at) + '</span>'
                          : '<span style="color:var(--text-dim)">' + esc(m.status || '') + '</span>';
          const replyText = m.reply_text
            ? '<div class="dip-msg-text" style="color:var(--safe);text-align:right">' + esc(m.reply_text) + '</div>'
            : '';
          let cancelBtn = '';
          if (m.trade_offer && m.trade_offer.status === 'pending') {
            const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim);text-align:right">expires ' + fmtEta(m.expires_at) + '</div>' : '';
            cancelBtn = countdown + '<div style="text-align:right;margin-top:.2rem"><button class="btn-small btn-danger" onclick="dipCancel(\'' + m.id + '\',this)">Cancel offer ✗</button></div>';
          }
          html += '<div class="dip-msg-row dip-msg-row-out">'
            + '<div class="dip-bubble dip-bubble-out">'
            + '<div class="dip-bubble-meta dip-bubble-meta-out">You → ' + esc(m.destination_name || 'unknown') + ' · ' + fmtAgo(m.sent_at || m.created_at) + '</div>'
            + '<div class="dip-msg-text">' + esc(m.message_text || m.message || '') + '</div>'
            + replyText
            + '<div style="font-size:.68rem;font-family:var(--mono);text-align:right">' + statusBit + '</div>'
            + cancelBtn
            + '</div></div>';
        }
      }

      // Inline compose
      if (thread.settlement_id && State.MY_SETTLEMENT_ID) {
        const cid = threadId + '-compose';
        html += '<div class="dip-inline-compose" id="' + cid + '">'
          + '<textarea id="' + cid + '-text" maxlength="1000" placeholder="Send a message to ' + safeName + '…"></textarea>'
          + '<details><summary style="cursor:pointer;color:var(--accent);font-size:.75rem;user-select:none">+ Attach trade offer</summary>'
          + '<div class="dip-inline-trade" style="margin-top:.3rem">'
          + '<div style="display:flex;gap:.6rem;font-size:.7rem;margin-bottom:.3rem">'
          + '<label><input type="radio" name="' + cid + '-kind" value="buy" checked onchange="dipToggleKind(\'' + cid + '\')"> Buy</label>'
          + '<label><input type="radio" name="' + cid + '-kind" value="sell" onchange="dipToggleKind(\'' + cid + '\')"> Sell</label>'
          + '</div>'
          + '<div id="' + cid + '-buy-fields">'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Want good</div><input id="' + cid + '-good" type="text" placeholder="grain"></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Quantity</div><input id="' + cid + '-qty" type="number" min="0.1" step="0.1" placeholder="50"></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer silver</div><input id="' + cid + '-silver" type="number" min="1" step="1" placeholder="60"></div>'
          + '</div>'
          + '<div id="' + cid + '-sell-fields" style="display:none">'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer good</div><input id="' + cid + '-offer-good" type="text" placeholder="copper"></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Quantity</div><input id="' + cid + '-offer-qty" type="number" min="0.1" step="0.1" placeholder="20"></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Want silver</div><input id="' + cid + '-want-silver" type="number" min="1" step="1" placeholder="80"></div>'
          + '</div>'
          + '</div></details>'
          + '<div class="dip-inline-compose-row">'
          + '<button class="btn-small btn-primary" onclick="dipSendInThread(\'' + cid + '\',\'' + safeDestId + '\')">Dispatch →</button>'
          + '<span class="dip-compose-status" id="' + cid + '-status"></span>'
          + '</div>'
          + '</div>';
      }

      html += '</div></div>'; // close thread-body + thread
      return html;
    }).join('');
    el.innerHTML += await renderLockedActions('diplomacy');

  } catch(e) {
    console.error('loadDipThreads', e);
    el.innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load correspondence.</p>';
  }
}

export function dipToggleKind(cid) {
  const kind = document.querySelector('input[name="' + cid + '-kind"]:checked')?.value || 'buy';
  const buyEl = document.getElementById(cid + '-buy-fields');
  const sellEl = document.getElementById(cid + '-sell-fields');
  if (buyEl) buyEl.style.display = kind === 'buy' ? '' : 'none';
  if (sellEl) sellEl.style.display = kind === 'sell' ? '' : 'none';
}

export function dipToggleThread(id) {
  const el = document.getElementById(id);
  if (!el) return;
  const isOpen = el.hasAttribute('data-open');
  if (isOpen) {
    el.removeAttribute('data-open');
  } else {
    el.setAttribute('data-open', '');
  }
  // Update chevron hint
  const hint = el.querySelector('.dip-thread-expand-hint');
  if (hint) hint.textContent = el.hasAttribute('data-open') ? '▲' : '▼';
  // Show/hide preview when collapsed
  const preview = el.querySelector('.dip-thread-preview');
  if (preview) preview.style.display = el.hasAttribute('data-open') ? 'none' : '';
}

export async function dipSendInThread(cid, destId) {
  const textEl   = document.getElementById(cid + '-text');
  const statusEl = document.getElementById(cid + '-status');
  const text = textEl ? textEl.value.trim() : '';
  function showStatus(msg, ok) {
    if (statusEl) { statusEl.style.color = ok ? 'var(--safe)' : 'var(--accent)'; statusEl.textContent = msg; }
  }
  if (!text)   { showStatus('write a message', false); return; }
  if (!destId) { showStatus('no destination', false); return; }
  if (!State.MY_SETTLEMENT_ID) { showStatus('no settlement', false); return; }
  const body = { destination_id: destId, message: text };
  const kind = document.querySelector('input[name="' + cid + '-kind"]:checked')?.value || 'buy';
  if (kind === 'sell') {
    const offerGood = document.getElementById(cid + '-offer-good')?.value.trim();
    const offerQty  = parseFloat(document.getElementById(cid + '-offer-qty')?.value || '0');
    const wantSilver = parseFloat(document.getElementById(cid + '-want-silver')?.value || '0');
    if (offerGood && offerQty > 0 && wantSilver > 0) body.trade_offer = { kind: 'sell', offer_good: offerGood, offer_qty: offerQty, want_silver: wantSilver };
  } else {
    const good   = document.getElementById(cid + '-good')?.value.trim();
    const qty    = parseFloat(document.getElementById(cid + '-qty')?.value || '0');
    const silver = parseFloat(document.getElementById(cid + '-silver')?.value || '0');
    if (good && qty > 0 && silver > 0) body.trade_offer = { kind: 'buy', want_good: good, want_qty: qty, offer_silver: silver };
  }
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/messengers', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    showStatus('✓ Dispatched · arrives ' + fmtArrival(data.arrives_at), true);
    if (textEl) textEl.value = '';
    fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers').then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    // Reload threads after a short delay so sent message appears
    setTimeout(() => loadDipThreads(), 1200);
  } else {
    showStatus(data.error || 'send failed', false);
  }
}

export async function dipCancel(id, btn) {
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/trade-cancel', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  if (res.ok) {
    await loadDipThreads();
  } else {
    const data = await res.json().catch(() => ({}));
    btn.disabled = false;
    alert(data.error || 'Cancel failed');
  }
}

export async function dipAccept(id, btn) {
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/trade-accept', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const data = await res.json().catch(() => ({}));
  const block = document.getElementById('dip-trade-' + id);
  if (res.ok && block) {
    block.innerHTML = '<span style="color:var(--safe)">✓ Accepted — ' + data.quantity + ' ' + esc(data.good_key || '') + ' arriving ' + arrivalHTML(data.goods_arrives_at) + ' · ' + data.silver_paid + ' silver paid</span>';
  } else {
    btn.disabled = false;
    if (block) {
      const err = document.createElement('div');
      err.style.cssText = 'color:var(--accent);font-size:.72rem;margin-top:.2rem';
      err.textContent = data.error || 'failed';
      block.appendChild(err);
      setTimeout(() => err.remove(), 6000);
    }
  }
}

export async function dipDecline(id, btn) {
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/trade-decline', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const block = document.getElementById('dip-trade-' + id);
  if (res.ok && block) block.innerHTML = '<span style="color:var(--text-dim);font-style:italic">Declined.</span>';
  else btn.disabled = false;
}

export async function dipReply(id) {
  const input = document.getElementById('dip-reply-' + id);
  const text = input ? input.value.trim() : '';
  if (!text) return;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/reply', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ reply: text }),
  });
  const data = await res.json().catch(() => ({}));
  const row = document.getElementById('dip-reply-row-' + id);
  if (res.ok && row) {
    row.outerHTML = '<div style="font-size:.72rem;color:var(--safe);margin-top:.2rem">✓ Reply dispatched · returns ' + arrivalHTML(data.returns_at) + '</div>';
  }
}

function loadDipCompose() {
  const el = document.getElementById('dtab-compose');
  if (!el || el.dataset.loaded) return;
  el.dataset.loaded = '1';
  const others = State.provinceData.filter(p => !p.own && !p.is_outpost && p.settlement_id && p.name);
  const opts = others.length
    ? '<option value="">— choose settlement —</option>' + others.map(p => '<option value="' + p.settlement_id + '">' + esc(p.name) + ' (' + (p.allied ? 'ally' : 'foreign') + ')</option>').join('')
    : '<option value="">No visible settlements — explore the map</option>';
  el.innerHTML = '<div class="dsec"><div class="dsec-title">Send Messenger</div>'
    + '<div style="display:flex;flex-direction:column;gap:.5rem">'
    + '<select id="dip-dest" style="background:var(--warm-white);border:1px solid var(--border);padding:.3rem .4rem;font-size:.82rem">' + opts + '</select>'
    + '<textarea id="dip-msg-text" maxlength="1000" placeholder="Your words travel with the messenger…" style="resize:vertical;min-height:4rem;background:var(--warm-white);border:1px solid var(--border);padding:.3rem .4rem;font-size:.82rem;font-family:var(--font)"></textarea>'
    + '<details><summary style="cursor:pointer;color:var(--accent);font-size:.78rem;user-select:none">+ Attach trade offer</summary>'
    + '<div style="display:flex;gap:.6rem;font-size:.75rem;margin:.4rem 0 .2rem">'
    + '<label><input type="radio" name="dip-kind" value="buy" checked onchange="dipComposeToggleKind()"> Buy</label>'
    + '<label><input type="radio" name="dip-kind" value="sell" onchange="dipComposeToggleKind()"> Sell</label>'
    + '</div>'
    + '<div id="dip-buy-fields" style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:.4rem">'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Want good</div><input id="dip-ogood" type="text" placeholder="grain" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Quantity</div><input id="dip-oqty" type="number" min="0.1" step="0.1" placeholder="50" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Offer silver</div><input id="dip-osilver" type="number" min="1" step="1" placeholder="60" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '</div>'
    + '<div id="dip-sell-fields" style="display:none;grid-template-columns:1fr 1fr 1fr;gap:.4rem">'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Offer good</div><input id="dip-offer-good" type="text" placeholder="copper" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Quantity</div><input id="dip-offer-qty" type="number" min="0.1" step="0.1" placeholder="20" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '<div><div style="font-size:.7rem;color:var(--text-dim)">Want silver</div><input id="dip-want-silver" type="number" min="1" step="1" placeholder="80" style="width:100%;background:var(--warm-white);border:1px solid var(--border);padding:.25rem .35rem;font-size:.78rem;box-sizing:border-box"></div>'
    + '</div>'
    + '</details>'
    + '<button onclick="dipSend()" style="padding:.3rem .8rem;background:var(--sandstone);border:2px solid var(--border);font-family:var(--mono);font-size:.78rem;letter-spacing:.07em;cursor:pointer;align-self:flex-start">Dispatch →</button>'
    + '<div id="dip-compose-res" style="font-size:.78rem;min-height:1rem"></div>'
    + '</div></div>';
}

export function dipComposeToggleKind() {
  const kind = document.querySelector('input[name="dip-kind"]:checked')?.value || 'buy';
  const buyEl = document.getElementById('dip-buy-fields');
  const sellEl = document.getElementById('dip-sell-fields');
  if (buyEl) buyEl.style.display = kind === 'buy' ? 'grid' : 'none';
  if (sellEl) sellEl.style.display = kind === 'sell' ? 'grid' : 'none';
}

export async function dipSend() {
  const destID = document.getElementById('dip-dest')?.value;
  const text   = document.getElementById('dip-msg-text')?.value.trim();
  const resEl  = document.getElementById('dip-compose-res');
  function showDipRes(msg, ok) { if (resEl) { resEl.style.color = ok ? 'var(--safe)' : 'var(--accent)'; resEl.textContent = msg; } }
  if (!destID) { showDipRes('choose a destination', false); return; }
  if (!text)   { showDipRes('write a message', false); return; }
  if (!State.MY_SETTLEMENT_ID) { showDipRes('you have no settlement', false); return; }
  const body = { destination_id: destID, message: text };
  const kind = document.querySelector('input[name="dip-kind"]:checked')?.value || 'buy';
  if (kind === 'sell') {
    const offerGood = document.getElementById('dip-offer-good')?.value.trim();
    const offerQty  = parseFloat(document.getElementById('dip-offer-qty')?.value || '0');
    const wantSilver = parseFloat(document.getElementById('dip-want-silver')?.value || '0');
    if (offerGood && offerQty > 0 && wantSilver > 0) body.trade_offer = { kind: 'sell', offer_good: offerGood, offer_qty: offerQty, want_silver: wantSilver };
  } else {
    const good   = document.getElementById('dip-ogood')?.value.trim();
    const qty    = parseFloat(document.getElementById('dip-oqty')?.value || '0');
    const silver = parseFloat(document.getElementById('dip-osilver')?.value || '0');
    if (good && qty > 0 && silver > 0) body.trade_offer = { kind: 'buy', want_good: good, want_qty: qty, offer_silver: silver };
  }
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/messengers', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    showDipRes('✓ Dispatched · arrives ' + fmtArrival(data.arrives_at), true);
    const mtel = document.getElementById('dip-msg-text'); if (mtel) mtel.value = '';
    const dsel = document.getElementById('dip-dest'); if (dsel) dsel.value = '';
    fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers').then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
  } else {
    showDipRes(data.error || 'send failed', false);
  }
}
