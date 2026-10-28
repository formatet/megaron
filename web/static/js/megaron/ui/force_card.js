import { State, ownCapital } from '../state.js';
import { serverNow } from '../clock.js';
import { esc } from './format.js';
import { fmtArrival, arrivalHTML } from './time.js';
import { actorName, unitTypeLabel } from './actornames.js';
import { foodGoodsText } from './food.js';
import { fmtNum, fmtDays } from './fmt_num.js';
import { fmtTickTime } from './fmt_clock.js';
import { canTakeStance } from './stance.js';
import { unitRetreatControlHTML } from './retreat.js';
import { expeditionMissionText } from './expedition.js';
import { orderRunnerHTML } from './runner_status.js';
import { roadPanelHTML } from './roadorder.js';
export function readyWord(iso) {
  const eta = fmtArrival(iso, undefined, 'ready');
  return !eta || eta === 'ready' ? eta : 'ready ' + eta;
}

export function unitStatusLabel(u) {
  const naval = u.category === 'naval';
  return ({
    garrison: naval ? 'in harbour' : 'in the city',
    positioned: naval ? 'off the coast' : 'in the field',
    marching: naval ? 'at sea' : 'on the march',
    forming: naval ? 'being built' : 'gathering men',
    training: 'training', embarked: 'aboard a ship', repairing: 'being repaired',
    freighting: 'carrying goods', laid_up: 'Laid up', fitting_out: 'fitting out', disbanded: 'dismissed',
  })[u.status] || 'awaiting news';
}

// roadFromHex: the hex an army would build a road from — where it stands in the
// field, or its city's hex when garrisoned. null when the client cannot tell.
function roadFromHex(u) {
  if (u.status === 'positioned' && u.q != null && u.r != null) return { q: u.q, r: u.r };
  if (u.status === 'garrison' && u.settlement_id) {
    const p = (State.provinceData || []).find(x => x.settlement_id === u.settlement_id);
    if (p) return { q: p.q, r: p.r };
  }
  return null;
}

