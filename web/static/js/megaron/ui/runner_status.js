// ── Where a runner actually is, in words ───────────────────────────────────
// One reading of a messenger row (status + passage_status) for the two web
// surfaces that describe one: the Diplomacy outbox (ListSent rows) and the
// War → Army card's pending-order line (/messengers markers, kind='order').
//
// Both used to read arrives_at alone. While a runner waits in port for a ship
// (passage_status='awaiting_passage') arrives_at is the moment it REACHED the
// port, not its destination — so once that passed, the order card said
// "carrying the order…" for a runner standing still on the quay. The outbox
// matched status against 'delivering'/'returned', words the server never
// sends (it sends outbound → delivered → returning → arrived), so every row
// fell through to the raw status and the passage note never showed.
//
// Lifecycle source: internal/messenger (return_leg.go, call_back.go,
// passage.go) and mig 148. Codex: sea.md §"A letter or runner…".
import { esc } from './format.js';
import { arrivalHTML } from './time.js';

const dim = s => '<span style="color:var(--text-dim)">' + s + '</span>';

// passagePhrase: the sea-lift states that make arrives_at meaningless as an
// arrival. '' when the runner is simply on the road (or riding a ship, where
// arrives_at IS the real landfall-plus-walk time).
function passagePhrase(m) {
  const port = m.passage_port ? ' in ' + esc(m.passage_port) : '';
  if (m.passage_status === 'awaiting_passage') return 'waiting' + port + ' for a ship';
  if (m.passage_status === 'unknown') return 'no word';
  if (m.passage_status === 'returning_sealed') return 'its ship was lost — sealed, coming back to port' + (m.passage_port ? ' (' + esc(m.passage_port) + ')' : '');
  return '';
}

function aboardBit(m) {
  return m.passage_status === 'aboard' && m.carrier_name ? ' · ⛵ aboard ' + esc(m.carrier_name) : '';
}

// sentStatusHTML: the outbox status line for a letter's runner, once no trade
// status (accepted/declined/expired) has already said what happened.
export function sentStatusHTML(m) {
  const stuck = passagePhrase(m);
  switch (m.status) {
    case 'outbound':
      return dim(stuck ? stuck : 'en route · arrives ' + arrivalHTML(m.arrives_at) + aboardBit(m));
    case 'delivered':
      return dim('✓ delivered · waiting on a reply');
    case 'returning':
      return dim('↩ coming home · ' + (stuck ? stuck : 'back ' + arrivalHTML(m.arrives_at, undefined, 'any moment') + aboardBit(m)));
    case 'arrived':
      return '<span style="color:var(--safe)">↩ home</span>';
    default:
      return dim(esc(m.status || ''));
  }
}

// orderRunnerHTML: the War → Army card's line for a runner carrying an order
// to this unit. nowMs is server time (serverNow()), passed in to keep this pure.
export function orderRunnerHTML(runner, nowMs) {
  const line = s => '<div style="font-size:.65rem;color:var(--text-dim)">🏃 ' + s + '</div>';
  const stuck = passagePhrase(runner);
  if (stuck) return line('Messenger ' + stuck + ' — the order has not reached the unit');
  // Once the runner has arrived, the order is being applied server-side (a
  // worker poll away) — say so, not the stale "en route" ETA.
  if (nowMs >= new Date(runner.arrives_at).getTime()) return line('Messenger carrying the order…');
  return line('Messenger en route — order arrives ' + arrivalHTML(runner.arrives_at) + aboardBit(runner));
}
