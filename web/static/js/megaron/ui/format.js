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
    ForeignMarchSighted: '🛡',
    SubsistenceWarning: '🌾',
    OfferAccepted:      '🤝',
    OfferDeclined:      '🚫',
    OfferExpired:       '⏳',
    ScoutReport:        '🔭',
    BattleWon:          '⚔',
    BattleLost:         '⚔',
    ShipDamaged:        '⛵',
    ShipRepaired:       '🔨',
    DivinePunishment:   '⚡',
    DivineBlessing:     '✨',
    FoodShortfall:      '🍽',
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
    case 'ForeignMarchSighted': {
      // The notice that starts the clock in the asynchronicity gate: a foreign
      // march entered this Wanax's live tier. Says WHO, WHERE IT IS GOING and
      // WHICH TICK IT LANDS — the tick because that is what the player plans in
      // (speldygn, never wall clock), and because the whole point is that there
      // is still travel time left to answer it.
      const owner = body.owner ? `${body.owner}'s` : 'An unknown';
      const force = body.unit_type || 'force';
      const size = body.size ? ` (${body.size})` : '';
      const lands = body.arrive_tick ? ` — lands tick ${body.arrive_tick}` : '';
      // The threatened city is the urgent end of the irreversibility gradient
      // and must not read like a march merely passing through.
      if (body.threatens_name) {
        return `${owner} ${force}${size} is marching on ${body.threatens_name}${lands}`;
      }
      return `${owner} ${force}${size} sighted marching to (${body.target_q},${body.target_r})${lands}`;
    }
    case 'SubsistenceWarning': {
      // Payload per kharis.emitSubsistenceWarning: name, tier, net_per_tick,
      // ticks_left, pop_loss. net_per_day/days_left are the pre-rename keys
      // (2026-08-06) — the server keeps writing them alongside the new ones so
      // old persisted notifications stay readable; prefer the new keys and
      // fall back to the old. Never a percent — ticks and grain/tick only.
      const name = body.name || 'A settlement';
      const netPerTick = body.net_per_tick ?? body.net_per_day;
      const ticksLeft = body.ticks_left ?? body.days_left;
      if (body.tier === 'critical') {
        return `${name} is STARVING — ${body.pop_loss || 0} citizens lost. Grain ${(netPerTick || 0).toFixed(0)}/tick.`;
      }
      const ticks = ticksLeft ? ` — grain lasts ~${Math.round(ticksLeft)} ticks` : '';
      return `${name}: grain net ${(netPerTick || 0).toFixed(0)}/tick${ticks}`;
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
    case 'BattleWon':
    case 'BattleLost': {
      // Payload per BattleTickHandler.notifyBattleEnded (megaron_plan_stridsrapport.md
      // §4/S5) — the KR3 battle-end notification, sent to EVERY owner on both
      // sides (a field battle used to be totally silent; a defender got nothing
      // at all). own_unit/enemy_unit are aggregated across every unit that owner
      // had in the battle, not a single unit — a battle can hold several units
      // per side once the other entry points are cut over (plan §8).
      const own = body.own_unit || {};
      const enemy = body.enemy_unit || {};
      const place = body.place || `(${body.q}, ${body.r})`;
      const outcomeWord = kind === 'BattleWon' ? 'Victory' : 'Defeat';
      let trailer = '';
      if (body.outcome === 'mutual_wipe') trailer = ' No survivors on either side.';
      else if (body.outcome === 'attacker_wins' && body.role === 'attacker') trailer = ' The field was taken.';
      else if (body.outcome === 'attacker_wins' && body.role === 'defender') trailer = ' The field was lost.';
      else if (body.outcome === 'defender_holds' && body.role === 'defender') trailer = ' The field held.';
      else if (body.outcome === 'defender_holds' && body.role === 'attacker') trailer = ' The attack was repelled.';
      return `${outcomeWord} at ${place} — your ${own.type || 'unit'} (${own.size_before ?? '?'}→${own.size_after ?? '?'}, ` +
             `−${own.pop_lost ?? 0} dead) vs ${body.opponent_name || 'an enemy'}'s ${enemy.type || 'unit'} ` +
             `(${enemy.size_before ?? '?'}→${enemy.size_after ?? '?'}).${trailer}`;
    }
    case 'ShipDamaged': {
      // Payload per BattleTickHandler.notifyShipDamaged (megaron_plan_
      // skeppsreparation.md Slice B point 6): hull is the OUTCOME value after
      // the draw (0 ⇒ sunk is also true). returning_home is only ever true
      // for a routed side's survivor — the winning side's damaged ships keep
      // their orders and never carry this flag, even at the same hull loss.
      if (body.sunk) return `Your ${body.unit_type || 'ship'} was sunk in battle`;
      const hull = `hull ${body.hull ?? '?'}/${body.hull_max ?? 5}`;
      return body.returning_home
        ? `Your ${body.unit_type || 'ship'} took damage (${hull}) and is limping home for repair`
        : `Your ${body.unit_type || 'ship'} took damage (${hull}) but holds its orders`;
    }
    case 'ShipRepaired': {
      // Payload per ShipRepairCompleteHandler (megaron_plan_skeppsreparation.md
      // Slice C point 4) — hull is always hull_max here, the repair job's outcome.
      return `Your ${body.unit_type || 'ship'} is repaired (hull ${body.hull ?? 5}/5) and ready to sail`;
    }
    case 'CityOccupied': {
      // Payload per occupation.go's occupySettlement (megaron_plan_erovring.md
      // S1/S6) — the erövring victory notification. role distinguishes the
      // occupying attacker (offered the sack/burn/occupy choice — annex comes
      // later, once occupation_ticks_to_annex have passed unchallenged) from
      // the dispossessed defender (the city is not lost yet — a relief force
      // can still reset the occupant's counter).
      const ticks = body.occupation_ticks_to_annex;
      if (body.role === 'attacker') {
        return `${body.name || 'The city'} has fallen — your army holds it under occupation. ` +
               `Doing nothing leaves it occupied; annex becomes possible after ${ticks ?? '?'} unchallenged ticks.`;
      }
      return `${body.name || 'Your city'} has fallen under occupation — not lost yet. ` +
             `A relief force within ${ticks ?? '?'} ticks resets the enemy's countdown.`;
    }
    case 'OccupationDefended':
      return `The occupation of ${body.name || 'the city'} held off an attack — the annex countdown reset ` +
             `(${body.occupation_ticks_to_annex ?? '?'} ticks needed again).`;
    case 'CityAnnexReady':
      return `${body.name || 'The city'} has stood unchallenged long enough — you may annex it now.`;
    case 'SettlementLooted': {
      if (body.role === 'attacker') return `${body.name || 'The city'} was sacked — the loot is on its way home as a caravan (it can be intercepted).`;
      const hit = body.building_hit ? `, its ${body.building_hit} knocked down a level` : '';
      return `${body.name || 'Your city'} was sacked — population −⅓${hit}. It stands, weakened.`;
    }
    case 'SettlementBurned': {
      if (body.role === 'attacker') return `${body.name || 'The city'} was sacked and burned — the loot is on its way home; the ruin cannot be recolonized for a time.`;
      return `${body.name || 'Your city'} was sacked and burned — it is a ruin now.`;
    }
    case 'SettlementCaptured':
      // Payload (occupation.go's annex path, unit_arrival.go's land-march annex)
      // carries only settlement_id + role — no name survives to either side.
      return body.role === 'attacker'
        ? 'Your army has taken an enemy settlement'
        : 'One of your settlements has fallen to conquest';
    case 'SettlementDefended': {
      const outcomes = {
        attacker_destroyed: 'the attacking army was destroyed',
        attacker_routed:    'the attacking army was routed',
        attacker_beaten:    'the attacking army was beaten back',
      };
      const line = outcomes[body.outcome] || 'the attack was repelled';
      return body.role === 'attacker'
        ? `Your assault failed — ${line}`
        : `Your settlement held — ${line}`;
    }
    case 'SettlementSacked': {
      if (body.role === 'attacker') {
        const looted = Object.entries(body.looted || {})
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([g, q]) => `${Math.round(q)} ${g}`).join(', ');
        return `You sacked and razed a settlement${looted ? ` — looted ${looted}` : ''}`;
      }
      return 'Your settlement was sacked and razed';
    }
    case 'CityCollapsed': {
      const name = body.name || 'A settlement';
      return body.last_settlement
        ? `${name} has collapsed — that was your last settlement`
        : `${name} has collapsed`;
    }
    case 'UnitLostAtSea': {
      const cause = body.reason === 'silver_shortage' ? 'the ship went unpaid' : 'the ship starved';
      return `${body.unit_type || 'A unit'} lost at sea — ${cause}, ${body.lost || 0} men gone`;
    }
    case 'CaravanSeized':   return `You seized an enemy caravan at (${body.q}, ${body.r})`;
    case 'CaravanRaided':   return `Your caravan was raided at (${body.q}, ${body.r})`;
    case 'MarchStalled':    return body.reason || 'A march could not be processed';
    case 'UnitArrived': {
      const type = body.type ? `${body.type} ` : 'A unit ';
      const stance = body.stance ? `, standing ${body.stance}` : '';
      return `${type}arrived at (${body.q}, ${body.r}) — ${body.status || 'positioned'}${stance}`;
    }
    case 'UnitExploreReturned': {
      const eta = fmtSoon(body.arrives_at);
      return `Scout returning home${eta ? ` — arrives ${eta}` : ''}`;
    }
    case 'UnitReturnedStarving': {
      // Payload per dispatchReturnHome's returnReasonStarvation branch
      // (megaron_plan_svaltretur_till_sjoss.md) — a ship at sea receives no
      // orders, so this turn-for-home is the unit's own decision, not one the
      // Wanax gave. Text must say why (crew starved to half strength) and what
      // it costs (a thinned crew sails slower), never reuse the scout's text.
      const eta = fmtSoon(body.arrives_at);
      const crew = body.crew_after != null ? ` (crew down to ${body.crew_after})` : '';
      return `Ship's crew starved to half strength${crew} — turning home on its own, sailing slower${eta ? `, arrives ${eta}` : ''}`;
    }
    case 'OrderFailed':
      return body.reason ? `Order failed (${body.verb || '?'}): ${body.reason}` : `Order failed (${body.verb || '?'})`;
    case 'UnitRecalled':
    case 'UnitRedirected': {
      const eta = fmtSoon(body.arrives_at);
      const verb = kind === 'UnitRecalled' ? 'recalled' : 'redirected';
      return `Unit ${verb} — new course to (${body.target_q}, ${body.target_r})${eta ? `, arrives ${eta}` : ''}`;
    }
    case 'SitosGranaryRelease': {
      const empty = body.granary_empty ? ' — granary now empty' : '';
      return `Granary released ${Math.round(body.food_released || 0)} grain (${body.coverage_days || 0} days' coverage)${empty}`;
    }
    case 'TransferDelivered': {
      const goods = (body.goods || [])
        .map(g => `${Math.round(g.quantity || 0)} ${g.good_key}`).join(', ');
      return `Transfer delivered to ${body.dest_name || 'destination'}${goods ? `: ${goods}` : ''}`;
    }
    case 'SentryAlerted':
      return `Sentry spotted ${body.foreign_owner || 'an unknown'}'s ${body.foreign_type || 'unit'} at (${body.q}, ${body.r})`;
    case 'DivinePunishment': {
      // Payload: type/amount/name (megaron_plan_tre_tysta_notiserna.md — name
      // is additive, older persisted notifications may lack it). amount's UNIT
      // depends on type — a generic "+N" would be wrong for harvest_failure
      // (grain) vs the three man/ship-counting types.
      const place = body.name ? ` at ${body.name}` : '';
      const amount = body.amount || 0;
      switch (body.type) {
        case 'chariot_loss':    return `The gods scattered your war chariots in the night${place} — ${amount} lost`;
        case 'ship_loss':       return `A divine storm claimed a vessel from your harbour${place}`;
        case 'harvest_failure': return `The fields lay fallow by divine will${place} — ${Math.round(amount)} grain rotted`;
        case 'garrison_plague': return `A pestilence swept the barracks${place} — ${amount} fell`;
        default:                return `${kind} — ${payloadSummary(body)}`;
      }
    }
    case 'DivineBlessing': {
      const place = body.name ? ` at ${body.name}` : '';
      const amount = body.amount || 0;
      switch (body.type) {
        case 'harvest_blessing': return `The gods fill your granaries${place} — +${Math.round(amount)} grain`;
        case 'divine_recruits':  return `Warriors answer a divine call${place} — +${amount} men`;
        case 'sea_blessing':     return `Poseidon guides a vessel to your harbour${place} — +${amount} galley`;
        default:                 return `${kind} — ${payloadSummary(body)}`;
      }
    }
    case 'FoodShortfall': {
      // unmet is the day's UNFULFILLED ration, not population lost (that is
      // SubsistenceWarning's pop_loss — a different mechanic, don't conflate).
      const place = body.name || 'A settlement';
      return `${place} went hungry today — ${Math.round(body.unmet || 0)} grain unmet`;
    }
    // Every kind above is hand-written. This arm stays as a generic fallback
    // for any future NotifyPlayer kind added without a case here — the 2026-08-05
    // audit found 17 such kinds; all now have real text (A7, megaron_mvp_mandag.md
    // §A7). keryx never had this problem: cmd_notifications.go always prints the
    // whole body as raw JSON next to the kind.
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
// grain_ticks/grain_days: grain_days is the pre-rename key (2026-08-06) — the
// server keeps writing it alongside grain_ticks so old persisted
// notifications stay readable; prefer grain_ticks and fall back to grain_days.
export function colonyFoundedGrainLine(body) {
  if (!body || body.grain_net_per_tick == null) return '';
  const name = body.name || 'The colony';
  // grain_net_per_tick IS the per-tick rate now (tick == day, mig 109) — do
  // not scale it, that was the ×24 error found alongside cmd_goods.go's.
  const perTick = body.grain_net_per_tick;
  const ticksLeft = body.grain_ticks ?? body.grain_days;
  if (perTick < 0) {
    const ticks = ticksLeft != null ? ` — grain lasts ~${Math.round(ticksLeft)} ticks` : '';
    return `${name} does not feed itself (~${Math.round(-perTick)} grain/tick deficit)${ticks}. Build a farm if the land bears it, or send grain by internal transfer.`;
  }
  return `${name} feeds itself (~+${Math.round(perTick)} grain/tick).`;
}
