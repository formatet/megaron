import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { serverNow } from '../../clock.js';
import { esc, formatApiError } from '../format.js';
import { fmtTickTime } from '../fmt_clock.js';
import { fmtNum } from '../fmt_num.js';
import { readDiplomacyLetters } from '../diplomacy_reads.js';

export function queueActions(item) {
  const supplied = Array.isArray(item.actions) ? item.actions : [];
  const actions = item.reply_window_closed ? supplied.filter(a => a !== 'reply') : supplied;
  // A page may stay open across its deadline. Never offer a late response;
  // the server rechecks the same deadline atomically when an action is sent.
  const elapsed = item.offer_state === 'pending' && item.expires_at && Date.parse(item.expires_at) <= serverNow();
  return elapsed ? actions.filter(a => a !== 'trade_accept' && a !== 'trade_decline') : actions;
}

export function queueState(item) {
  if (!item.trade_offer && item.reply_text) return 'Reply dispatched';
  if (item.reply_window_closed) return 'Return post closed';
  if (item.offer_state === 'expired' || (item.offer_state === 'pending' && item.expires_at && Date.parse(item.expires_at) <= serverNow())) return 'Expired without answer';
  const states = { pending: 'Awaiting your answer', accepted: 'Accepted', declined: 'Declined', cancelled: 'Cancelled' };
  return states[item.offer_state] || (item.queue_group === 'waiting_you' ? 'Awaiting your reply' : 'No reply pending');
}

function offerText(offer) {
  if (!offer) return '';
  return offer.kind === 'sell'
    ? `Buy ${fmtNum(offer.offer_qty)} ${esc(offer.offer_good)} · pay ${fmtNum(offer.want_silver)} silver`
    : `Sell ${fmtNum(offer.want_qty)} ${esc(offer.want_good)} · receive ${fmtNum(offer.offer_silver)} silver`;
}

export function queuePageHTML(page) {
  const items = page.items || [];
  const sections = [ ['waiting_you', 'Waiting on you'], ['completed', 'Past correspondence'] ];
  return '<p class="dip-queue-note">Incoming letters, newest first. Groups apply to this page; older unanswered letters may be on the next page.</p>'
    + sections.map(([group, title]) => {
      const rows = items.filter(m => m.queue_group === group);
      if (!rows.length) return '';
      return `<section><h3 class="dsec-title">${title}</h3>` + rows.map(m => {
        const actions = queueActions(m);
        const replyDeadline = m.reply_before || Number.isInteger(m.reply_deadline_tick)
          ? `<div>Return post before ${esc(fmtTickTime(m.reply_deadline_tick, m.reply_before))}</div>`
          : actions.includes('reply') ? '<div>Return post deadline unavailable</div>' : '';
        const deadline = m.expires_at ? `<div>Answer deadline: <time datetime="${esc(m.expires_at)}" title="${esc(new Date(m.expires_at).toLocaleString())}">${esc(fmtTickTime(undefined, m.expires_at))}</time></div>` : '';
        return `<article class="dip-queue-item" data-letter="${esc(m.id)}"><div class="dip-queue-heading"><strong>${esc(m.from_name || 'Unknown origin')}</strong><span>${esc(queueState(m))}</span></div>
          <div class="dip-queue-note">To ${esc(m.to_name || 'your city')}${m.read_at ? ' · Read' : ' · Unread'}</div>
          ${m.message ? `<p class="dip-queue-preview">${esc(m.message.slice(0, 100))}${m.message.length > 100 ? '…' : ''}</p>` : ''}
          ${m.trade_offer ? `<div>${offerText(m.trade_offer)}</div>` : ''}${deadline}${replyDeadline}
          ${m.missing_payment ? `<p class="dip-queue-payment">${esc(m.missing_payment)}.${actions.includes('trade_decline') ? ' You can still decline.' : ''}</p>` : ''}
          <details data-open-letter="${esc(m.id)}"><summary>Read letter</summary><p class="dip-queue-message">${esc(m.message || '(No letter text)')}</p>${m.reply_text ? `<p class="dip-queue-message">Your reply: ${esc(m.reply_text)}</p>` : ''}
          ${actions.includes('reply') ? `<label>Reply<textarea maxlength="1000" data-queue-reply aria-label="Reply to ${esc(m.from_name || 'letter')}"></textarea></label><button class="btn-small" data-queue-action="reply">Dispatch reply</button>` : ''}</details>
          <div class="dip-queue-actions">${actions.includes('trade_accept') ? '<button class="btn-small" data-queue-action="trade_accept">Accept</button>' : ''}${actions.includes('trade_decline') ? '<button class="btn-small" data-queue-action="trade_decline">Decline</button>' : ''}</div>
          <div class="dip-queue-result" role="status"></div></article>`;
      }).join('') + '</section>';
    }).join('') + (items.length ? '' : '<p class="empty-state">No received correspondence on this page.</p>');
}