export function renderUnitCard(u, options = {}) {
  const hex = options.context === 'hex';
  const lbl = esc(actorName(u));
  const isNaval = u.category === 'naval';
  const isForming = u.status === 'forming';
  const isTraining = u.status === 'training';
  const isGarrison = u.status === 'garrison';
  const isPositioned = u.status === 'positioned';
  const isMarching = u.status === 'marching';
  const isEmbarked = u.status === 'embarked';
  // Sjöhandel kräver skepp (megaron_plan_sjohandel_kraver_skepp.md R2/R4): a
  // ship bound to a naval transfer or standing sea route — not deployable,
  // not drawn on the map (the transport/route is), parked at the settlement
  // it was bound from.
  const isFreighting = u.status === 'freighting';
	const isLaidUp = u.status === 'laid_up';
	const isFittingOut = u.status === 'fitting_out';

  // Location string
  let loc = '';
  if (isFreighting) {
    loc = 'Carrying goods — ' + (u.freighting_note ? esc(u.freighting_note) : 'bound to a sea route');
  } else if (isGarrison || isForming || isTraining || isEmbarked || isLaidUp || isFittingOut) {
    const prov = State.provinceData.find(p => p.settlement_id === u.settlement_id || p.id === u.settlement_id);
    const place = prov ? esc(prov.name) : 'city';
    // A galley idle at its coastal settlement is docked — say it is "in harbour"
    // rather than just naming the city (a land garrison is self-evidently in the
    // city; a ship being IN PORT vs at sea is the meaningful distinction).
    loc = (isNaval && isGarrison) ? '⚓ in harbour — ' + place : place;
  } else if (isPositioned && u.march_intent === 'pickup_wait' && u.q != null) {
    // Pickup wait (megaron_plan_hamta_hem.md, slice 2b): the ship holds off
    // the shore for the fetched unit — ScheduledPickupTimeout is what sends
    // it home if the unit never makes it.
    const who = u.pickup_for ? esc(u.pickup_for) : 'the unit';
    const until = u.waiting_until_tick != null ? ' until ' + fmtTickTime(u.waiting_until_tick) : '';
    loc = 'waiting off (' + u.q + ',' + u.r + ') for ' + who + until;
  } else if (isMarching && u.target_q != null) {
    // Pickup (megaron_plan_hamta_hem.md): sailing to fetch a unit — target_q/r
    // is the sea waypoint, not the shore itself.
    const pickupPrefix = u.march_intent === 'pickup'
      ? 'sailing to fetch ' + (u.pickup_for ? esc(u.pickup_for) : 'a unit') + ' — '
      : '';
    // arrival_tick is the authoritative arrival (K4) — the stored arrives_at
    // stamp lies across server downtime; the tick self-corrects.
    const arrival = arrivalHTML(u.arrives_at, u.arrival_tick);
    loc = pickupPrefix + '→ (' + u.target_q + ',' + u.target_r + ')' + (arrival ? ' arrives ' + arrival : '');
  } else if (u.q != null) {
    loc = '(' + u.q + ',' + u.r + ')';
  }

  if (u.bound_route_name) loc = 'on the ' + esc(u.bound_route_name) + ' route' + (loc ? ' · ' + loc : '');

  // Progress: naval forming shows the build ETA (size is always 1); land forming
  // shows the men/100 gathering bar; land training shows a full bar + ready ETA
  // (it has all 100 men and is maturing to a deployable garrison).
  const bar = (pct) => '<div style="margin:.2rem 0;background:var(--border);height:4px;border-radius:2px"><div style="background:var(--accent-war);height:4px;width:' + pct + '%"></div></div>';
  const dim = (txt) => '<span style="font-size:.65rem;color:var(--text-dim)">' + txt + '</span>';
  let progress = '';
  if (u.shipyard_waiting) {
    progress = dim('waiting for shipyard workers');
  } else if (u.status === 'repairing') {
    progress = dim('being repaired — ' + readyWord(u.build_complete_at));
  } else if (isNaval && isForming) {
    progress = dim('building — ' + readyWord(u.build_complete_at));
  } else if (isForming) {
    // men_to_deploy is server-computed (100 − size); recruiting more of the
    // same type into this settlement is what fills it — say so, so a
    // half-formed unit doesn't read as a stuck pipeline.
    const needed = u.men_to_deploy != null ? u.men_to_deploy : (100 - u.size);
    progress = bar(u.size) + dim(fmtNum(u.size) + ' of 100 · ' + fmtNum(needed) + ' more men needed before training starts');
  } else if (isTraining) {
    progress = bar(100) + dim('100 men · training — ' + readyWord(u.build_complete_at));
  } else if (isFittingOut) {
    progress = dim('crew aboard · fitting out — ' + readyWord(u.build_complete_at));
  } else if (isGarrison && u.reinforcing) {
    // Manskaps-underhåll (megaron_plan_rekryteringsmodell.md): a decimated
    // cohort trickles back to 100 out of its home city's population growth,
    // a few men per game-day — not a stuck pipeline, just slow by design.
    // (No separate origin-city name field on the wire — the server exposes
    // origin_settlement_id, not a name, so this stays generic.)
    progress = bar(u.size) + dim(fmtNum(u.size) + ' of 100 · reinforcing from home-city growth');
  }

  // The server owns current strength and field ration rates. Zero silver is
  // a value, not a missing field; never derive soldier pay in the client.
  const upkeepLabel = isLaidUp ? 'Ship upkeep while laid up'
    : u.upkeep_context === 'inactive' ? 'Upkeep when active'
    : u.upkeep_context === 'subsistence' ? 'Upkeep after settling' : 'Upkeep';
  const foodRate = u.upkeep_food_per_tick ?? u.upkeep_grain_per_tick;
  const foodLabel = u.upkeep_food_per_tick != null ? 'food' : 'grain';
  const upkeep = u.upkeep_context !== 'none' && Number.isFinite(foodRate) && Number.isFinite(u.upkeep_silver_per_tick)
    ? '<div class="unit-upkeep" style="font-size:.65rem;color:var(--text-dim)">' + upkeepLabel + ': '
      + foodRate.toLocaleString('en-US', {maximumFractionDigits: 4}) + ' ' + foodLabel + ', '
      + u.upkeep_silver_per_tick.toLocaleString('en-US', {maximumFractionDigits: 4}) + ' silver per game day</div>'
    : '';

  // Pending order (Fas 5): a Runner is en route to this unit — the order
  // executes only on delivery; surface the courier ETA on the card.
  const runner = (State.messengerData || []).find(m => m.own && m.kind === 'order' && m.order_unit_id === u.id);
  const pendingOrder = runner ? orderRunnerHTML(runner, serverNow()) : (u.orders_en_route || []).map(o => '<p class="force-order-note">Messenger carrying ' + esc(o.verb || 'order') + (o.target_q != null ? ' toward (' + o.target_q + ',' + o.target_r + ')' : '') + (o.following_wait ? ' — waiting for the force' : o.following_unit ? ' — following the force' : o.arrives_at ? ' — ' + arrivalHTML(o.arrives_at) : '') + '</p>').join('');

  // Stance badge
  const stanceBadge = u.stance
    ? '<span style="font-size:.6rem;padding:.1rem .25rem;border:1px solid var(--border);color:var(--text-dim);margin-left:.2rem">' + esc(u.stance) + '</span>'
    : '';

  // Crew badge for naval
  const crewBadge = isNaval && u.crew
    ? '<span style="font-size:.6rem;color:var(--text-dim);margin-left:.3rem">' + (isLaidUp ? 'crew ashore ' : 'crew ') + fmtNum(u.crew) + '</span>'
    : '';

  // Hull badge (megaron_plan_skeppsreparation.md §B2) — only shown while
  // damaged (hull < 5); a pristine ship (hull omitted or 5) shows nothing,
  // same "don't clutter the common case" posture as crewBadge above.
  const hullBadge = isNaval && u.hull != null && (hex || u.hull < 5)
    ? '<span style="font-size:.6rem;color:var(' + (u.hull < 5 ? '--accent-war' : '--text-dim') + ');margin-left:.3rem">hull ' + fmtNum(u.hull) + '/5</span>'
    : '';

  // Matmätaren (megaron_plan_skeppsproviant.md §7, Timothy 2026-08-26). Dygn,
  // inte råa korn — resten av spelet mäts i speldygn.
  //
  // Döljs i hamn med flit: där äter skeppet ur stadens magasin, så ett lager
  // ombord säger ingenting. Till sjöss visas den ALLTID, inte bara när den är
  // låg — till skillnad från hull-bricka ovan. Skälet är att det här är talet
  // som avgör om du kan ge en order alls, och ett skepp som tyst närmar sig
  // noll är precis det mekaniken finns för att göra synligt.
  // Freight ships consume their own provisions too, including waits between
  // legs. Display the same server projection as marching/positioned ships.
  const atSea = isNaval && (isMarching || isPositioned || isFreighting);
  const days = u.provision_days || 0;
  const foodDetail = foodGoodsText(u.provisions_by_good);
  const foodBadge = atSea
    ? '<span style="font-size:.6rem;margin-left:.3rem;color:' +
      (days <= 0 ? 'var(--accent-war)' : days < 3 ? 'var(--accent-war)' : 'var(--text-dim)') + '">' +
      (days > 0 ? 'food for ' + fmtDays(days) : u.provisions_aboard ? 'food for less than a game day' : 'out of food') + (foodDetail ? ' (' + foodDetail + ')' : '') + '</span>'
    : '';

  // Cargo badge
  // Passengers: the server lists every land unit aboard (an Emporos carries
  // up to three) and the hull's troop capacity; the client never re-derives it.
  const aboard = u.cargo_units || (u.cargo_unit_id ? [{ id: u.cargo_unit_id }] : []);
  const troopCap = Number(u.troop_capacity) || 1;
  const cargoBadge = aboard.length
    ? '<span style="font-size:.6rem;color:var(--accent-city-text);margin-left:.3rem">Passengers ' + aboard.length + ' / ' + (troopCap || aboard.length) + ': '
      + aboard.map(p => esc(p.display_name || p.type || 'unit') + (p.size ? ' (' + fmtNum(p.size) + ')' : '')).join(', ') + '</span>'
    : '';

  // Action buttons
  let actions = '';

  // March button: garrison or positioned, deployable per the
  // server's own march grind (march_start.go:120-136 — status must be garrison
  // or positioned, and fortify stance blocks it). The server has NO size gate:
  // a battle-worn cohort below 100 men can still march, so the client must not
  // invent one either — u.deployable is the field the server already computes
  // (`status != forming && status != training`, api/handlers/unit.go:1342).
  // Positioned units (out on the map, e.g. a ship that finished a plain march)
  // must be orderable too — otherwise they're stranded. The server already
  // allows marching a positioned unit; this just surfaces the button. (The
  // map right-click used to read the unit's own hex as the target — fixed by
  // routing that click to warFocusUnit, landing here, render/map.js contextmenu.)
  // R3: ships off the coast take no new orders; R6 preserves the stranded
  // exception when the Wanax has no city left (RequireShipInPort).
  const canMarch = (isGarrison || (isPositioned && (!isNaval || !ownCapital())))
    && u.deployable && u.stance !== 'fortify';
  if (canMarch) {
    actions += '<button onclick="unitMarch(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">March</button> ';
  }

  // Stance buttons: land only — naval units carry no stance (ship-build
  // overhaul 2026-07-09). A marching unit takes one too: the Runner must catch
  // up with it (megaron_styrande_beslut §11, canTakeStance mirrors the server).
  if (canTakeStance(u)) {
    actions += '<select id="ustance-' + u.id + '" style="font-size:.65rem;padding:.1rem;border:1px solid var(--border);background:var(--warm-white)">'
      + '<option value="none">stance…</option>'
      + '<option value="fortify">fortify</option>'
      + '<option value="storm">storm</option>'
      + '<option value="sentry">sentry</option>'
      + (u.stance ? '<option value="none">— clear</option>' : '')
      + '</select> '
      + '<button onclick="unitStance(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Set</button> ';

    // Retreat order (KR3 §5): the per-unit override of the realm-wide
    // setting, for the current battle only. Shown only while the unit is
    // fighting (in_battle on the units list — the same condition
    // SetStandingOrders checks); outside battle it could only ever answer
    // "unit is not in an active battle", and the realm-wide setting at the
    // top of this tab is what applies.
    if (isGarrison || isPositioned) actions += unitRetreatControlHTML(u);
  }

  if (u.can_cancel_training) {
    actions += '<button onclick="unitCancelTraining(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">' + (isNaval ? 'Cancel build' : 'Cancel training') + '</button> ';
  }

  // Reinforce button (megaron_plan_rekryteringsmodell.md): only when the
  // server says this cohort can actually receive it — garrisoned in its own
  // origin city and under 100 men (can_reinforce mirrors the endpoint's own
  // gate exactly, api/handlers/unit.go Reinforce). Hidden once reinforcing is
  // already true — the progress bar above says the same thing, a second
  // button to press would just re-flip a flag that's already set.
  if (isGarrison && u.can_reinforce && !u.reinforcing) {
    actions += '<button onclick="unitReinforce(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Reinforce</button> ';
  }

  // Load button: naval garrison without cargo — pick from co-located garrison land units
  if (isNaval && isGarrison && aboard.length < troopCap && u.settlement_id) {
    actions += '<button onclick="unitLoadPrompt(\'' + u.id + '\',\'' + (u.settlement_id||'') + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Load</button> ';
  }

  // Unload button: naval garrison with cargo
  if (isNaval && isGarrison && aboard.length) {
    actions += '<button onclick="unitUnload(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">' + (aboard.length > 1 ? 'Unload all' : 'Unload') + '</button> ';
  }

  // Fetch by ship (R1, megaron_plan_hamta_hem.md, slice 2b): a field-
  // positioned own land unit the server says can be fetched — can_fetch_by_
  // ship and pickup_ships are computed server-side (attachPickupNotes),
  // never re-derived here (megaron_arbetssatt: the client must never promise
  // an action the server cannot perform).
  let pickupRow = '';
  if (isPositioned && !isNaval && u.can_fetch_by_ship && (u.pickup_ships || []).length) {
    actions += '<button onclick="unitPickupToggle(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Fetch by ship</button> ';
    const shipOptions = u.pickup_ships.map(s =>
      '<option value="' + s.id + '">' + esc(s.name) + ' (' + esc(s.settlement_name) + ')' +
      (s.can_carry_runner ? '' : ' — war galley, must already be on the shore') + '</option>'
    ).join('');
    pickupRow = '<div id="upick-' + u.id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
      + '<select id="upick-ship-' + u.id + '" style="font-size:.65rem;padding:.1rem;border:1px solid var(--border);background:var(--warm-white)">' + shipOptions + '</select> '
      + '<label>wait <input id="upick-wait-' + u.id + '" type="number" min="1" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem" placeholder="default"></label> '
      + '<button onclick="unitPickup(\'' + u.id + '\')" style="padding:.1rem .3rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.65rem;cursor:pointer">Send</button>'
      + '<div id="upick-res-' + u.id + '" style="margin-top:.15rem"></div>'
      + '</div>';
  }

  // Build road (megaron_plan_vagar.md V2): a land army standing in the field or
  // in a city may be ordered to build ONE road edge. The server judges terrain,
  // stone and route; the panel only lists neighbours the map shows can carry a road.
  let roadRow = '';
  if (!isNaval && u.type !== 'nomadic_host' && (isPositioned || isGarrison)) {
    const roadFrom = roadFromHex(u);
    if (roadFrom) {
      actions += '<button onclick="unitRoadToggle(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Build road</button> ';
      roadRow = roadPanelHTML(u.id, roadFrom, State.tileData, State.roadEdges || []);
    }
  }

  // Repair button (megaron_plan_skeppsreparation.md Slice C): naval garrison
  // with hull < 5. The server rejects it if the settlement has no shipyard
  // or the yard is full — this button only knows the ship is damaged and
  // docked, same "let the server be the judge" posture as Load/Unload.
  if (isNaval && isGarrison && u.hull != null && u.hull < 5) {
    actions += '<button onclick="unitRepair(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Repair</button> ';
  }
	if (isNaval && u.can_lay_up) {
		actions += '<button onclick="unitLayUp(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Lay up</button> ';
	}
	if (isNaval && isLaidUp) {
		if (u.can_fit_out) actions += '<button onclick="unitFitOut(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Fit out</button> ';
		else actions += '<button disabled title="Needs enough people in your active harbour" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;opacity:.55">Fit out</button> ';
	}

  // Recall/redirect: marching LAND units only. A marching ship is on a
  // mission with a built-in return leg and takes no orders at all (R3,
  // megaron_plan_skeppsuppdrag_landsatt.md) — the server would refuse both
  // buttons, so the semantic grind forbids offering them (UI must not
  // promise a command the game cannot deliver). The order travels by
  // messenger for land units — it does not apply instantly (temenos_
  // settlement.md load-bearing pillar). Redirect's primary path is now the
  // right-click march menu (Timothy 2026-09-25) — it lists this same
  // marching unit and needs no typed coordinates. The button here just
  // points there; typed Q/R survives as a no-cost fallback behind a link,
  // for whoever prefers it or is off-map.
  let redirectRow = '';
  if (isMarching && !isNaval) {
    actions += '<button onclick="unitRecall(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Recall</button> ';
    actions += '<button onclick="unitRedirectToggle(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Redirect</button> ';
    redirectRow = '<div id="uredir-' + u.id + '" style="display:none;margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">'
      + '<div>Right-click the new destination on the map — a messenger carries the order to this unit.</div>'
      + '<div style="margin-top:.2rem"><a href="#" onclick="unitRedirectTypedToggle(\'' + u.id + '\');return false" style="color:var(--text-dim)">or type coordinates</a></div>'
      + '<div id="uredir-typed-' + u.id + '" style="display:none;margin-top:.2rem;gap:.25rem;align-items:center">'
        + '<label>Q <input id="uredir-q-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
        + '<label>R <input id="uredir-r-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
        + '<button onclick="unitRedirect(\'' + u.id + '\')" style="padding:.1rem .3rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.65rem;cursor:pointer">Send order →</button>'
      + '</div>'
      + '</div>';
  } else if (isMarching && isNaval) {
    const mission = u.march_intent === 'dock'
      ? 'Sailing to your city — the ship docks and stays. Everyone aboard joins the city’s garrison. No orders reach it at sea.'
      : 'On its mission — returns to port automatically, no orders reach it at sea.';
    redirectRow = '<div style="margin-top:.2rem;font-size:.65rem;color:var(--text-dim)">' + mission + '</div>';
  }
  if (hex) {
    actions = canMarch ? '<button data-hex-march="' + esc(u.id) + '">March</button>' : '';
    if (isMarching && !isNaval) actions += '<button data-hex-redirect="' + esc(u.id) + '">Redirect</button><button data-hex-recall="' + esc(u.id) + '">Recall</button>';
    if (isMarching && !isNaval) redirectRow = '<p class="force-order-note">A messenger carries the order to this force.</p>';
    pickupRow = ''; roadRow = '';
  }
  const orderStatus = '<div id="' + (hex ? 'hex-' : '') + 'uorder-' + u.id + '" style="font-size:.65rem;color:var(--text-dim);margin-top:.15rem"></div>';

  return '<div class="force-card" id="' + (hex ? 'hex-' : '') + 'ucard-' + u.id + '" style="padding:.3rem .2rem;border-bottom:1px solid var(--border)">'
    + '<div style="display:flex;align-items:center;gap:.3rem;flex-wrap:wrap">'
    + (hex ? '<button class="force-name-link" data-unit-id="' + esc(u.id) + '">View in Forces →</button>' : '')
    + '<a class="cx-link force-codex-name" href="#" data-cx-kind="unit" data-cx-key="'+esc(u.type)+'">' + lbl + '</a>'
    + stanceBadge + crewBadge + hullBadge + foodBadge + cargoBadge
    + '<span style="font-size:.68rem;color:var(--text-dim);margin-left:auto">' + esc(unitStatusLabel(u)) + '</span>'
    + '</div>'
    + (hex ? '<p class="force-strength">' + esc(unitTypeLabel(u.type)) + ' ×' + fmtNum(u.size) + '</p>' : '')
    + progress
    + upkeep
    + (loc ? '<div style="font-size:.65rem;color:var(--text-dim)">' + loc + '</div>' : '')
    + (u.expedition ? '<div style="font-size:.72rem;color:var(--text-dim)">' + esc(expeditionMissionText(u.expedition)) + '</div>' : '')
    + forceGoodsHTML(u)
    + pendingOrder
    + (actions ? '<div style="margin-top:.2rem;display:flex;gap:.2rem;flex-wrap:wrap;align-items:center">' + actions + '</div>' : '')
    + redirectRow + pickupRow + roadRow + orderStatus
    + '</div>';
}


