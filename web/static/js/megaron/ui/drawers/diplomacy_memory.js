import { fetchAuth } from '../../api.js';
import { esc, formatApiError } from '../format.js';
import { fmtNum } from '../fmt_num.js';

export function citationFormHTML(letter) {
  return `<details><summary>Keep a private passage</summary><form data-citation-letter="${esc(letter.id)}"><label>Source<select name="source"><option value="message">Letter</option>${letter.reply_text ? '<option value="reply">Reply</option>' : ''}</select></label><label>Exact passage<textarea name="quote" maxlength="1000" required>${esc(letter.message_text)}</textarea></label><button class="btn-small">Keep as my note</button><p role="status"></p></form></details>`;
}
function termsHTML(offer) {
  return offer.kind === 'sell'
    ? `${fmtNum(offer.offer_qty)} ${esc(offer.offer_good)} for ${fmtNum(offer.want_silver)} silver`
    : `${fmtNum(offer.want_qty)} ${esc(offer.want_good)} for ${fmtNum(offer.offer_silver)} silver`;
}
export function memoryHTML(page) {
  return '<h3 class="dip-archive-heading">Memory with this Wanax</h3><p class="dip-queue-note">Across your cases with this person. Your notes are private; an accepted agreement is separate from delivery.</p><h4 class="dip-memory-heading">Accepted agreements</h4>'
    + ((page.transactions || []).map(t => `<article class="dip-queue-item"><strong>${esc(t.subject)}</strong><div>${esc(t.sender_name)} · ${esc(t.origin_name)} → ${esc(t.recipient_name)} · ${esc(t.destination_name)}</div><p>${termsHTML(t.terms)} · Accepted</p><p class="dip-queue-note">${t.accepted_at ? 'Accepted ' + esc(new Date(t.accepted_at).toLocaleString()) : 'Acceptance time unavailable'}. Delivery outcome unavailable in this record.</p><button class="btn-small" data-memory-case="${esc(t.case_id)}" data-memory-letter="${esc(t.id)}">Open source letter</button></article>`).join('') || '<p>No accepted agreements on this page.</p>')
    + '<h4 class="dip-memory-heading">My notes from letters</h4>' + ((page.citations || []).map(c => `<article class="dip-queue-item"><p class="dip-queue-note">My note · ${c.source === 'reply' ? 'reply' : 'letter'}${c.author_name ? ' · ' + esc(c.author_name) : ''}${c.city_name ? ' · ' + esc(c.city_name) : ''}</p><blockquote class="dip-queue-message">${esc(c.quote)}</blockquote><div class="dip-queue-actions"><button class="btn-small" data-memory-case="${esc(c.case_id)}" data-memory-letter="${esc(c.letter_id)}">Open source letter</button><button class="btn-small" data-citation-delete="${esc(c.id)}">Remove my note</button></div></article>`).join('') || '<p>No private passages on this page.</p>');
}

export async function mountCaseMemory(root, world, caseID, alive, archiveRoot, letters, openCase) {
  const base = `/api/v1/worlds/${world}/correspondence/cases/${encodeURIComponent(caseID)}`;
  let generation = 0;
  async function load(cursor = '') {
    const n = ++generation;
    root.innerHTML = '<div class="loading">Loading memory…</div>';
    try {
      const response = await fetchAuth(base + '/memory?limit=30' + (cursor ? '&cursor=' + encodeURIComponent(cursor) : ''), {sensitive:true});
      const page = await response.json();
      if (!alive() || n !== generation) return;
      if (!response.ok) throw new Error(formatApiError(page,'Could not load memory.'));
      root.innerHTML = '<div class="dip-queue-toolbar"><button class="btn-small" data-memory-refresh>Refresh memory</button></div>' + memoryHTML(page) + (page.next_cursor ? '<button class="btn-small" data-memory-next>Older memory →</button>' : '') + '<p role="status"></p>';
      root.querySelector('[data-memory-refresh]')?.addEventListener('click', () => load());
      root.querySelector('[data-memory-next]')?.addEventListener('click', () => load(page.next_cursor));
      root.querySelectorAll('[data-memory-case]').forEach(button => button.addEventListener('click', () => { if (alive()) openCase(button.dataset.memoryCase, button.dataset.memoryLetter); }));
      root.querySelectorAll('[data-citation-delete]').forEach(button => button.addEventListener('click', async () => {
        if (!alive()) return; button.disabled = true;
        try {
          const response = await fetchAuth(base + '/citations/' + encodeURIComponent(button.dataset.citationDelete), {method:'DELETE',sensitive:true});
          if (!alive() || n !== generation) return;
          if (!response.ok) throw new Error(formatApiError(await response.json().catch(()=>({})),'Could not remove your note.'));
          await load(cursor);
        } catch (error) { if (alive() && n === generation) { root.querySelector('[role="status"]').textContent = error.message; button.disabled = false; } }
      }));
    } catch (error) {
      if (!alive() || n !== generation) return;
      root.innerHTML = `<p>${esc(error.message)}</p><button class="btn-small" data-memory-retry>Retry memory</button>`;
      root.querySelector('[data-memory-retry]')?.addEventListener('click', () => load(cursor));
    }
  }
  archiveRoot.querySelectorAll('[data-citation-letter]').forEach(form => {
    const letter = letters.find(m => m.id === form.dataset.citationLetter);
    if (!letter) return;
    form.elements.source.addEventListener('change', () => { form.elements.quote.value = form.elements.source.value === 'reply' ? letter.reply_text || '' : letter.message_text; });
    form.addEventListener('submit', async event => {
      event.preventDefault(); if (!alive()) return;
      const quote = form.elements.quote.value, source = form.elements.source.value;
      const button = form.querySelector('button'), status = form.querySelector('[role="status"]');
      if (!quote) { status.textContent = 'Choose an exact passage from this letter.'; return; }
      button.disabled = true;
      try {
        const response = await fetchAuth(base + '/citations', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({letter_id:letter.id,source,quote}),sensitive:true});
        const result = await response.json().catch(()=>({}));
        if (!alive()) return;
        if (!response.ok) throw new Error(formatApiError(result,'Could not keep your note.'));
        status.textContent = 'Kept as your private note. This creates no shared agreement.';
        await load();
      } catch (error) { if (alive()) status.textContent = error.message; }
      finally { if (alive()) button.disabled = false; }
    });
  });
  await load();
}
