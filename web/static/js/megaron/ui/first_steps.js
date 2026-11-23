import { fetchAuth, onWorldMutation } from '../api.js';
import { State } from '../state.js';
import { fmtTickTime } from './fmt_clock.js';

// ── The Lawagetas' first steps — one line on the map ───────────────────────
// Rules and texts live on the server (api/handlers/first_steps.go); this file
// only renders what GET /worlds/{w}/first-steps says, so web and keryx cannot
// disagree. The card ends by itself: when the steps are done it says the
// closing line once (remembered per world in localStorage) and never returns.

const closedKey = worldID => `lawagetas_closed_${worldID}`;
function storageGet(key) { try { return localStorage.getItem(key); } catch (_) { return null; } }
function storageSet(key, val) { try { localStorage.setItem(key, val); } catch (_) { /* private mode: it just shows again */ } }

// Pure: what the card shows for a server body. `closedBefore` = the closing
// line was already shown in an earlier visit. Returns null when nothing shows.
export function cardModel(data, { closedBefore = false, when = fmtTickTime } = {}) {
  if (!data || data.hidden) return null;
  const progress = `${data.done_count}/${data.total}`;
  if (!data.next) {
    if (closedBefore || !data.closing_text) return null;
    return { text: data.closing_text, progress, closing: true };
  }
  const step = (data.steps || []).find(s => s.key === data.next);
  if (!step) return null;
  let text = step.text;
  if (text.includes('{when}')) {
    let at = step.ready_tick == null ? 'time unavailable' : when(step.ready_tick);
    if (!at || at === 'time unavailable') at = 'soon';
    text = text.replace('{when}', at);
  }
  return { text, progress, closing: false };
}

let shownClosingFor = null; // keep the closing line up until the page reloads

export function renderCard(el, model) {
  if (!el) return;
  if (!model) { el.hidden = true; return; }
  el.hidden = false;
  el.classList.toggle('lawagetas-closing', !!model.closing);
  el.querySelector('.lawagetas-text').textContent = model.text;
  el.querySelector('.lawagetas-progress').textContent = model.progress;
}

export async function refreshFirstSteps() {
  const worldID = State.WORLD_ID;
  const el = document.getElementById('lawagetas-card');
  if (!worldID || !el) return;
  try {
    const r = await fetchAuth(`/api/v1/worlds/${encodeURIComponent(worldID)}/first-steps`);
    if (!r.ok || State.WORLD_ID !== worldID) return;
    const data = await r.json();
    const closedBefore = storageGet(closedKey(worldID)) === '1' && shownClosingFor !== worldID;
    const model = cardModel(data, { closedBefore });
    if (model && model.closing) { shownClosingFor = worldID; storageSet(closedKey(worldID), '1'); }
    renderCard(el, model);
  } catch (_) { /* the card is a convenience; stay quiet */ }
}

export async function setFirstStepsHidden(hidden) {
  const worldID = State.WORLD_ID;
  if (!worldID) return false;
  const r = await fetchAuth(`/api/v1/worlds/${encodeURIComponent(worldID)}/first-steps/${hidden ? 'hide' : 'show'}`, { method: 'POST' });
  return r.ok;
}

export function initFirstSteps() {
  const el = document.getElementById('lawagetas-card');
  if (!el) return;
  el.querySelector('.lawagetas-close').addEventListener('click', async () => {
    el.hidden = true;
    try { await setFirstStepsHidden(true); } catch (_) { /* hidden for this view at least */ }
  });
  let timer = null;
  // Anything that can change a step (founding, placement, build, march) is a
  // successful mutation on a world route: ask again, once, a moment later.
  onWorldMutation(url => {
    if (url.includes('/first-steps')) return;
    clearTimeout(timer);
    timer = setTimeout(refreshFirstSteps, 400);
  });
  refreshFirstSteps();
}