function forceGoodsHTML(u) {
 const loads=(State.tradeData||[]).filter(t=>t.mine===true&&t.ship_unit_id===u.id);
 return loads.map(t=>'<p class="force-cargo">Goods: '+(t.items||[]).map(i=>esc(i.good_key)+' '+fmtNum(i.quantity)).join(', ')+'</p>').join('');
}

// Whitelist exactly the current foreign-units API. Own-force fields must never
// turn up here, even if a caller supplies an object containing private values.
export function renderForeignUnitCard(u) {
 const cargo=u.cargo ? esc(unitTypeLabel(u.cargo.type))+' ×'+fmtNum(u.cargo.size)+(u.cargo.units>1?' in '+fmtNum(u.cargo.units)+' cohorts':'') : 'unknown';
 const apparent=u.toward ? '<p>Seems bound toward '+esc(u.toward.name)+' — if so, '+arrivalHTML(u.toward.eta_at)+'</p>' : '';
 return '<div class="force-card foreign-force-card"><div class="force-card-title">'+esc(u.name||actorName(u))+'</div><p>'+esc(u.owner||'unknown')+' · '+esc(unitTypeLabel(u.type))+' ×'+fmtNum(u.size)+' · '+esc(unitStatusLabel(u))+'</p>'
  +(u.crew!=null?'<p>Crew '+fmtNum(u.crew)+'</p>':'')+(u.stance?'<p>Stance: '+esc(u.stance)+'</p>':'')
  +(u.heading?'<p>Heading '+esc(u.heading)+'</p>':'')+apparent
  +(u.category==='naval'?'<p>Hull: unknown · Provisions: unknown</p><p>Passengers: '+cargo+'</p>':'')
  +'<p>Orders and destination: unknown</p></div>';
}
