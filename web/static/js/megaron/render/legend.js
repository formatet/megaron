// ── Map legend table ─────────────────────────────────────────────────────
// Every symbol the map draws on a hex, with the name the player should read.
// Each entry draws itself with the SAME function the map uses (deposit marks,
// city icon, actor sprites, storm, worker marker) — never a second hand-made
// copy that can drift. The deposit rows come from map.js's own DEPOSIT_KINDS.
import {
  DEPOSIT_KINDS, drawDepositIcons, drawProvinceIcon, drawWorkerMarker, drawStormSample,
} from './map.js';
import { drawActor, FOREIGN_ACCENT, FOREIGN_OUTLINE } from './actorsprites.js';

export const LEGEND_W = 32;
export const LEGEND_H = 30;
const CX = 16, CY = 15;   // hex centre inside a legend canvas

// The map's own city icon draws its foot 14 px below the centre; the legend
// frame is a little taller so the flag fits.
const CITY = { id: 1, q: 0, r: 0, size_tier: 0, walls: 0, culture: 'akhaier' };

const deposit = k => ({
  id: 'deposit_' + k.key, label: k.label, group: 'Deposits',
  draw: ctx => drawDepositIcons(ctx, CX - 9, CY + 1, { [k.field]: true }, true),
});

export const LEGEND_ENTRIES = [
  ...DEPOSIT_KINDS.map(deposit),
  { id: 'own_city', label: 'Your city', group: 'Cities',
    draw: ctx => drawProvinceIcon(ctx, CX, CY - 4, { ...CITY, own: true }) },
  { id: 'foreign_city', label: "Another Wanax's city", group: 'Cities',
    draw: ctx => drawProvinceIcon(ctx, CX, CY - 4, { ...CITY }) },
  { id: 'host', label: 'Host (your people before a city)', group: 'People',
    draw: ctx => drawActor(ctx, 'nomadic_host', CX, CY + 8, '', 0) },
  { id: 'army', label: 'Your army', group: 'People',
    draw: ctx => drawActor(ctx, 'spearman', CX, CY + 8, '', 0) },
  { id: 'foreign_army', label: "Another Wanax's army", group: 'People',
    draw: ctx => drawActor(ctx, 'spearman', CX, CY + 8, '', 0, FOREIGN_ACCENT, FOREIGN_OUTLINE) },
  { id: 'ship', label: 'Ship', group: 'People',
    draw: ctx => drawActor(ctx, 'galley', CX, CY + 8, '', 0) },
  { id: 'caravan', label: 'Caravan', group: 'People',
    draw: ctx => drawActor(ctx, 'caravan', CX, CY + 8, '', 0) },
  { id: 'messenger', label: 'Messenger', group: 'People',
    draw: ctx => drawActor(ctx, 'runner', CX, CY + 8, '', 0, '#6B8B4A') },
  { id: 'workers', label: 'Your citizens are working here', group: 'Land',
    draw: ctx => drawWorkerMarker(ctx, CX + 7, CY - 6) },
  { id: 'storm', label: 'Storm at sea', group: 'Land',
    draw: ctx => drawStormSample(ctx, CX, CY) },
];

/** Draw one entry into a fresh canvas element. */
export function legendCanvas(entry, doc = document) {
  const c = doc.createElement('canvas');
  c.width = LEGEND_W; c.height = LEGEND_H;
  c.className = 'legend-sym';
  const ctx = c.getContext('2d');
  ctx.imageSmoothingEnabled = false;
  entry.draw(ctx);
  return c;
}
