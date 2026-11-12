// ── Founding forecast rows (Host panel and empty-land panel) ─────────────
// The old summary said "yes" for timber, stone and food on nearly every hex,
// so two sites could not be told apart. These rows give whole numbers from
// what /colonize-preview already returns (goods rates, grain block, catchment
// hexes with their deposits) — no new server computation.
// `label(terrain)` turns a terrain key into the player's word.

const plural = (n, one, many = one + 's') => `${n} ${n === 1 ? one : many}`;

export function forecastRows(p, label = t => t) {
  const labels = ['Food', 'Timber', 'Stone', 'Copper', 'Tin', 'Silver', 'Ground', 'Ships'];
  if (!p || typeof p !== 'object') return labels.map(l => ({ label: l, value: 'unknown' }));

  const hasGoods = p.goods && typeof p.goods === 'object';
  const hasCatchment = Array.isArray(p.catchment);
  const rate = good => {
    if (!hasGoods) return 'unknown';
    const r = p.goods[good];
    const n = Math.round(Number.isFinite(r) ? r : 0);
    return n > 0 ? `up to ${n} a game day, fully worked` : 'none';
  };
  const known = (p.catchment || []).filter(c => c.known);
  const deposit = field => {
    if (!hasCatchment) return 'unknown';
    const n = known.filter(c => c[field]).length;
    return n > 0 ? plural(n, 'hex', 'hexes') : 'none seen';
  };

  const g = p.grain || {};
  const net = g.food_net_per_tick ?? g.est_net_per_tick ?? 0;
  const eaten = g.consumption_per_tick ?? ((g.base_per_tick || 0) - net);
  const food = p.grain && typeof p.grain === 'object'
    ? `${Math.round(g.base_per_tick || 0)} grain + ${Math.round(g.fish_per_tick || 0)} fish a game day, ${Math.round(eaten)} eaten`
    : 'unknown';

  const counts = new Map();
  for (const c of known) if (c.terrain) counts.set(c.terrain, (counts.get(c.terrain) || 0) + 1);
  const ground = !hasCatchment ? 'unknown' : [...counts].sort((a, b) => b[1] - a[1]).slice(0, 4)
    .map(([t, n]) => `${n} ${label(t)}`).join(', ') || 'none known';

  const ships = typeof p.harbour_note === 'string'
    ? (p.harbour_note.startsWith('Coastal') ? 'yes, coast' : 'no ships ever — inland') : 'unknown';

  return [
    { label: 'Food', value: food },
    { label: 'Timber', value: rate('timber') },
    { label: 'Stone', value: rate('stone') },
    { label: 'Copper', value: deposit('copper_deposit') },
    { label: 'Tin', value: deposit('tin_deposit') },
    { label: 'Silver', value: deposit('silver_deposit') },
    { label: 'Ground', value: ground },
    { label: 'Ships', value: ships },
  ];
}
