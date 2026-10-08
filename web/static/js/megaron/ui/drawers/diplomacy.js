import { showInlineResult } from '../inline_result.js';
import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { track } from '../../telemetry.js';
import { esc, fmtAgo, formatApiError, passageNote } from '../format.js';
import { fmtEta, fmtArrival, arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { fmtNum } from '../fmt_num.js';
import { sentStatusHTML } from '../runner_status.js';

// "expires <eta>" while a trade offer's window is still open, collapsing to a
// bare "expired" once it's closed — a closed offer isn't an arrival, and
// "expires expired" is not a state a player recognises.
function expiryWord(iso) {
  const eta = fmtEta(iso, undefined, 'expired');
  return eta === 'expired' ? eta : 'expires ' + eta;
}

// Tradeable-goods catalogue (GET /api/v1/goods) — the same set
// MessengerHandler.tradeableGood validates a trade offer's want_good/
// offer_good against (server: api/handlers/goods.go + messenger.go). Static
// for a world's lifetime, so fetch once and memoize rather than refetching
// on every drawer render — same pattern as getRecipes() in city.js:421.
// On failure the promise is cleared so the next render retries instead of
// permanently caching a failure; null (not []) is returned to distinguish
// "could not load" from "server has no tradeable goods" — see
// goodsOptionsHTML below.
let _tradeableGoodsPromise = null;
async function getTradeableGoods() {
  if (!_tradeableGoodsPromise) {
    _tradeableGoodsPromise = fetchAuth('/api/v1/goods').then(r => {
      if (!r.ok) throw new Error('goods fetch failed: ' + r.status);
      return r.json();
    }).catch(e => {
      console.error('getTradeableGoods', e);
      _tradeableGoodsPromise = null;
      return null;
    });
  }
  return _tradeableGoodsPromise;
}

// Renders the <option> list for a want_good/offer_good <select> — shared by
// both fields in the inline conversation composer,
// so the dropdowns can never drift apart from each other or from what
// the server actually accepts. `goods === null` means the catalogue fetch
// failed: an honest "could not load" placeholder, never a silent fallback
// to free text (the tyst fallback CLAUDE.md forbids) — pair with
// goodsSelectDisabledAttr so the control can't actually be used in that case.
export function goodsOptionsHTML(goods) {
  if (goods === null) return '<option value="">Could not load goods</option>';
  if (!goods.length) return '<option value="">No tradeable goods</option>';
  return '<option value="">— choose good —</option>'
    + goods.map(g => '<option value="' + esc(g.key) + '">' + esc(g.name || g.key) + '</option>').join('');
}

// A failed catalogue fetch must disable the control, not just show a
// placeholder option — otherwise a blank selection ships silently as
// want_good/offer_good (megaron_plan_offertens_varulista.md Steg 3).
export function goodsSelectDisabledAttr(goods) {
  return goods === null ? ' disabled' : '';
}

// ── Diplomacy drawer ──────────────────────────────────────────────────────
let draftDestination = null;

// The exact destination set the former Compose dropdown used. The city
// directory includes rumours too; it must never become a dispatch catalogue.
function writableCities() {
  return State.provinceData.filter(p => !p.own && !p.is_outpost && p.settlement_id && p.name);
}

export async function loadDiplomacyDrawer() {
  draftDestination = null;
  const body = document.getElementById('diplomacy-body');
  body.innerHTML = `
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="threads">Correspondence</button>
      <button class="dtab" data-tab="known">Known</button>
    </div>
    <div id="dtab-threads" class="city-tab"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="dtab-known" class="city-tab" hidden></div>`;

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => { c.hidden = true; c.style.display = 'none'; });
      const el = document.getElementById('dtab-' + this.dataset.tab);
      if (el) { el.hidden = false; el.style.display = ''; }
      if (this.dataset.tab === 'known') return loadDipKnown();
    });
  });

  await loadDipThreads();
}

