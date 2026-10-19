import { fmtNum } from './fmt_num.js';
// Presentation order only: quantities, balances and durations belong to the server.
export function foodGoodsText(goods) {
  if (!goods) return '';
  return ['grain', 'fish', 'oil', 'wine', 'livestock']
    .filter(good => Number.isFinite(goods[good]))
    .map(good => `${goods[good] > 0 && goods[good] < 0.1 ? 'under 0.1' : fmtNum(goods[good])} ${good}`).join(', ');
}
