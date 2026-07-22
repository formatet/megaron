// ── Shared formatting helpers ──────────────────────────────────────────────
// Pure functions, no DOM/State deps — safe for any other module to import
// directly regardless of layer (config/state ← api/ws ← render ← ui ← main).
// (clock.js sits on the same low layer, so importing it keeps that promise.)
import { serverNow } from '../clock.js';
export function esc(s) { return (s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); }

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
    SubsistenceWarning: '🌾',
    OfferAccepted:      '🤝',
    OfferDeclined:      '🚫',
    OfferExpired:       '⏳',
  };
  return icons[kind] || '◉';
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
    default:                   return kind;
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
