// ── Shared formatting helpers ──────────────────────────────────────────────
// Pure functions, no DOM/State deps — safe for any other module to import
// directly regardless of layer (config/state ← api/ws ← render ← ui ← main).
// (clock.js sits on the same low layer, so importing it keeps that promise.)
import { serverNow } from '../clock.js';
export function esc(s) { return (s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); }

// True when `el` is a live text-entry target — an <input>/<textarea>/<select>
// or any contenteditable region — and single-letter keyboard shortcuts (map
// pan, search's f//) must not fire. Takes the element instead of reading
// document.activeElement itself, keeping this pure per the file header;
// callers pass document.activeElement.
export function isTypingTarget(el) {
  if (!el) return false;
  if (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return true;
  return !!el.isContentEditable;
}

// NOTE (FAS 2, flagged in the execution report): the original map.html script
// already defined ITS OWN fmtSilver (this one) in addition to the copy of
// base.html's fmtSilver that FAS 1 prepended to the classic script per the
// exekveringsplan. Two `function fmtSilver` declarations in one classic
// <script> are legal — the later one silently wins — so base.html's version
// was already dead/shadowed on the map page before this split (and still was
// after FAS 1). ES modules make duplicate top-level declarations a hard
// SyntaxError, so only the one that was actually live is kept here; the
// shadowed base.html copy is dropped as unreachable code, not a behaviour
// change.
export function fmtSilver(amount) {
  const a = Math.floor(amount || 0);
  if (a >= 3600) return (a / 3600).toFixed(1) + ' talent';
  if (a >= 60)   return (a / 60).toFixed(1) + ' mina';
  return a + ' shekel';
}

// fmtEta moved to ui/time.js (Tid & kalender Fas B) — the whole ETA family
// (relative, local-clock, tick-aware) lives there now.

export function fmtAgo(iso) {
  const ms = serverNow() - new Date(iso).getTime();
  if (ms < 60000)   return 'just now';
  if (ms < 3600000) return Math.floor(ms / 60000) + 'm ago';
  if (ms < 86400000) return Math.floor(ms / 3600000) + 'h ago';
  return Math.floor(ms / 86400000) + 'd ago';
}

// fmtSoon: local, minimal future-relative helper for notifText's OfferAccepted
// ETA tail. Deliberately NOT delegated to ui/time.js's fmtEta — that module
// imports esc/fmtAgo FROM this file, so importing it back here would be a
// cycle. Same rough bucketing as fmtAgo above, just future-facing. Guards
// missing/invalid timestamps by returning '' so callers can omit the tail.
function fmtSoon(iso) {
  const t = iso ? new Date(iso).getTime() : NaN;
  if (Number.isNaN(t)) return '';
  const ms = t - serverNow();
  if (ms <= 0)        return 'any moment now';
  if (ms < 3600000)   return 'in ~' + Math.max(1, Math.round(ms / 60000)) + ' min';
  if (ms < 86400000)  return 'in ~' + (ms / 3600000).toFixed(1) + ' h';
  return 'in ~' + (ms / 86400000).toFixed(1) + ' d';
}

export function notifIcon(kind) {
  const icons = {
    BuildComplete:      '🏛',
    GoodsCrafted:       '🔨',
    TrainComplete:      '⚔',
    ArmyArrival:        '⚔',
    ColonyFounded:      '🏛',
    MetropolisFounded:  '👑',
    OutpostEstablished: '⛺',
    OutpostCaptured:    '⚔',
    TradeDelivery:      '🐂',
    TradeLost:          '🌊',
    TradeReturn:        '🐂',
    MessengerArrival:   '✉',
    UnitAttrition:      '💀',
    UnitDeserted:       '🏃',
    UpkeepUnpaid:       '⚠',
    SubsistenceWarning: '🌾',
    OfferAccepted:      '🤝',
    OfferDeclined:      '🚫',
    OfferExpired:       '⏳',
    ScoutReport:        '🔭',
  };
  return icons[kind] || '◉';
}

// Caps for payloadSummary below: a notif chip is one line, so both a key
// count and a character count are enforced — either one alone can be beaten
// (few keys with huge values, or many keys with tiny ones).
const PAYLOAD_SUMMARY_MAX_KEYS = 6;
const PAYLOAD_SUMMARY_MAX_CHARS = 120;

// Turns an arbitrary notification payload into a short, readable tail —
// "key: value, key: value" — for notifText's default arm (below). Keys are
// sorted so the same notif always reads the same way, never leaning on
// insertion order or json.Marshal's incidental ordering. null/undefined
// values are skipped; nested objects/arrays fall back to JSON.stringify
// rather than printing '[object Object]'. Returns '' for an empty or
// missing payload so the caller can omit the tail entirely.
function payloadSummary(body) {
  if (!body || typeof body !== 'object') return '';
  const keys = Object.keys(body).sort();
  const parts = [];
  let truncated = false;
  for (const key of keys) {
    if (parts.length >= PAYLOAD_SUMMARY_MAX_KEYS) { truncated = true; break; }
    let value = body[key];
    if (value == null) continue;
    if (typeof value === 'number') value = Number.isInteger(value) ? value : Math.round(value);
    else if (typeof value === 'object') value = JSON.stringify(value);
    parts.push(`${key.replace(/_/g, ' ')}: ${value}`);
  }
  if (!parts.length) return '';
  let out = parts.join(', ');
  if (out.length > PAYLOAD_SUMMARY_MAX_CHARS) {
    out = out.slice(0, PAYLOAD_SUMMARY_MAX_CHARS);
    truncated = true;
  }
  return truncated ? out + '…' : out;
}

export function notifText(kind, body) {
  switch (kind) {
    case 'BuildComplete':      return `Build complete: ${body.building_type || ''}`;
    case 'GoodsCrafted': {
      // Payload per ProvinceHandler.Craft: output_key, produced, consumed{good:qty}.
      // Name what went in — casting bronze is the moment the copper/tin chain pays
      // off, and the player should see the trade it made, not just the output.
      // Sort by good name so the line reads the same every time — Go's
      // json.Marshal happens to emit map keys sorted, but don't lean on that.
      const from = Object.entries(body.consumed || {})
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([g, q]) => `${Math.round(q)} ${g}`).join(' + ');
      return `Cast ${Math.round(body.produced || 0)} ${body.output_key || ''}` +
             (from ? ` from ${from}` : '');
    }
    case 'TrainComplete':      return `Training done: ${body.count || ''} ${body.unit_type || ''}`;
    case 'ArmyArrival':        return `Army arrived — ${body.outcome || ''}`;
    case 'ColonyFounded':      return `Colony founded: ${body.name || ''}`;
    case 'MetropolisFounded': {
      // The founder phase's closing line: the one-per-world capital. Catchment
      // knowledge + Poseidon ride along; the grain balance reuses the colony line
      // (colonyFoundedGrainLine reads the same grain_* fields).
      const parts = [`Your metropolis is founded: ${body.name || ''}`];
      if (body.known_hexes != null) parts.push(`${body.known_hexes}/${(body.known_hexes || 0) + (body.unknown_hexes || 0)} catchment hexes known`);
      if (body.poseidon_gift) parts.push('Poseidon grants a galley');
      return parts.join(' — ');
    }
    case 'OutpostEstablished': return 'Outpost established';
    case 'OutpostCaptured':    return 'Enemy outpost captured';
    case 'TradeDelivery':      return `Trade delivered: ${Math.floor(body.quantity || 0)} ${body.good_key || ''}`;
    case 'TradeLost':          return `Caravan lost to ${body.reason || 'misfortune'}`;
    case 'TradeReturn':        return `Trade returned: ${Math.floor(body.quantity || 0)} ${body.good_key || ''}`;
    case 'MessengerArrival':   return body.message || 'Messenger arrived';
    case 'UnitAttrition':      return body.disbanded
                                 ? `${body.unit_type || 'A unit'} starved to nothing — no grain`
                                 : `${body.unit_type || 'A unit'} starving — lost ${body.lost || 0} to hunger`;
    case 'UnitDeserted':       return body.disbanded
                                 ? `${body.unit_type || 'A unit'} deserted — unpaid, unit lost`
                                 : `${body.unit_type || 'A unit'} deserting — ${body.lost || 0} left (unpaid)`;
    case 'UpkeepUnpaid': {
      // Forewarning fired BEFORE desertion starts (SLICE A) — recordUnpaid's
      // else-branch used to bump unpaid_periods with zero player-facing signal
      // until desertion itself fired two speldygn later. periods_until_desertion
      // counts down to the final, urgent period.
      const left = body.periods_until_desertion;
      const tail = left === 1
        ? ' — one more unpaid period and they desert'
        : ` — ${left} periods left before desertion`;
      return `${body.unit_type || 'A unit'} unpaid (period ${body.unpaid_periods || 0})${tail}`;
    }
    case 'SubsistenceWarning': {
      // Payload per kharis.emitSubsistenceWarning: name, tier, net_per_day,
      // days_left, pop_loss. Never a percent — days and grain/day only.
      const name = body.name || 'A settlement';
      if (body.tier === 'critical') {
        return `${name} is STARVING — ${body.pop_loss || 0} citizens lost. Grain ${(body.net_per_day || 0).toFixed(0)}/day.`;
      }
      const days = body.days_left ? ` — grain lasts ~${Math.round(body.days_left)} days` : '';
      return `${name}: grain net ${(body.net_per_day || 0).toFixed(0)}/day${days}`;
    }
    case 'OfferAccepted': {
      // Payload per TradeAccept (messenger.go): good_key/quantity/silver are
      // already kind-branched correctly on the server, unlike Declined/Expired
      // below. Direction-aware ETA tail: for 'sell' the originator is the
      // seller waiting on the silver leg; for 'buy' the originator is the
      // buyer waiting on the goods leg.
      const qty = Math.floor(body.quantity || 0);
      const eta = body.kind === 'sell' ? fmtSoon(body.silver_arrives_at) : fmtSoon(body.goods_arrives_at);
      const tail = eta ? (body.kind === 'sell' ? ` — silver arrives ${eta}` : ` — goods arrive ${eta}`) : '';
      return `Offer accepted: ${qty} ${body.good_key || ''} ⇄ ${body.silver || 0} silver${tail}`;
    }
    case 'OfferDeclined':
    case 'OfferExpired': {
      // NOTE: unlike OfferAccepted, the server's Decline/Expiry handlers
      // (trade.go OfferExpiryHandler, messenger.go TradeDecline) query only
      // offer_good/offer_qty/offer_silver regardless of kind — those three
      // columns only exist in a 'sell' trade_offer JSON; a 'buy' offer stores
      // its good under want_good/want_qty. So for kind==='buy', good_key/
      // quantity in this payload are empty/0 (server-side bug, out of scope
      // for this client change). Sidestepping it: 'sell' escrows GOODS (whose
      // fields ARE correct here), 'buy' escrows SILVER (whose field IS
      // correct here) — so picking by kind shows only the correct half.
      // Refund is immediate (direct settlement_goods credit in the same DB
      // transaction, verified in trade.go) — no caravan, no later TradeReturn.
      const verb = kind === 'OfferDeclined' ? 'declined' : 'expired';
      const refund = body.kind === 'buy'
        ? `${body.silver || 0} silver`
        : `${Math.floor(body.quantity || 0)} ${body.good_key || ''}`;
      return `Offer ${verb}: ${refund} — escrow refunded immediately`;
    }
    case 'ScoutReport': {
      // Payload per UnitScoutReportPayload: q/r/terrain + four deposit bools.
      // The common case is "nothing of value" — that must read as a clean
      // report, not a blank/empty one (temenos_todo.md's own example line).
      const deposits = [];
      if (body.copper_deposit) deposits.push('copper');
      if (body.tin_deposit)    deposits.push('tin');
      if (body.silver_deposit) deposits.push('silver');
      if (body.cedar_deposit)  deposits.push('cedar');
      const found = deposits.length ? deposits.join(', ') : 'nothing of value';
      return `Explored (${body.q}, ${body.r}) — ${body.terrain || 'unknown terrain'}, ${found}`;
    }
    // Every kind above is hand-written; every kind NOT above used to fall
    // through to `return kind`, silently discarding the payload — 17 verified
    // NotifyPlayer kinds have no case here (2026-08-05 audit). keryx never had
    // this problem: cmd_notifications.go always prints the whole body as raw
    // JSON next to the kind, so the web client was structurally quieter than
    // the terminal for exactly the notifs missing a case. This mirrors
    // keryx's "print everything" behaviour in one readable line instead of
    // raw JSON — ugly, but nothing is thrown away. Writing a real case here
    // is a separate, per-kind slice (megaron_todo.md NU §"Striden har ingen
    // informationsyta"); this arm is deliberately generic.
    default: {
      const summary = payloadSummary(body);
      return summary ? `${kind} — ${summary}` : kind;
    }
  }
}

// Mirrors keryx's printColonyFoundedGrainLine (cmd_notifications.go): the
// founding grain balance carried additively in a ColonyFounded body. A colony
// does NOT feed itself automatically, so a founding deficit is surfaced
// immediately, with how long the seed lasts and the two remedies. Returns ''
// for older bodies without grain_net_per_tick (back-compatible).
export function colonyFoundedGrainLine(body) {
  if (!body || body.grain_net_per_tick == null) return '';
  const name = body.name || 'The colony';
  const perDay = body.grain_net_per_tick * 24;
  if (perDay < 0) {
    const days = body.grain_days != null ? ` — grain lasts ~${Math.round(body.grain_days)} days` : '';
    return `${name} does not feed itself (~${Math.round(-perDay)} grain/day deficit)${days}. Build a farm if the land bears it, or send grain by internal transfer.`;
  }
  return `${name} feeds itself (~+${Math.round(perDay)} grain/day).`;
}
