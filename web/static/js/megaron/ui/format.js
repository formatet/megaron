// ── Shared formatting helpers ──────────────────────────────────────────────
// Pure functions, no DOM/State deps — safe for any other module to import
// directly regardless of layer (config/state ← api/ws ← render ← ui ← main).
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

// Helper: format ETA timestamp to human-readable
export function fmtEta(iso) {
  const ms = new Date(iso).getTime() - Date.now();
  if (ms <= 0) return 'arrived';
  const h = Math.floor(ms / 3600000), m = Math.floor((ms % 3600000) / 60000);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

export function fmtAgo(iso) {
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 60000)   return 'just now';
  if (ms < 3600000) return Math.floor(ms / 60000) + 'm ago';
  if (ms < 86400000) return Math.floor(ms / 3600000) + 'h ago';
  return Math.floor(ms / 86400000) + 'd ago';
}

export function notifIcon(kind) {
  const icons = {
    BuildComplete:      '🏛',
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
  };
  return icons[kind] || '◉';
}

export function notifText(kind, body) {
  switch (kind) {
    case 'BuildComplete':      return `Build complete: ${body.building_type || ''}`;
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
