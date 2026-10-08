import { fetchAuth } from '../api.js';
import { loadSearchMessages, searchMessagesHTML } from './search_messages.js';
import { State } from '../state.js';
import { hexPx, SCALE, canvas, clampCamera } from '../render/map.js';
import { arrivalHTML } from './time.js';
import { isTypingTarget, esc } from './format.js';
import { warMovements } from './movements.js';

let searchSession = 0;
let correspondence = {};
let correspondenceLoading = false;

// ── Search overlay (Sprint 4) ─────────────────────────────────────────────
export function toggleSearch() {
  const o = document.getElementById('search-overlay');
  o.classList.toggle('open');
  if (o.classList.contains('open')) {
    State.searchFocusIdx = -1;
    const input = document.getElementById('search-input');
    input.value = '';
    input.focus();
    const session = ++searchSession;
    const worldID = State.WORLD_ID;
    correspondence = {};
    correspondenceLoading = true;
    renderSearch('');
    loadSearchMessages(worldID, fetchAuth).then(data => {
      if (session !== searchSession || worldID !== State.WORLD_ID || !o.classList.contains('open')) return;
      // The late message index must not drop an arrow-selection the player has
      // already made: re-find the same row after the re-render.
      const keep = State.searchFocusIdx >= 0
        ? document.querySelector('#search-results .sr-item.focused .sr-name')?.textContent : null;
      correspondence = data;
      correspondenceLoading = false;
      State.searchFocusIdx = -1;
      renderSearch(input.value);
      if (keep != null) {
        const items = [...document.querySelectorAll('#search-results .sr-item')];
        const idx = items.findIndex(el => el.querySelector('.sr-name')?.textContent === keep);
        if (idx >= 0) {
          State.searchFocusIdx = idx;
          items.forEach((el, i) => el.classList.toggle('focused', i === idx));
        }
      }
    });
  } else {
    closeSearch();
  }
}
export function closeSearch(e) {
  ++searchSession;
  correspondence = {};
  document.getElementById('search-overlay').classList.remove('open');
}

// Which result Enter acts on. With an explicit arrow-selection, that row; with
// nothing selected (searchFocusIdx = -1, the state right after typing), the
// first hit — a search box that ignores Enter reads as broken, and Enter-to-go
// is the only way to navigate to something you can't see (Timothy). Pure half,
// unit-tested; returns -1 when there is nothing to act on.
export function enterTargetIndex(focusIdx, count) {
  if (count <= 0) return -1;
  if (focusIdx >= 0 && focusIdx < count) return focusIdx;
  return 0;
}

export function centreOn(q, r) {
  const {x, y} = hexPx(q, r);
  State.camera.x = canvas.width/2  - x * SCALE * State.camera.zoom;
  State.camera.y = canvas.height/2 - y * SCALE * State.camera.zoom;
  clampCamera(); // the seventh camera-mutation site — World-Rim step 1
  State.dirty = true;
}

document.getElementById('search-input').addEventListener('input', function() {
  State.searchFocusIdx = -1;
  renderSearch(this.value.toLowerCase());
});

document.getElementById('search-input').addEventListener('keydown', function(e) {
  const items = [...document.querySelectorAll('#search-results .sr-item')];
  if (!items.length) return;
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    State.searchFocusIdx = Math.min(State.searchFocusIdx + 1, items.length - 1);
    items.forEach((el, i) => el.classList.toggle('focused', i === State.searchFocusIdx));
    items[State.searchFocusIdx].scrollIntoView({block: 'nearest'});
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    State.searchFocusIdx = Math.max(State.searchFocusIdx - 1, 0);
    items.forEach((el, i) => el.classList.toggle('focused', i === State.searchFocusIdx));
    items[State.searchFocusIdx].scrollIntoView({block: 'nearest'});
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const idx = enterTargetIndex(State.searchFocusIdx, items.length);
    if (idx >= 0) items[idx].click();
  }
});

function renderSearch(q) {
  q = q.trim().toLowerCase();
  const results = document.getElementById('search-results');

  // Own settlements and FOW-visible others (State.provinceData is already server-side FOW-filtered)
  const ownSettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
  const visibleOther   = State.provinceData.filter(p => !p.own && !p.is_outpost && p.name);

  // Own armies in the field — the unit layer plus legacy recall columns (movements.js).
  const ownArms = warMovements({
    units: State.unitsData, marches: State.marchData, provinces: State.provinceData,
  }).outgoing;

  function match(s) { return !q || (s || '').toLowerCase().includes(q); }

  function terrainLabel(p) {
    const t = State.tileData.find(t => t.q === p.q && t.r === p.r);
    return t ? t.terrain.replace(/_/g, ' ') : '';
  }

  let html = '';
  const settOwn = ownSettlements.filter(p => match(p.name));
  if (settOwn.length) {
    html += `<div class="sr-category">Your Cities</div>`;
    html += settOwn.map(p => `
      <div class="sr-item" onclick="closeSearch();centreOn(${p.q},${p.r})">
        <span class="sr-icon">🏛</span>
        <span class="sr-name">${p.name}</span>
        <span class="sr-meta">${terrainLabel(p)}${p.walls ? ' · L' + p.walls + ' walls' : ''}</span>
        <span class="sr-type">City</span>
      </div>`).join('');
  }
  const settOther = visibleOther.filter(p => match(p.name));
  if (settOther.length) {
    html += `<div class="sr-category">Visible Cities</div>`;
    html += settOther.slice(0, 8).map(p => `
      <div class="sr-item" onclick="closeSearch();centreOn(${p.q},${p.r})">
        <span class="sr-icon">🏛</span>
        <span class="sr-name">${p.name}</span>
        <span class="sr-meta">${p.allied ? 'Ally' : (p.owner || 'Enemy')}</span>
        <span class="sr-type">${p.allied ? 'Ally' : 'Enemy'}</span>
      </div>`).join('');
  }
  if (ownArms.length && (!q || match('army march'))) {
    html += `<div class="sr-category">Your Armies</div>`;
    html += ownArms.map(m => `
      <div class="sr-item" onclick="closeSearch();centreOn(${m.target_q},${m.target_r})">
        <span class="sr-icon">⚔</span>
        <span class="sr-name">${esc(m.title)} → (${m.target_q},${m.target_r})</span>
        <span class="sr-meta">Arrives ${arrivalHTML(m.arrives_at)}</span>
        <span class="sr-type">Army</span>
      </div>`).join('');
  }
  html += searchMessagesHTML(q, correspondence);
  if (q && correspondenceLoading) html += '<div class="sr-category">Loading letters and rumours…</div>';
  if (q && correspondence.failed?.length) html += `<div class="sr-category">Could not load: ${esc(correspondence.failed.join(', '))}. Close and reopen search to retry.</div>`;
  if (!html) html = '<div style="padding:.6rem .8rem;font-size:.8rem;color:var(--text-dim)">No results.</div>';
  results.innerHTML = html;
  results.querySelectorAll('[data-search-drawer]').forEach(item => {
    item.addEventListener('click', () => {
      closeSearch();
      window.openDrawer(item.dataset.searchDrawer);
    });
  });
}

document.addEventListener('keydown', e => {
  if ((e.key === 'f' || e.key === '/') && !isTypingTarget(document.activeElement)) {
    e.preventDefault();
    toggleSearch();
  }
});
