import { openMobilePanel, closeMobilePanel } from './mobile_panels.js';
import { LEGEND_ENTRIES, legendCanvas } from '../render/legend.js';

// ── Map legend — what the little symbols on the hexes mean. One row per
// symbol, drawn by the map's own drawing code (render/legend.js). Same
// overlay chrome as the account window.

export function closeLegendWindow() {
  const el = document.getElementById('legend-window-overlay');
  closeMobilePanel(el);
  if (el) el.classList.remove('open');
}

export function toggleLegendWindow() {
  const overlay = document.getElementById('legend-window-overlay');
  if (!overlay) return;
  if (overlay.classList.contains('open')) { closeLegendWindow(); return; }
  const body = document.getElementById('legend-window-body');
  if (!body) return;
  body.replaceChildren();
  let group = null;
  for (const e of LEGEND_ENTRIES) {
    if (e.group !== group) {
      group = e.group;
      const h = document.createElement('h3');
      h.className = 'legend-group';
      h.textContent = group;
      body.appendChild(h);
    }
    const row = document.createElement('div');
    row.className = 'legend-row';
    const label = document.createElement('span');
    label.textContent = e.label;
    row.append(legendCanvas(e), label);
    body.appendChild(row);
  }
  openMobilePanel(overlay, closeLegendWindow);
  overlay.classList.add('open');
}