const mountedQueues = new WeakMap();

// Each mounted drawer owns its page and request generation. Reopening the
// drawer or switching worlds makes late fetch responses harmless.
export async function loadDipQueue(root = document.getElementById('dtab-queue')) {
  if (!root) return;
  const world = State.WORLD_ID;
  const mount = Symbol('queue mount');
  mountedQueues.set(root, mount);
  let cursor = '';
  let generation = 0;
  let current = null;
  const alive = () => State.WORLD_ID === world && document.getElementById('dtab-queue') === root && mountedQueues.get(root) === mount;
  async function load(next = '') {
    const request = ++generation;
    root.innerHTML = '<div class="loading">Loading correspondence…</div>';
    try {
      const response = await fetchAuth(`/api/v1/worlds/${world}/messengers/queue?limit=30${next ? '&cursor=' + encodeURIComponent(next) : ''}`);
      if (!response.ok) throw new Error('Could not load correspondence.');
      const page = await response.json();
      if (!Array.isArray(page.items)) throw new Error('Could not load correspondence.');
      if (!alive() || request !== generation) return;
      cursor = next;
      current = page;
      root.dataset.loaded = '1';
      root.innerHTML = '<div class="dip-queue-toolbar"><button class="btn-small" data-queue-refresh>Refresh from newest</button>'
        + (page.next_cursor ? '<button class="btn-small" data-queue-next>Older correspondence →</button>' : '') + '</div>' + queuePageHTML(page);
      root.querySelector('[data-queue-refresh]')?.addEventListener('click', () => load());
      root.querySelector('[data-queue-next]')?.addEventListener('click', () => load(page.next_cursor));
      root.querySelectorAll('[data-open-letter]').forEach(details => details.addEventListener('toggle', async () => {
        const item = page.items.find(m => m.id === details.dataset.openLetter);
        if (!details.open || !item || !queueActions(item).includes('read')) return;
        const read = await readDiplomacyLetters([item.id]);
        if (!alive() || request !== generation) return;
        if (read) {
          item.read_at = true;
          details.closest('[data-letter]').querySelector('.dip-queue-note').textContent = `To ${item.to_name || 'your city'} · Read`;
        } else details.closest('[data-letter]').querySelector('[role="status"]').textContent = 'Letter opened, but could not mark it read. Close and reopen to retry.';
      }));
      root.querySelectorAll('[data-queue-action]').forEach(button => button.addEventListener('click', () => act(button, request)));
    } catch (error) {
      if (!alive() || request !== generation) return;
      root.innerHTML = '<p class="empty-state">Could not load correspondence.</p><button class="btn-small" data-queue-retry>Retry</button>';
      root.querySelector('[data-queue-retry]')?.addEventListener('click', () => load(next));
    }
  }
  async function act(button, request) {
    const article = button.closest('[data-letter]');
    const item = current?.items.find(m => m.id === article.dataset.letter);
    const action = button.dataset.queueAction;
    const result = article.querySelector('[role="status"]');
    if (!item || !queueActions(item).includes(action)) { result.textContent = 'This response is no longer available. Refresh correspondence.'; return; }
    const reply = article.querySelector('[data-queue-reply]')?.value.trim();
    if (action === 'reply' && !reply) { result.textContent = 'Write a reply first.'; return; }
    article.querySelectorAll('button').forEach(b => { b.disabled = true; });
    try {
      const endpoint = { trade_accept: 'trade-accept', trade_decline: 'trade-decline', reply: 'reply' }[action];
      const response = await fetchAuth(`/api/v1/worlds/${world}/messengers/${encodeURIComponent(item.id)}/${endpoint}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(action === 'reply' ? { reply } : {}),
      });
      const data = await response.json().catch(() => ({}));
      if (!alive() || request !== generation) return;
      if (!response.ok) throw new Error(formatApiError(data, 'Response failed.'));
      // Keep the sent reply's success visible, then let an explicit refresh
      // obtain live groups rather than overwriting another draft in this page.
      article.querySelector('.dip-queue-actions').innerHTML = '';
      article.querySelectorAll('[data-queue-action], [data-queue-reply]').forEach(c => { c.disabled = true; });
      const heading = article.querySelector('.dip-queue-heading span');
      if (heading) heading.textContent = action === 'reply' ? 'Reply dispatched' : action === 'trade_accept' ? 'Accepted' : 'Declined';
      item.actions = item.actions.filter(a => a === 'read');
      result.textContent = action === 'reply' ? 'Reply dispatched.' : action === 'trade_accept' ? 'Offer accepted.' : 'Offer declined.';
    } catch (error) {
      if (!alive() || request !== generation) return;
      result.textContent = error.message || 'Response failed. Try again.';
      article.querySelectorAll('button').forEach(b => { b.disabled = false; });
    }
  }
  await load(cursor);
}
