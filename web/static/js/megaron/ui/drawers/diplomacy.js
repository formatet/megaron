import { readDiplomacyLetters } from '../diplomacy_reads.js';
import { showInlineResult } from '../inline_result.js';
import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc, fmtAgo, formatApiError, passageNote } from '../format.js';
import { fmtArrival, arrivalHTML } from '../time.js';
import { fmtTickTime } from '../fmt_clock.js';
import { serverNow } from '../../clock.js';
import { loadDipRunners } from './diplomacy_runners.js';

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

let currentPerson = null;
let draftDestination = null;
let requestGeneration = 0;
const drafts = new Map();
const draftKey = () => State.WORLD_ID+':'+State.MY_PLAYER_ID+':'+(currentPerson || draftDestination?.settlement_id);
const lettersRoot = () => document.getElementById('dtab-letters');
const writableCities = () => State.provinceData.filter(p => !p.own && !p.is_outpost && p.settlement_id && p.name);
const personName = p => p.owner || p.owner_name || 'Wanax';

export function lettersState(state) {
  return { waiting_you: 'Waiting on you', waiting_them: 'Waiting on them', done: 'Done' }[state] || 'Done';
}
export function lettersListHTML(page) {
  return (page.items || []).map(p => `<button class="letter-person" data-person="${esc(p.counterpart_id)}"><strong>${esc(p.counterpart_name)}</strong><span class="letter-person-state">${lettersState(p.state)}${p.reply_before ? ' · before ' + esc(fmtTickTime(undefined,p.reply_before)) : ''}</span><span class="letter-person-preview">${esc(p.preview || '')}</span></button>`).join('') || '<p class="empty-state">No letters yet.</p>';
}
export function letterActions(m) {
  const actions = Array.isArray(m.actions) ? m.actions : [];
  return actions.filter(a => !(a === 'reply' && (m.reply_window_closed || (m.reply_before && Date.parse(m.reply_before) <= serverNow()))) && !(['trade_accept','trade_decline'].includes(a) && m.expires_at && Date.parse(m.expires_at) <= serverNow()));
}
export function offerLine(m) {
  const o=m.trade_offer;if(!o)return '';
  const terms=o.kind==='sell' ? `${String(o.offer_qty)} ${o.offer_good} for ${String(o.want_silver)} silver` : `${String(o.want_qty)} ${o.want_good} for ${String(o.offer_silver)} silver`;
  const state=m.offer_state || o.status;
  return (['accepted','returned'].includes(state) ? 'Agreed: ' : state==='pending' ? 'Offer: ' : state==='expired' ? 'Expired without answer: ' : `${state || 'Trade'}: `)+terms;
}
export function lettersThreadHTML(page) {
  const rows=page.items || [];
  return rows.map((m,i)=>{
    const actions=letterActions(m);
    const controls=actions.includes('trade_accept') || actions.includes('trade_decline') || actions.includes('trade_cancel') ? `<div id="dip-trade-${esc(m.id)}">${actions.includes('trade_accept') ? `<button class="btn-small" onclick="dipAccept('${esc(m.id)}',this)">Accept</button>` : ''}${actions.includes('trade_decline') ? `<button class="btn-small" onclick="dipDecline('${esc(m.id)}',this)">Decline</button>` : ''}${actions.includes('trade_cancel') ? `<button class="btn-small" onclick="dipCancel('${esc(m.id)}',this)">Cancel offer</button>` : ''}</div>` : '';
    return `<details class="letter-entry" data-letter="${esc(m.id)}"${i===rows.length-1?' open':''}><summary>${esc(m.direction==='outgoing'?'You':m.sender_name || page.counterpart_name)} · ${fmtAgo(m.sent_at)}${m.kind==='gift'?' · Gift':''}</summary><p class="dip-msg-text">${esc(m.message || '')}${m.kind==='gift'&&m.goods ? ': '+esc(Object.entries(m.goods).map(([good,qty])=>String(qty)+' '+good).join(', ')) : ''}</p>${m.reply_text ? `<p class="dip-msg-text">${m.direction==='incoming'?'Your reply':'Reply'}: ${esc(m.reply_text)}</p>` : ''}${m.trade_offer ? `<p>${esc(offerLine(m))}</p>` : ''}${m.missing_payment ? `<p>${esc(m.missing_payment)}</p>` : ''}${controls}</details>`;
  }).join('');
}
function rememberDraft() {
  const el=document.getElementById('letter-compose');if(!el)return;
  const saved={};el.querySelectorAll('input,textarea,select').forEach(c=>{if(c.id)saved[c.id]=c.type==='radio'?c.checked:c.value;});
  drafts.set(draftKey(),saved);
}
function restoreDraft() {
  const saved=drafts.get(draftKey());if(!saved)return;
  for(const [id,value] of Object.entries(saved)){const c=document.getElementById(id);if(c){if(c.type==='radio')c.checked=value;else c.value=value;}}
  dipToggleKind('letter-compose');
}
function toolbar(title='') { return `<div class="letters-toolbar"><button class="btn-small" data-letters-back>Letters</button><strong>${esc(title)}</strong><button class="btn-small" data-write-list>Write</button></div>`; }
function bindToolbar(el) {
  el.querySelector('[data-write-list]')?.addEventListener('click',showWriteList);
  el.querySelector('[data-letters-back]')?.addEventListener('click',()=>{rememberDraft();currentPerson=null;draftDestination=null;return loadDipThreads();});
}
// A dispatch's "Reply" opens the drawer on the thread with that Wanax.
let pendingPerson = null;
export function openLettersWith(person) {
  pendingPerson = person;
  if (window.openDrawer) window.openDrawer('diplomacy');
}
export async function loadDiplomacyDrawer() {
  rememberDraft();currentPerson=pendingPerson;pendingPerson=null;draftDestination=null;
  const body=document.getElementById('diplomacy-body');
  body.innerHTML='<div class="drawer-tabs"><button class="dtab active" data-tab="letters">Letters</button><button class="dtab" data-tab="runners">Runners</button></div><div id="dtab-letters" class="city-tab"></div><div id="dtab-runners" class="city-tab" hidden></div>';
  body.querySelectorAll('.dtab').forEach(tab=>tab.addEventListener('click',()=>switchDiplomacyTab(tab.dataset.tab)));
  await loadDipThreads();
}
export async function switchDiplomacyTab(name) {
  rememberDraft();
  if(name==='threads')name='letters';
  const body=document.getElementById('diplomacy-body');const el=document.getElementById('dtab-'+name);if(!body||!el)return;
  body.querySelectorAll('.dtab').forEach(t=>t.classList.toggle('active',t.dataset.tab===name));
  body.querySelectorAll('.city-tab').forEach(t=>{t.hidden=t!==el;t.style.display=t===el?'':'none';});
  return name==='runners'?loadDipRunners():loadDipThreads();
}
export async function refreshDiplomacyLetters() {
  const el=lettersRoot();if(!el)return;
  if(el.hidden) { const runners=document.getElementById('dtab-runners'); if(runners&&!runners.hidden)return loadDipRunners(); return; }
  rememberDraft();return loadDipThreads();
}
async function showWriteList() {
  rememberDraft();currentPerson=null;draftDestination=null;
  const el=lettersRoot();if(!el)return;
  const generation=++requestGeneration,world=State.WORLD_ID;
  el.innerHTML='<div class="loading">Loading…</div>';
  try {
    const r=await fetchAuth(`/api/v1/worlds/${world}/cities`);if(!r.ok)throw Error();const cities=await r.json();
    if(generation!==requestGeneration||world!==State.WORLD_ID||lettersRoot()!==el)return;
    const contacts=new Set(writableCities().map(p=>p.settlement_id));const people=new Map();
    for(const c of cities){if(c.own||!contacts.has(c.settlement_id))continue;const key=c.owner_id || c.owner;if(!people.has(key))people.set(key,{name:c.owner,cities:[]});people.get(key).cities.push(c);}
    el.innerHTML=toolbar('Write')+[...people.values()].map(p=>`<section class="dsec"><strong>${esc(p.name)}</strong>${p.cities.map(c=>`<div class="stat-row"><span>${esc(c.name)}</span><button class="btn-small" data-write="${esc(c.settlement_id)}">Write</button></div>`).join('')}</section>`).join('')+(people.size?'':'<p class="empty-state">No Wanax you can reach yet.</p>');
    bindToolbar(el);el.querySelectorAll('[data-write]').forEach(b=>b.addEventListener('click',()=>dipWrite(b.dataset.write)));
  }catch(_){if(generation===requestGeneration)el.innerHTML=toolbar('Write')+'<p class="empty-state">Could not load contacts.</p>';bindToolbar(el);}
}
export async function dipWrite(id) {
  const p=writableCities().find(p=>p.settlement_id===id);if(!p||!State.MY_SETTLEMENT_ID)return;
  rememberDraft();draftDestination=p;currentPerson=null;
  const body=document.getElementById('diplomacy-body');
  body.querySelectorAll('.dtab').forEach(t=>t.classList.toggle('active',t.dataset.tab==='letters'));
  body.querySelectorAll('.city-tab').forEach(t=>{t.hidden=t.id!=='dtab-letters';t.style.display=t.hidden?'none':'';});
  await loadDipThreads();
}
async function openLetters(id) {
  rememberDraft();draftDestination=null;currentPerson=id;return loadDipThreads();
}
function composerHTML(reply,destination,goods) {
  if(reply)return `<div class="dip-inline-compose" id="letter-compose"><div id="dip-reply-row-${esc(reply.id)}"><textarea id="dip-reply-${esc(reply.id)}" maxlength="1000" aria-label="Reply"></textarea><button class="btn-small btn-primary" onclick="dipReply('${esc(reply.id)}')">Send with their runner${reply.reply_before ? ' (leaves '+esc(fmtTickTime(undefined,reply.reply_before))+')' : ''}</button></div></div>`;
  if(!destination||!State.MY_SETTLEMENT_ID)return '';
  return `<div class="dip-inline-compose" id="letter-compose"><textarea id="letter-compose-text" maxlength="1000" aria-label="New letter" placeholder="Write a letter…"></textarea><details><summary>Attach trade offer</summary><div class="dip-inline-trade"><label><input id="letter-compose-buy" type="radio" name="letter-compose-kind" value="buy" checked onchange="dipToggleKind('letter-compose')"> Buy</label><label><input id="letter-compose-sell" type="radio" name="letter-compose-kind" value="sell" onchange="dipToggleKind('letter-compose')"> Sell</label><div id="letter-compose-buy-fields"><label>Want good<select id="letter-compose-good"${goodsSelectDisabledAttr(goods)}>${goodsOptionsHTML(goods)}</select></label><label>Quantity<input id="letter-compose-qty" type="number" min="0.1" step="0.1"></label><label>Offer silver<input id="letter-compose-silver" type="number" min="1"></label></div><div id="letter-compose-sell-fields" hidden><label>Offer good<select id="letter-compose-offer-good"${goodsSelectDisabledAttr(goods)}>${goodsOptionsHTML(goods)}</select></label><label>Quantity<input id="letter-compose-offer-qty" type="number" min="0.1" step="0.1"></label><label>Want silver<input id="letter-compose-want-silver" type="number" min="1"></label></div></div></details><div class="dip-inline-compose-row"><button class="btn-small btn-primary" onclick="dipSendInThread('letter-compose','${esc(destination.settlement_id)}')">Send a runner</button><span id="letter-compose-status" role="status"></span></div></div>`;
}
async function loadDipThreads() {
  const el=lettersRoot();if(!el)return;
  const generation=++requestGeneration,world=State.WORLD_ID;
  const person=currentPerson,destination=draftDestination;
  const alive=()=>generation===requestGeneration&&State.WORLD_ID===world&&lettersRoot()===el;
  el.innerHTML='<div class="loading">Loading…</div>';
  if(destination){const goods=await getTradeableGoods();if(!alive())return;el.innerHTML=toolbar(personName(destination))+composerHTML(null,destination,goods);bindToolbar(el);restoreDraft();return;}
  try {
    const response=await fetchAuth(`/api/v1/worlds/${world}/letters${person?'/'+encodeURIComponent(person):''}`);if(!response.ok)throw Error();const page=await response.json();if(!Array.isArray(page.items))throw Error();if(!alive())return;
    if(!person){el.innerHTML='<div class="letters-toolbar"><strong>Letters</strong><button class="btn-small" data-write-list>Write</button></div>'+lettersListHTML(page);bindToolbar(el);el.querySelectorAll('[data-person]').forEach(b=>b.addEventListener('click',()=>openLetters(b.dataset.person)));return;}
    const reply=[...page.items].reverse().find(m=>m.direction==='incoming'&&letterActions(m).includes('reply'));
    const city=writableCities().find(p=>p.owner_id===person || personName(p)===page.counterpart_name);
    const goods=reply||!city?[]:await getTradeableGoods();if(!alive())return;
    el.innerHTML=toolbar(page.counterpart_name)+lettersThreadHTML(page)+composerHTML(reply,city,goods);bindToolbar(el);restoreDraft();
    const mark=details=>{if(details.open){const m=page.items.find(m=>m.id===details.dataset.letter);if(m?.direction==='incoming'&&m.kind!=='gift'&&!m.read_at)readDiplomacyLetters([m.id]);}};
    el.querySelectorAll('[data-letter]').forEach(d=>{d.addEventListener('toggle',()=>mark(d));mark(d);});
  }catch(_){if(alive()){el.innerHTML=toolbar()+'<p class="empty-state">Could not load letters.</p>';bindToolbar(el);}}
}

