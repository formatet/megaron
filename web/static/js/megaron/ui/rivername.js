// A river's canonical anchor is stable. The server grants naming to its first physical arrival.
import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';

export function riverPanelHTML(tile) {
  if (!tile.can_name || !/^-?\d+,-?\d+$/.test(tile.river_id || '') ||
      !['river', 'river_ford', 'river_delta'].includes(tile.terrain) ||
      tile.tier === 'fog' || tile.visible === false) return '';
  const id = tile.river_id;
  return `<div class="river-note" style="font-size:.73rem;border-top:1px solid var(--border);padding:.4rem 0">`
    + `<div style="margin-top:.3rem">Your unit was the first to reach this river. Give it a name (once, 2–30 characters):</div>`
    + `<input id="river-name-${esc(id)}" aria-label="River name" maxlength="30" style="width:60%"> `
    + `<button onclick="nameRiver('${id}')">Name it</button> <span id="river-name-err-${esc(id)}" role="status"></span></div>`;
}

export async function nameRiver(riverID) {
  const input = document.getElementById('river-name-' + riverID);
  const err = document.getElementById('river-name-err-' + riverID);
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/rivers/${riverID}/name`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: input ? input.value : '' }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      if (err) { err.style.color = 'var(--accent)'; err.textContent = body.error || 'Could not name the river.'; }
      return;
    }
    // Refresh the authoritative FOW map before reopening; every known hex of the
    // river then gets its name and the spent right, without revealing unseen hexes.
    const map = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`);
    if (map.ok) { State.tileData = await map.json(); State.dirty = true; }
    if (window.reopenSelectedHex) window.reopenSelectedHex();
  } catch (_) {
    if (err) { err.style.color = 'var(--accent)'; err.textContent = 'Could not reach the server. Check the river before trying again.'; }
  }
}
