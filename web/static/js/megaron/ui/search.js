import { State } from '../state.js';
import { hexPx, SCALE, canvas } from '../render/map.js';
import { fmtEta } from './format.js';

// ── Search overlay (Sprint 4) ─────────────────────────────────────────────
export function toggleSearch() {
  const o = document.getElementById('search-overlay');
  o.classList.toggle('open');
  if (o.classList.contains('open')) {
    State.searchFocusIdx = -1;
    document.getElementById('search-input').focus();
    renderSearch('');
  }
}
export function closeSearch(e) {
  document.getElementById('search-overlay').classList.remove('open');
}

export function centreOn(q, r) {
  const {x, y} = hexPx(q, r);
  State.camera.x = canvas.width/2  - x * SCALE * State.camera.zoom;
  State.camera.y = canvas.height/2 - y * SCALE * State.camera.zoom;
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
    if (State.searchFocusIdx >= 0 && State.searchFocusIdx < items.length) {
      items[State.searchFocusIdx].click();
    }
  }
});

function renderSearch(q) {
  const results = document.getElementById('search-results');

  // Own settlements and FOW-visible others (State.provinceData is already server-side FOW-filtered)
  const ownSettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
  const visibleOther   = State.provinceData.filter(p => !p.own && !p.is_outpost && p.name);

  // Own marches: origin q,r matches any own province
  const ownPos = new Set(State.provinceData.filter(p => p.own).map(p => `${p.q},${p.r}`));
  const ownArms = State.marchData.filter(m => ownPos.has(`${m.origin_q},${m.origin_r}`));

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
        <span class="sr-name">${m.intent.charAt(0).toUpperCase()+m.intent.slice(1)} → (${m.target_q},${m.target_r})</span>
        <span class="sr-meta">ETA ${fmtEta(m.arrives_at)}</span>
        <span class="sr-type">Army</span>
      </div>`).join('');
  }
  if (!html) html = '<div style="padding:.6rem .8rem;font-size:.8rem;color:var(--text-dim)">No results.</div>';
  results.innerHTML = html;
}

document.addEventListener('keydown', e => {
  if ((e.key === 'f' || e.key === '/') && document.activeElement.tagName !== 'INPUT') {
    e.preventDefault();
    toggleSearch();
  }
});