export function dipToggleKind(cid) {
  const kind = document.querySelector('input[name="' + cid + '-kind"]:checked')?.value || 'buy';
  const buyEl = document.getElementById(cid + '-buy-fields');
  const sellEl = document.getElementById(cid + '-sell-fields');
  if (buyEl) { buyEl.hidden = kind !== 'buy'; buyEl.style.display = kind === 'buy' ? '' : 'none'; }
  if (sellEl) { sellEl.hidden = kind !== 'sell'; sellEl.style.display = kind === 'sell' ? '' : 'none'; }
}

export function dipToggleThread(id) { return openLetters(id.replace(/^dip-thread-/, '')); }

export async function dipSendInThread(cid, destId) {
  const sentDraftKey = draftKey();
  const textEl   = document.getElementById(cid + '-text');
  const statusEl = document.getElementById(cid + '-status');
  const text = textEl ? textEl.value.trim() : '';
  function showStatus(msg, ok) {
    if (statusEl) { statusEl.style.color = ok ? 'var(--safe-text)' : 'var(--accent)'; statusEl.textContent = msg; }
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
    showStatus('✓ Dispatched · arrives ' + fmtArrival(data.arrives_at) + passageNote(data), true);
    if (textEl) textEl.value = '';
    drafts.delete(sentDraftKey);
    fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers').then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    // Reload threads after a short delay so sent message appears
    setTimeout(() => refreshDiplomacyLetters(), 1200);
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
    rememberDraft(); await loadDipThreads();
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
    const goodsETA = data.route_version===2 ? arrivalHTML(data.goods_arrives_at) : Number.isInteger(data.goods_arrival_tick) ? fmtTickTime(data.goods_arrival_tick) : arrivalHTML(data.goods_arrives_at);
    const silverETA = data.route_version===2 ? arrivalHTML(data.silver_arrives_at) : Number.isInteger(data.silver_arrival_tick) ? fmtTickTime(data.silver_arrival_tick) : arrivalHTML(data.silver_arrives_at);
    const direction = data.kind === 'sell' ? 'incoming' : 'outgoing';
    const payment = data.kind === 'sell' ? 'silver sent' : 'silver incoming';
    block.innerHTML = '<span style="color:var(--safe-text)">✓ Accepted — ' + data.quantity + ' ' + esc(data.good_key || '') + ' ' + direction + ' · goods arrive ' + goodsETA + ' · ' + data.silver_paid + ' ' + payment + ' · arrives ' + silverETA + shipBit + '</span>';
    if (lettersRoot()) { rememberDraft(); await loadDipThreads(); }
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
  const sentDraftKey = draftKey();
  const input = document.getElementById('dip-reply-' + id);
  const text = input ? input.value.trim() : '';
  if (!text) return;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/reply', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ reply: text }),
  });
  const data = await res.json().catch(() => ({}));
  const row = document.getElementById('dip-reply-row-' + id);
  if (res.ok && row) {
    if (input) input.value = '';
    drafts.delete(sentDraftKey);
    row.outerHTML = '<div style="font-size:.72rem;color:var(--safe-text);margin-top:.2rem">✓ Reply dispatched · returns ' + arrivalHTML(data.returns_at) + passageNote(data) + '</div>';
  } else if (row) {
    showInlineResult(row, formatApiError(data, 'Reply failed'), true);
  }
}

