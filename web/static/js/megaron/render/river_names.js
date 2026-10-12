// Names arrive from Temenos. Never derive chains from a partial FOW view.
export function namedRiverTerrain(tile, fallback) {
  if (!tile.river_name || tile.terrain === 'fog' || tile.tier === 'fog' || tile.visible === false) return fallback;
  switch (tile.terrain) {
    case 'river': return `River ${tile.river_name}`;
    case 'river_ford': return `Ford of ${tile.river_name}`;
    case 'river_delta': return `Delta of ${tile.river_name}`;
    default: return fallback;
  }
}
