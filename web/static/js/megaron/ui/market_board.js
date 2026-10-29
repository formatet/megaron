// market_board.js — the city's market board (megaron_plan_marknadstavlan).
// The owner writes what the city buys and sells; others read it on site and
// see it as it stood then. Nothing here looks at stock.
import { State } from '../state.js';
import { fetchAuth } from '../api.js';
import { esc, formatApiError } from './format.js';
import { showInlineResult } from './inline_result.js';
import { track } from '../telemetry.js';
import { fmtTickTime } from './fmt_clock.js';

let _goods = null;
async function boardGoods() {
  if (!_goods) {
    _goods = fetchAuth('/api/v1/goods').then(r => r.ok ? r.json() : null).catch(() => null);
  }
  const list = await _goods;
  if (!list) _goods = null;
  return list || [];
}

// "Buys tin · Sells wine", or that the board is empty.
export function boardLine(board) {
  const buys = board?.buys || [], sells = board?.sells || [];
  if (!buys.length && !sells.length) return 'the board is empty';
  const parts = [];
  if (buys.length) parts.push('Buys ' + buys.join(', '));
  if (sells.length) parts.push('Sells ' + sells.join(', '));
  return parts.join(' · ');
}

// A foreign board as the viewer last read it: when (wall clock) and how old.
export function readingHTML(board) {
  if (!board) return '';
  const when = board.read_at ? fmtTickTime(null, board.read_at) : '';
  const age = board.read_game_days_ago != null ? `, ${board.read_game_days_ago} game days ago` : '';
  return `<div class="dsec-title">Market board</div>
    <p class="market-board-reading">${esc(boardLine(board))}</p>
    <p class="empty-state">Read ${esc(when)}${esc(age)}. It says what its ruler chose to write, as it stood then. Ask in a letter.</p>`;
}

function lineSelects(line, chosen, goods, max) {
  const opts = sel => ['<option value="">—</option>', ...goods.map(g =>
    `<option value="${esc(g.key)}"${g.key === sel ? ' selected' : ''}>${esc(g.name || g.key)}</option>`)].join('');
  return Array.from({ length: max }, (_, i) =>
    `<select class="build-select market-board-select" data-board-line="${line}">${opts(chosen[i] || '')}</select>`).join('');
}

// The owner's board with its editor: up to max_goods goods on each line.
export function ownBoardHTML(board, goods, settlementID) {
  if (!board) return '';
  const max = board.max_goods || 3;
  return `<div class="dsec-title">Market board</div>
    <p class="market-board-reading">${esc(boardLine(board))}</p>
    <p class="empty-state">Other rulers' caravans, armies and messengers read it within ${board.reach} hexes of the city and remember it as it stood. It is your word; nothing checks it against your stores.</p>
    <div class="market-board-line"><span>Buys</span>${lineSelects('buys', board.buys || [], goods, max)}</div>
    <div class="market-board-line"><span>Sells</span>${lineSelects('sells', board.sells || [], goods, max)}</div>
    <button class="btn-small" onclick="saveMarketBoard('${esc(settlementID)}')">Put up the board</button>
    <div id="city-board-res" class="action-result"></div>`;
}

export async function ownBoardSection(board, settlementID) {
  if (!board) return '';
  return ownBoardHTML(board, await boardGoods(), settlementID);
}

export async function saveMarketBoard(settlementID) {
  const pick = line => [...document.querySelectorAll(`select[data-board-line="${line}"]`)]
    .map(s => s.value).filter(Boolean);
  const res = document.getElementById('city-board-res');
  const r = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/${settlementID}/market-board`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ buys: pick('buys'), sells: pick('sells') }),
  });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) { showInlineResult(res, formatApiError(data, 'Could not put up the board.'), true); return; }
  track('market_board_set', {});
  showInlineResult(res, 'The board now says: ' + boardLine(data));
}