// dipArrangePassage is 3b-3's "Arrange passage" button: POST the chosen ship
// against the runner waiting for passage. res.ok drives the outcome — a
// refusal shows the server's own text, never a client-guessed reason.
export async function dipArrangePassage(id, selId, btn, blockId = 'dip-passage-' + id) {
  const sel = document.getElementById(selId);
  const shipId = sel ? sel.value : '';
  if (!shipId) return;
  btn.disabled = true;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/messengers/' + id + '/passage', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ ship_id: shipId }),
  });
  const data = await res.json().catch(() => ({}));
  const block = document.getElementById(blockId);
  if (res.ok) {
    if (block) block.outerHTML = '<div style="font-size:.72rem;color:var(--safe-text);margin-top:.3rem;text-align:right">✓ Passage arranged — arrives ' + arrivalHTML(data.arrives_at) + '</div>';
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
  if (Number.isInteger(offer.goods_arrival_tick)) legs.push(`goods scheduled ${fmtTickTime(offer.route_version===2?null:offer.goods_arrival_tick,offer.goods_arrives_at)}`);
  if (Number.isInteger(offer.silver_arrival_tick)) legs.push(`silver scheduled ${fmtTickTime(offer.route_version===2?null:offer.silver_arrival_tick,offer.silver_arrives_at)}`);
  return legs.length ? ' · ' + legs.join(' · ') : '';
}