async function loadDipKnown() {
  const el = document.getElementById('dtab-known');
  if (!el || el.dataset.loaded) return;
  el.dataset.loaded = '1';
  el.innerHTML = '<div class="loading">Loading…</div>';
  try {
    const [cityResponse, rulerResponse] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/cities`),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/diplomacy`),
    ]);
    if (!cityResponse.ok || !rulerResponse.ok) throw new Error('directory unavailable');
    const [cities, rulers] = await Promise.all([cityResponse.json(), rulerResponse.json()]);
    const contacts = new Map(writableCities().map(p => [p.settlement_id, p]));
    const writeButton = id => id && State.MY_SETTLEMENT_ID
      ? `<button class="btn-small" data-write="${esc(id)}">Write</button>` : '';
    const cityRow = c => {
      const contact = !c.own && contacts.has(c.settlement_id);
      const status = c.own ? 'Your city' : contact ? 'Can write' : c.knowledge === 'rumour' ? 'Rumour only' : 'Known';
      const deposits = [c.copper_deposit ? 'Copper' : '', c.tin_deposit ? 'Tin' : '', c.silver_deposit ? 'Silver' : '', c.cedar_deposit ? 'Cedar' : ''].filter(Boolean).join(', ');
      const location = c.knowledge === 'known' && c.q != null && c.r != null
        ? `${fmtNum(c.q)}, ${fmtNum(c.r)}` : c.bearing ? c.bearing.replace(/^~(\d+) hexes /, (_, n) => `~${fmtNum(Number(n))} hexes `) : 'Location unknown';
      return `<div class="dsec"><div class="stat-row"><span class="sr-label">${esc(c.name)}</span><span class="sr-val">${contact ? writeButton(c.settlement_id) : ''}</span></div>
        <div>${esc(c.owner || 'Owner unknown')} · ${status}</div>
        <details><summary>Details</summary><div>${esc(location)}</div><div>${esc(deposits || c.industry_hint || '')}</div></details></div>`;
    };
    const grouped = new Set();
    el.innerHTML = rulers.map(r => {
      const owned = cities.filter(c => c.owner_id === r.owner_id);
      owned.forEach(c => grouped.add(c));
      const destination = owned.find(c => !c.own && contacts.has(c.settlement_id));
      const known = `${fmtNum(r.known_cities)} known ${r.known_cities === 1 ? 'city' : 'cities'}`;
      const rumoured = `${fmtNum(r.rumour_cities)} rumoured ${r.rumour_cities === 1 ? 'city' : 'cities'}`;
      return `<div class="dsec-title">${esc(r.owner)} ${writeButton(destination?.settlement_id)}</div><div>${known} · ${rumoured}</div>` + owned.map(cityRow).join('');
    }).join('') + cities.filter(c => !grouped.has(c)).map(cityRow).join('');
    if (!cities.length && !rulers.length) el.innerHTML = '<p class="empty-state">No cities or rulers known yet — explore the map.</p>';
    el.querySelectorAll('[data-write]').forEach(button => button.addEventListener('click', () => dipWrite(button.dataset.write)));
  } catch (_) {
    delete el.dataset.loaded;
    el.innerHTML = '<p class="empty-state">Could not load known cities and rulers. Open Known to retry.</p>';
  }
}

// Start the same inline composer used by existing correspondence, including
// its untouched buy/sell offer fields. A directory rumour cannot seed a draft.
export async function dipWrite(settlementID) {
  const destination = writableCities().find(p => p.settlement_id === settlementID);
  if (!destination || !State.MY_SETTLEMENT_ID) return;
  draftDestination = destination;
  const body = document.getElementById('diplomacy-body');
  body.querySelectorAll('.dtab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.tab === 'threads');
  });
  body.querySelectorAll('.city-tab').forEach(tab => {
    const show = tab.id === 'dtab-threads';
    tab.hidden = !show;
    tab.style.display = show ? '' : 'none';
  });
  document.getElementById('dtab-threads').innerHTML = '<div class="loading">Loading…</div>';
  await loadDipThreads();
}

// A re-render (a delayed post-send refresh, a new reply) rebuilds every thread.
// Whatever the player is mid-way through writing must survive it: draft text,
// trade fields, buy/sell choice, open trade details, focus and text selection.
function snapshotDrafts(root) {
  const snap = { controls: new Map(), radios: new Map(), details: [], focus: null };
  root.querySelectorAll('[id]').forEach(c => {
    if (c.matches('input:not([type=radio]), textarea, select')) snap.controls.set(c.id, c.value);
  });
  root.querySelectorAll('input[type=radio]').forEach(r => { if (r.checked) snap.radios.set(r.name, r.value); });
  root.querySelectorAll('.dip-thread').forEach(t => {
    t.querySelectorAll('details').forEach((d, i) => { if (d.open) snap.details.push(t.id + ':' + i); });
  });
  const a = document.activeElement;
  if (a && a.id && root.contains(a)) {
    snap.focus = { id: a.id, start: a.selectionStart ?? null, end: a.selectionEnd ?? null };
  }
  return snap;
}

