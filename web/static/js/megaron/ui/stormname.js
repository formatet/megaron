// Naming a storm (megaron_plan_stormnamn.md): the first Wanax whose ship a storm strikes may
// name it, once. GET /storms says which storms you may still name (can_name); the sea-hex
// panel shows the name, or a small form if the right is yours.
import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';

// stormPanelHTML renders the panel block for the storms on one hex ('' when there are none).
export function stormPanelHTML(storms) {
  if (!storms.length) return '';
  return storms.map(st => {
    const when = st.tier === 'live' ? 'in sight' : `last seen on day ${st.seen_tick}`;
    const title = st.name ? `Storm ${esc(st.name)}` : 'Storm';
    let html = `<div class="storm-note" style="font-size:.73rem;border-top:1px solid var(--border);padding:.4rem 0"><strong>${title}</strong> — ${when}`;
    if (st.can_name) {
      html += `<div style="margin-top:.3rem">You were the first to meet this storm. Give it a name (once, 2–30 letters):</div>`
        + `<input id="storm-name-${esc(st.id)}" maxlength="30" style="width:60%"> `
        + `<button onclick="nameStorm('${esc(st.id)}')">Name it</button> <span id="storm-name-err-${esc(st.id)}"></span>`;
    }
    return html + '</div>';
  }).join('');
}

export async function nameStorm(stormID) {
  const input = document.getElementById('storm-name-' + stormID);
  const err = document.getElementById('storm-name-err-' + stormID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/storms/${stormID}/name`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: input ? input.value : '' }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    if (err) { err.style.color = 'var(--accent)'; err.textContent = body.error || 'Could not name the storm.'; }
    return;
  }
  const list = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/storms`);
  if (list.ok) { State.stormData = (await list.json()).storms || []; State.dirty = true; }
  if (window.reopenSelectedHex) window.reopenSelectedHex();
}
