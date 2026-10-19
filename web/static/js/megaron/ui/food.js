import { fmtNum } from './fmt_num.js';
// Presentation order only: quantities, balances and durations belong to the server.
export function foodGoodsText(goods) {
  if (!goods) return '';
  return ['grain', 'fish', 'oil', 'wine', 'livestock']
    .filter(good => Number.isFinite(goods[good]))
    .map(good => `${fmtNum(goods[good])} ${good}`).join(', ');
}
