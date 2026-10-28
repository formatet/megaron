import { fmtTickTime } from './fmt_clock.js';
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
    const when = st.tier === 'live' ? 'in sight' : `last seen ${fmtTickTime(st.seen_tick)}`;
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

// A stable slot lets weather refresh without rebuilding worker/order controls.
function stormsAtHex(q,r,storms) { return storms.filter(st=>(st.hexes||[]).some(h=>h.q===q&&h.r===r)); }
function stormSignature(storms) { return JSON.stringify(storms.map(st=>[st.id,st.tier,st.name,st.can_name,st.tier==='live'?null:st.seen_tick])); }
export function stormHexPanelHTML(q,r,storms=State.stormData||[]) {
 const here=stormsAtHex(q,r,storms);
 return `<div class="hex-storm-info" data-storm-q="${Number(q)}" data-storm-r="${Number(r)}" data-storm-signature="${esc(stormSignature(here))}">${stormPanelHTML(here)}</div>`;
}
export function updateStormData(storms,root=globalThis.document) {
 State.stormData=storms||[];State.dirty=true;
 for(const el of root?.querySelectorAll?.('[data-storm-q][data-storm-r]')||[]) {
  const here=stormsAtHex(Number(el.dataset.stormQ),Number(el.dataset.stormR),State.stormData),signature=stormSignature(here);
  if(el.dataset.stormSignature===signature)continue;
  const drafts=[...(el.querySelectorAll?.('input')||[])].map(input=>[input.id,input.value]);
  el.innerHTML=stormPanelHTML(here);el.dataset.stormSignature=signature;
  for(const [id,value] of drafts){const input=root.getElementById?.(id);if(input)input.value=value;}
 }
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
  if (list.ok) updateStormData((await list.json()).storms || []);
}
