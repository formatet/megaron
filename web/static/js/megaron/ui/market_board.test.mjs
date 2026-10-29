import test from 'node:test';
import assert from 'node:assert/strict';
globalThis.window ??= { addEventListener() {}, matchMedia: () => ({ matches: false, addEventListener() {} }) };
globalThis.localStorage ??= { getItem: () => null, setItem() {} };
const { boardLine, readingHTML, ownBoardHTML } = await import('./market_board.js');

test('board line names what the ruler wrote, or that it is empty', () => {
  assert.equal(boardLine({ buys: ['tin'], sells: ['wine', 'oil'] }), 'Buys tin · Sells wine, oil');
  assert.equal(boardLine({ buys: [], sells: [] }), 'the board is empty');
});

test('a reading says how old it is and that it is only the ruler\'s word', () => {
  const html = readingHTML({ buys: ['tin'], sells: [], read_game_days_ago: 3 });
  assert.match(html, /Buys tin/);
  assert.match(html, /3 game days ago/);
  assert.match(html, /Ask in a letter/);
  assert.equal(readingHTML(null), '');
});

test('the owner edits up to max_goods goods per line, current choice selected', () => {
  const goods = [{ key: 'tin', name: 'Tin' }, { key: 'wine', name: 'Wine' }];
  const html = ownBoardHTML({ buys: ['tin'], sells: [], max_goods: 3, reach: 2 }, goods, 's1');
  assert.equal((html.match(/data-board-line="buys"/g) || []).length, 3);
  assert.equal((html.match(/data-board-line="sells"/g) || []).length, 3);
  assert.match(html, /<option value="tin" selected>Tin<\/option>/);
  assert.match(html, /within 2 hexes/);
  assert.match(html, /saveMarketBoard\('s1'\)/);
});
