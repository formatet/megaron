import { acceptMapTiles } from '../map_data.js';
// The server grants a place's naming right to its first physical arrival.
import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc } from './format.js';

export const PLACE_LABELS = { river: 'river', massif: 'mountain massif', desert: 'desert', forest: 'cedar forest' };

export function placePanelHTML(tile, { showName = false } = {}) {
  return toldByHTML(tile) + placeNameHTML(tile, showName);
}

// Ground known only from a story told in a letter (megaron_plan_vagbeskrivningar v1).
function toldByHTML(tile) {
  return tile.told_by && tile.tier === 'remembered'
    ? `<div class="place-note" style="font-size:.73rem;border-top:1px solid var(--border);padding:.4rem 0">Told by ${esc(tile.told_by)} — as they saw it, not seen by your own people</div>`
    : '';
}

function placeNameHTML(tile, showName) {
  const kind = tile.place_kind, id = tile.place_id;
  if (!Object.hasOwn(PLACE_LABELS, kind) || !/^-?\d+,-?\d+$/.test(id || '') ||
      tile.terrain === 'fog' || tile.tier === 'fog' || tile.visible === false) return '';
  if (!tile.can_name) {
    return showName && tile.place_name ? `<div class="place-note" style="font-size:.73rem;border-top:1px solid var(--border);padding:.4rem 0"><strong>${esc(tile.place_name)}</strong> — ${PLACE_LABELS[kind]}</div>` : '';
  }
  const key = kind + '-' + id;
  return `<div class="place-note" style="font-size:.73rem;border-top:1px solid var(--border);padding:.4rem 0">`
    + `<div style="margin-top:.3rem">Your unit was the first to reach this ${PLACE_LABELS[kind]}. Give it a name (once, 2–30 characters):</div>`
    + `<input id="place-name-${esc(key)}" aria-label="Place name" maxlength="30" style="width:60%"> `
    + `<button onclick="namePlace('${kind}','${id}')">Name it</button> <span id="place-name-err-${esc(key)}" role="status"></span></div>`;
}

export async function namePlace(kind, placeID) {
  if (!Object.hasOwn(PLACE_LABELS, kind) || !/^-?\d+,-?\d+$/.test(placeID || '')) return;
  const key = kind + '-' + placeID;
  const input = document.getElementById('place-name-' + key);
  const err = document.getElementById('place-name-err-' + key);
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/places/${kind}/${placeID}/name`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: input ? input.value : '' }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      if (err) { err.style.color = 'var(--accent)'; err.textContent = body.error || 'Could not name the place.'; }
      return;
    }
    // Refresh the authoritative FOW map before reopening: known hexes acquire
    // the name and spent right together, while unseen hexes stay hidden.
    const map = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/map`);
    if (map.ok) acceptMapTiles(await map.json());
    // Opening a drawer clears selectedHex. An owned-city naming form therefore
    // refreshes its current drawer rather than attempting to reopen a hex.
    if (State.activeDrawer === 'city' && window.openDrawer) window.openDrawer('city');
    else if (window.reopenSelectedHex) window.reopenSelectedHex();
  } catch (_) {
    if (err) { err.style.color = 'var(--accent)'; err.textContent = 'Could not reach the server. Check the place before trying again.'; }
  }
}