function restoreDrafts(root, snap) {
  snap.controls.forEach((value, id) => {
    const c = document.getElementById(id);
    if (c && root.contains(c) && value !== '' ) c.value = value;
  });
  snap.radios.forEach((value, name) => {
    root.querySelectorAll('input[type=radio]').forEach(r => { if (r.name === name) r.checked = r.value === value; });
    if (name.endsWith('-kind')) dipToggleKind(name.slice(0, -5));
  });
  root.querySelectorAll('.dip-thread').forEach(t => {
    t.querySelectorAll('details').forEach((d, i) => { if (snap.details.includes(t.id + ':' + i)) d.open = true; });
  });
  if (snap.focus) {
    const f = document.getElementById(snap.focus.id);
    if (f && root.contains(f)) {
      f.focus();
      if (snap.focus.start != null && f.setSelectionRange) f.setSelectionRange(snap.focus.start, snap.focus.end);
    }
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
  const tradeableGoods = await getTradeableGoods();
  try {
    const [inR, outR] = await Promise.all([
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/inbox'),
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + (State.MY_SETTLEMENT_ID
        ? '/settlements/' + State.MY_SETTLEMENT_ID + '/messengers'
        : '/founding/messengers')),
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

    let draftKey = null;
    if (draftDestination && writableCities().some(p => p.settlement_id === draftDestination.settlement_id)) {
      draftKey = Object.keys(threads).find(key => threads[key].settlement_id === draftDestination.settlement_id);
      if (!draftKey) {
        draftKey = draftDestination.name;
        if (threads[draftKey]) draftKey += ' ' + draftDestination.settlement_id;
        threads[draftKey] = { name: draftDestination.name, settlement_id: draftDestination.settlement_id, messages: [] };
      }
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

    const drafts = snapshotDrafts(el);
    el.innerHTML = keys.map(key => {
      const thread = threads[key];
      const msgs = [...thread.messages].sort((a,b) => msgTime(a) - msgTime(b));
      const latest = msgs[msgs.length - 1] || {};
      const hasPendingTrade = msgs.some(m => m._dir === 'in' && m.trade_offer && m.trade_offer.status === 'pending');
      const hasUnread = msgs.some(m => m._dir === 'in' && m.status === 'delivered');

      const threadId = 'dip-thread-' + key.replace(/[^a-z0-9]/gi,'_');
      const isOpen = key === draftKey || openSet.has(threadId);
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
        +   (msgs.length ? fmtAgo(latest.arrived_at || latest.sent_at || latest.created_at) : 'New conversation')
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
            const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim)">' + expiryWord(m.expires_at) + '</div>' : '';
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
            tradeHtml = '<div style="font-size:.72rem;color:var(--text-dim);font-style:italic;margin:.2rem 0">Trade: ' + esc(offer.status) + esc(tradeScheduleText(offer)) + '</div>';
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
          // sjöhandel mellan spelare (megaron_plan_sjohandel_mellan_spelare.md
          // R3): stamped onto trade_offer at accept — empty for a land trade.
          const shipBit = m.trade_offer && m.trade_offer.ship_name
            ? ' · ⛵ by sea on ' + esc(m.trade_offer.ship_name)
            : '';
          const statusBit = offerStatus === 'accepted' ? '<span style="color:var(--safe)">✓ accepted' + esc(tradeScheduleText(m.trade_offer)) + shipBit + '</span>'
                          : offerStatus === 'declined' ? '<span style="color:var(--text-dim)">✗ declined</span>'
                          : offerStatus === 'expired'  ? '<span style="color:var(--text-dim)">⏳ expired</span>'
                          : sentStatusHTML(m);
          const replyText = m.reply_text
            ? '<div class="dip-msg-text" style="color:var(--safe);text-align:right">' + esc(m.reply_text) + '</div>'
            : '';
          let cancelBtn = '';
          if (m.trade_offer && m.trade_offer.status === 'pending') {
            const countdown = m.expires_at ? '<div style="font-size:.68rem;color:var(--text-dim);text-align:right">' + expiryWord(m.expires_at) + '</div>' : '';
            cancelBtn = countdown + '<div style="text-align:right;margin-top:.2rem"><button class="btn-small btn-danger" onclick="dipCancel(\'' + m.id + '\',this)">Cancel offer ✗</button></div>';
          }
          // Arrange passage (megaron_plan_ordna_passage.md, 3b-3): only shown
          // while the runner is actually waiting for one — the ship choices
          // are the server's own (ListSent's eligible_ships), never counted
          // client-side, so the button can never promise a ship the server
          // would refuse.
          let passageBtn = '';
          if (m.can_arrange_passage) {
            const ships = Array.isArray(m.eligible_ships) ? m.eligible_ships : [];
            const selId = 'dip-passage-ship-' + m.id;
            if (ships.length > 0) {
              const opts = ships.map(s => '<option value="' + esc(s.id) + '">' + esc(s.name) + ' (' + esc(s.settlement_name) + ')</option>').join('');
              passageBtn = '<div id="dip-passage-' + m.id + '" style="display:flex;gap:.3rem;margin-top:.3rem;align-items:center">'
                + '<select id="' + selId + '" style="flex:1;background:var(--warm-white);border:1px solid var(--border);padding:.2rem .3rem;font-size:.7rem">' + opts + '</select>'
                + '<button class="btn-small btn-primary" onclick="dipArrangePassage(\'' + m.id + '\',\'' + selId + '\',this)">Arrange passage</button>'
                + '</div>';
            } else {
              passageBtn = '<div style="font-size:.68rem;color:var(--text-dim);font-style:italic;margin-top:.3rem;text-align:right">no ship of yours is free there yet</div>';
            }
          }
          // Call back (megaron_plan_ordna_passage.md, 3b-4 R5): the other
          // choice for a runner stuck waiting for passage — bring it home
          // now, undelivered. Only ever offered for an outbound runner in
          // its OWN port (server-computed can_call_back, same rule as
          // can_arrange_passage: never a client-side guess).
          let callBackBtn = '';
          if (m.can_call_back) {
            callBackBtn = '<div style="text-align:right;margin-top:.3rem">'
              + '<button class="btn-small btn-danger" onclick="dipCallBack(\'' + m.id + '\',this)">Call it back</button>'
              + '</div>';
          }
          html += '<div class="dip-msg-row dip-msg-row-out">'
            + '<div class="dip-bubble dip-bubble-out">'
            + '<div class="dip-bubble-meta dip-bubble-meta-out">You → ' + esc(m.destination_name || 'unknown') + ' · ' + fmtAgo(m.sent_at || m.created_at) + '</div>'
            + '<div class="dip-msg-text">' + esc(m.message_text || m.message || '') + '</div>'
            + replyText
            + '<div style="font-size:.68rem;font-family:var(--mono);text-align:right">' + statusBit + '</div>'
            + cancelBtn + passageBtn + callBackBtn
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
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Want good</div><select id="' + cid + '-good"' + goodsSelectDisabledAttr(tradeableGoods) + '>' + goodsOptionsHTML(tradeableGoods) + '</select></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Quantity</div><input id="' + cid + '-qty" type="number" min="0.1" step="0.1" placeholder="50"></div>'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer silver</div><input id="' + cid + '-silver" type="number" min="1" step="1" placeholder="60"></div>'
          + '</div>'
          + '<div id="' + cid + '-sell-fields" style="display:none">'
          + '<div><div style="font-size:.68rem;color:var(--text-dim)">Offer good</div><select id="' + cid + '-offer-good"' + goodsSelectDisabledAttr(tradeableGoods) + '>' + goodsOptionsHTML(tradeableGoods) + '</select></div>'
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
    restoreDrafts(el, drafts);
    // Do not recreate an already writable textarea when capability hints arrive.
    el.insertAdjacentHTML('beforeend', await renderLockedActions('diplomacy'));

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
    if (body.trade_offer) track('trade_offer', { kind: body.trade_offer.kind });
    else track('messenger_sent');
    showStatus('✓ Dispatched · arrives ' + fmtArrival(data.arrives_at) + passageNote(data), true);
    if (textEl) textEl.value = '';
    fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers').then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    // Reload threads after a short delay so sent message appears
    setTimeout(() => loadDipThreads(), 1200);
  } else {
    showStatus(formatApiError(data, 'send failed'), false);
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
    showInlineResult(document.getElementById('dip-trade-' + id) || btn.parentElement, formatApiError(data, 'Cancel failed'), true);
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
    // sjöhandel mellan spelare (R3): the response names the initiator's own
    // ship when the trade sails — empty when it walks.
    const shipBit = data.ship_name ? ' · ⛵ by sea on ' + esc(data.ship_name) : '';
    const goodsETA = Number.isInteger(data.goods_arrival_tick) ? 'tick ' + data.goods_arrival_tick : arrivalHTML(data.goods_arrives_at);
    const silverETA = Number.isInteger(data.silver_arrival_tick) ? 'tick ' + data.silver_arrival_tick : arrivalHTML(data.silver_arrives_at);
    const direction = data.kind === 'sell' ? 'incoming' : 'outgoing';
    const payment = data.kind === 'sell' ? 'silver sent' : 'silver incoming';
    block.innerHTML = '<span style="color:var(--safe)">✓ Accepted — ' + data.quantity + ' ' + esc(data.good_key || '') + ' ' + direction + ' · goods arrive ' + goodsETA + ' · ' + data.silver_paid + ' ' + payment + ' · arrives ' + silverETA + shipBit + '</span>';
  } else {
    btn.disabled = false;
    if (block) {
      const err = document.createElement('div');
      err.style.cssText = 'color:var(--accent);font-size:.72rem;margin-top:.2rem';
      err.textContent = formatApiError(data, 'failed');
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
    row.outerHTML = '<div style="font-size:.72rem;color:var(--safe);margin-top:.2rem">✓ Reply dispatched · returns ' + arrivalHTML(data.returns_at) + passageNote(data) + '</div>';
  }
}

// dipArrangePassage is 3b-3's "Arrange passage" button: POST the chosen ship
// against the runner waiting for passage. res.ok drives the outcome — a
// refusal shows the server's own text, never a client-guessed reason.
export async function dipArrangePassage(id, selId, btn) {
  const sel = document.getElementById(selId);
  const shipId = sel ? sel.value : '';
  if (!shipId) return;
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/passage', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ ship_id: shipId }),
  });
  const data = await res.json().catch(() => ({}));
  const block = document.getElementById('dip-passage-' + id);
  if (res.ok) {
    if (block) block.outerHTML = '<div style="font-size:.72rem;color:var(--safe);margin-top:.3rem;text-align:right">✓ Passage arranged — arrives ' + arrivalHTML(data.arrives_at) + '</div>';
  } else {
    btn.disabled = false;
    showInlineResult(block || btn.parentElement, formatApiError(data, 'Arrange passage failed'), true);
  }
}

// dipCallBack is 3b-4's "Call it back" button (R5, megaron_plan_ordna_
// passage.md): bring a runner stuck waiting for passage in its own port
// straight home, undelivered. res.ok drives the outcome — a refusal shows
// the server's own text, never a client-guessed reason.
export async function dipCallBack(id, btn) {
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/call-back', {
    method: 'POST',
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    const row = btn.closest('div');
    if (row) row.outerHTML = '<div style="font-size:.72rem;color:var(--text-dim);margin-top:.3rem;text-align:right">↩ Called back — returns ' + arrivalHTML(data.returns_at) + ', undelivered</div>';
  } else {
    btn.disabled = false;
    showInlineResult(btn.parentElement, formatApiError(data, 'Call back failed'), true);
  }
}

// A timestamp passing is not proof of delivery: jobs may be delayed or cargo
// intercepted. These are scheduled ticks, as persisted by the server.
export function tradeScheduleText(offer) {
  if (offer?.status !== 'accepted') return '';
  const legs = [];
  if (Number.isInteger(offer.goods_arrival_tick)) legs.push(`goods scheduled tick ${offer.goods_arrival_tick}`);
  if (Number.isInteger(offer.silver_arrival_tick)) legs.push(`silver scheduled tick ${offer.silver_arrival_tick}`);
  return legs.length ? ' · ' + legs.join(' · ') : '';
}
