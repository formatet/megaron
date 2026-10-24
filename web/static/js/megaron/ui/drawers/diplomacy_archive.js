import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc, formatApiError } from '../format.js';
import { fmtNum } from '../fmt_num.js';

export function caseListHTML(page) {
  return (page.items || []).map(c => `<article class="dip-queue-item" data-case="${esc(c.id)}"><button class="btn-small" data-case-open="${esc(c.id)}">${esc(c.subject || 'Correspondence')}</button><div><strong>${esc(c.counterpart_name)}</strong> · ${esc(c.city_name)}</div><p class="dip-queue-preview">${esc(c.preview || '')}</p><div class="dip-queue-note">${esc(String(c.letter_count))} letters · ${esc(new Date(c.latest_at).toLocaleString())}</div></article>`).join('') || '<p class="empty-state">No personal cases on this page.</p>';
}

function archiveOffer(offer) {
  if (!offer) return '';
  const terms = offer.kind === 'sell'
    ? `${fmtNum(offer.offer_qty)} ${esc(offer.offer_good)} for ${fmtNum(offer.want_silver)} silver`
    : `${fmtNum(offer.want_qty)} ${esc(offer.want_good)} for ${fmtNum(offer.offer_silver)} silver`;
  const status = {pending:'Awaiting answer',accepted:'Accepted',declined:'Declined',expired:'Expired without answer',cancelled:'Cancelled'}[offer.status] || 'Outcome unavailable';
  return `<p class="dip-queue-message">Recorded offer: ${terms} · ${status}</p>`;
}

export function caseDetailHTML(detail) {
  return `<h3 class="dip-archive-heading">${esc(detail.subject || 'Correspondence')}</h3><p class="dip-queue-note">Names are preserved as recorded when each letter was sent. Opening the archive does not mark letters read.</p>` + (detail.items || []).map((m, index) => `<article class="dip-queue-item" data-archive-letter="${esc(m.id)}"><div><strong>${esc(m.sender_name)}</strong> · ${esc(m.origin_name)} → ${esc(m.recipient_name)} · ${esc(m.destination_name)}</div><div class="dip-queue-note">${esc(new Date(m.sent_at).toLocaleString())}</div><details${index === 0 ? ' open' : ''}><summary>Letter</summary><p class="dip-queue-message">${esc(m.message_text)}</p>${m.reply_text ? `<p class="dip-queue-message">Reply: ${esc(m.reply_text)}</p>` : ''}${archiveOffer(m.trade_offer)}</details></article>`).join('');
}

export function caseDestinations(detail) {
  const historic = new Set((detail.items || []).flatMap(m => [m.origin_id, m.destination_id]).filter(Boolean));
  return State.provinceData.filter(p => !p.own && p.settlement_id && p.name && historic.has(p.settlement_id));
}

function caseComposeHTML(detail) {
  const destinations = caseDestinations(detail);
  if (!State.MY_SETTLEMENT_ID || !destinations.length) return '<p class="dip-queue-note">A new letter needs one of this case’s cities in your current contacts. Same-bearer answers remain in Queue.</p>';
  return `<form class="dip-case-compose" data-case-compose><label>Continue this case · destination<select name="destination">${destinations.map(p => `<option value="${esc(p.settlement_id)}">${esc(p.name)}</option>`).join('')}</select></label><label>Letter<textarea name="message" required maxlength="1000"></textarea></label><button class="btn-small">Dispatch new letter</button><div role="status"></div></form>`;
}

const mounts = new WeakMap();
export async function loadDipArchive(root = document.getElementById('dtab-archive')) {
  if (!root) return;
  const world = State.WORLD_ID, mount = Symbol('archive');
  mounts.set(root, mount);
  let generation = 0, query = '', cursor = '', selected = '';
  const alive = n => n === generation && State.WORLD_ID === world && mounts.get(root) === mount && document.getElementById('dtab-archive') === root;
  async function load(caseID = '', next = '') {
    const n = ++generation;
    root.innerHTML = '<div class="loading">Loading personal archive…</div>';
    try {
      const params = new URLSearchParams({ limit: '30' });
      if (next) params.set('cursor', next);
      if (!caseID && query) params.set('q', query);
      const response = await fetchAuth(`/api/v1/worlds/${world}/correspondence/cases${caseID ? '/' + encodeURIComponent(caseID) : ''}?${params}`, { sensitive: true });
      const data = await response.json();
      if (!response.ok) throw new Error(formatApiError(data, 'Could not load personal archive.'));
      if (!alive(n)) return;
      if (!Array.isArray(data.items)) throw new Error('Invalid archive response.');
      selected = caseID; cursor = next; root.dataset.loaded = '1';
      root.innerHTML = '<div class="dip-queue-toolbar">' + (caseID ? '<button class="btn-small" data-archive-back>Back to cases</button>' : '<form class="dip-archive-search" data-archive-search><label>Find person, city, subject or good <input name="q" maxlength="120" value="' + esc(query) + '"></label><button class="btn-small">Search</button></form>') + '<button class="btn-small" data-archive-refresh>Refresh</button></div><p class="dip-queue-note">Personal archive of newly sent correspondence. Older letters have no proven historical recipient and remain in the legacy correspondence view.</p>' + (caseID ? caseDetailHTML(data) + caseComposeHTML(data) : caseListHTML(data)) + (data.next_cursor ? '<button class="btn-small" data-archive-next>Older →</button>' : '');
      root.querySelector('[data-archive-search]')?.addEventListener('submit', e => { e.preventDefault(); query = e.currentTarget.elements.q.value.trim(); return load(); });
      root.querySelector('[data-archive-back]')?.addEventListener('click', () => load());
      root.querySelector('[data-archive-refresh]')?.addEventListener('click', () => load(caseID));
      root.querySelector('[data-archive-next]')?.addEventListener('click', () => load(caseID, data.next_cursor));
      root.querySelectorAll('[data-case-open]').forEach(button => button.addEventListener('click', () => load(button.dataset.caseOpen)));
      root.querySelector('[data-case-compose]')?.addEventListener('submit', async e => {
        e.preventDefault();
        if (!alive(n)) return;
        const form = e.currentTarget, text = form.elements.message.value.trim();
        const status = form.querySelector('[role="status"]'), button = form.querySelector('button');
        if (!text) { status.textContent = 'Write a letter first.'; return; }
        const origin = State.MY_SETTLEMENT_ID, destination = form.elements.destination.value;
        if (!origin || !caseDestinations(data).some(p => p.settlement_id === destination)) { status.textContent = 'This city is no longer a current contact.'; return; }
        button.disabled = true;
        try {
          const response = await fetchAuth(`/api/v1/worlds/${world}/settlements/${origin}/messengers`, {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({destination_id:destination,message:text,case_id:caseID}), sensitive:true});
          const result = await response.json().catch(() => ({}));
          if (!alive(n)) return;
          if (!response.ok) throw new Error(formatApiError(result,'Could not dispatch letter.'));
          form.elements.message.value = '';
          status.textContent = 'Letter dispatched in this case. Refresh to see it in the archive.';
        } catch (error) { if (alive(n)) status.textContent = error.message; }
        finally { if (alive(n)) button.disabled = false; }
      });
    } catch (error) {
      if (!alive(n)) return;
      root.innerHTML = `<p class="empty-state">${esc(error.message || 'Could not load personal archive.')}</p><button class="btn-small" data-archive-retry>Retry</button>`;
      root.querySelector('[data-archive-retry]')?.addEventListener('click', () => load(selected, cursor));
    }
  }
  await load();
}
