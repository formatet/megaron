import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc } from '../format.js';
import { fmtEta, fmtArrival, arrivalHTML } from '../time.js';
import { renderLockedActions } from '../misc.js';
import { loadMap } from '../../render/map.js';
import { loadCityDrawer } from './city.js';

// ── War drawer ────────────────────────────────────────────────────────────
export async function loadWarDrawer() {
  const body = document.getElementById('war-body');
  const capital = ownCapital();
  if (!capital) { body.innerHTML = '<p class="empty-state" style="padding:1rem">No settlement.</p>'; return; }
  // Preserve recruit-city selection across reloads
  const prevRecruitCity = document.getElementById('war-recruit-city')?.value || capital.id;

  body.innerHTML = `
    <div class="drawer-tabs">
      <button class="dtab active" data-tab="army">Army</button>
      <button class="dtab" data-tab="recruit">Recruit</button>
      <button class="dtab" data-tab="movements">Movements</button>
    </div>
    <div id="wtab-army" class="city-tab"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="wtab-recruit" class="city-tab" style="display:none"><div class="loading" style="font-size:.8rem">Loading…</div></div>
    <div id="wtab-movements" class="city-tab" style="display:none"></div>`;

  body.querySelectorAll('.dtab').forEach(tab => {
    tab.addEventListener('click', function() {
      body.querySelectorAll('.dtab').forEach(t => t.classList.remove('active'));
      this.classList.add('active');
      body.querySelectorAll('.city-tab').forEach(c => c.style.display = 'none');
      const el = document.getElementById('wtab-' + this.dataset.tab);
      if (el) el.style.display = '';
    });
  });

  renderWarMovements(capital);

  const UNIT_LBL  = { Spearman:'Spearmen', EliteInfantry:'Elite Infantry', WarChariot:'War Chariot', Ship:'Galley', WarGalley:'War Galley', Merchantman:'Emporos',
                      spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot', ship:'Galley', war_galley:'War Galley', merchantman:'Emporos' };
  const UNIT_DP   = { Spearman:1, EliteInfantry:3, WarChariot:4, Ship:1, WarGalley:3, Merchantman:0 };
  const POP_COSTS = { Spearman:5, EliteInfantry:10, WarChariot:8, Ship:10, WarGalley:12, Merchantman:8 };

  try {
    const needTwo = prevRecruitCity !== capital.id;
    const [res, recRes, unitsRes] = await Promise.all([
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${capital.id}`),
      needTwo ? fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/provinces/${prevRecruitCity}`) : Promise.resolve(null),
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`),
    ]);
    if (!res.ok) throw new Error();
    const pd = (await res.json()).settlement;
    const recPd = needTwo && recRes && recRes.ok ? (await recRes.json()).settlement : pd;
    const army = pd.army || {};
    const buildings = new Set((recPd.buildings || []).map(b => b.type));
    const canRec = {};
    (recPd.can_recruit || []).forEach(r => { canRec[r.unit] = r.can_recruit; });
    const allUnits = unitsRes && unitsRes.ok ? ((await unitsRes.json()).units || []) : [];

    // Army tab — discrete units list
    let armyHtml = '<div class="dsec"><div class="dsec-title">Units</div>';
    if (allUnits.length) {
      armyHtml += allUnits.map(u => renderUnitCard(u)).join('');
    } else {
      armyHtml += '<p class="empty-state">No units. Recruit in the Recruit tab.</p>';
    }
    armyHtml += '</div>';
    armyHtml += '<div id="war-unit-res" style="font-size:.75rem;margin:.3rem .6rem;min-height:1rem"></div>';
    armyHtml += `<div id="war-march-panel" style="display:none;background:var(--bg-raised);border:1px solid var(--border);border-radius:2px;margin:.4rem .6rem;padding:.5rem;font-size:.78rem">
      <div style="font-weight:bold;margin-bottom:.3rem">March unit</div>
      <div style="display:flex;gap:.3rem;align-items:center;flex-wrap:wrap">
        <label style="color:var(--text-dim)">Q</label>
        <input id="wmp-q" type="number" value="0" style="width:44px;padding:.1rem .25rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.75rem">
        <label style="color:var(--text-dim)">R</label>
        <input id="wmp-r" type="number" value="0" style="width:44px;padding:.1rem .25rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.75rem">
        <label style="color:var(--text-dim)">Stance</label>
        <select id="wmp-stance" style="font-size:.72rem;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white)">
          <option value="">none</option>
          <option value="storm">storm</option>
          <option value="sentry">sentry</option>
        </select>
        <button onclick="unitMarchSend()" style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.7rem;cursor:pointer">March →</button>
        <button onclick="closeMarchPanel()" style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.7rem;cursor:pointer">Cancel</button>
      </div>
      <div id="wmp-err" style="color:var(--accent);font-size:.7rem;margin-top:.2rem;min-height:.8rem"></div>
    </div>`;
    document.getElementById('wtab-army').innerHTML = armyHtml;
    applyUnitFocus();

    // Recruit tab
    const UNIT_SPECS = [
      { id:'spearman',       lbl:'Spearmen',    req: buildings.has('barracks') ? null : 'barracks',   cost:'3 grain/man'          },
      { id:'war_chariot',    lbl:'War Chariot',  req: buildings.has('stable')   ? null : 'stable',     cost:'3.75 grain + 0.625 timber + 0.375 bronze/man' },
      { id:'ship',           lbl:'Galley',       req: buildings.has('harbour')  ? null : 'harbour',    cost:'9 timber/man (crew 20)' },
      { id:'war_galley',     lbl:'War Galley',   req: !buildings.has('harbour') ? 'harbour' : (!buildings.has('foundry') ? 'foundry' : null), cost:'5 cedar + 0.33 bronze/man (crew 50)' },
      { id:'merchantman',    lbl:'Emporos',      req: buildings.has('harbour')  ? null : 'harbour',    cost:'8.75 timber/man (crew 10)' },
      { id:'elite_infantry', lbl:'Elite Infantry',        req: buildings.has('foundry')  ? null : 'foundry',    cost:'2.5 grain + 0.2 bronze/man' },
    ];
    const mySettlements = State.provinceData.filter(p => p.own && !p.is_outpost);
    let settlementOpts = mySettlements.map(s =>
      '<option value="' + s.id + '"' + (s.id === prevRecruitCity ? ' selected' : '') + '>' + esc(s.name) + (s.is_capital ? ' ★' : '') + '</option>'
    ).join('');
    let recHtml = '<div class="dsec">';
    if (mySettlements.length > 1) {
      recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.25rem 0;font-size:.78rem;color:var(--text-dim)">'
        + '<span>Recruit at:</span>'
        + '<select id="war-recruit-city" style="flex:1;font-size:.75rem;padding:.15rem .3rem;background:var(--warm-white);border:1px solid var(--border);" onchange="loadWarDrawer()">'
        + settlementOpts + '</select></div>';
    } else {
      recHtml += '<input type="hidden" id="war-recruit-city" value="' + capital.id + '">';
    }
    recHtml += '<div class="dsec-title">Train Units</div>';
    recHtml += '<div style="font-size:.65rem;color:var(--text-dim);margin-bottom:.3rem">Land units train in batches of 10 men (up to 100). Ships are built one at a time.</div>';
    const NAVAL_SPEC_IDS = ['ship', 'war_galley', 'merchantman'];
    for (const u of UNIT_SPECS) {
      const noBuilding = u.req !== null;
      const noResources = !noBuilding && canRec[u.id] === false;
      const disabled = noBuilding || noResources;
      const costText = noBuilding ? ('requires ' + u.req) : (noResources ? u.cost + ' — insufficient' : u.cost);
      const opStyle = disabled ? 'opacity:.5;' : '';
      if (NAVAL_SPEC_IDS.includes(u.id)) {
        // Ship-build overhaul: one vessel per build, optional name, no men select.
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + u.lbl + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<input id="wrc-name-' + u.id + '" type="text" placeholder="name (optional)" ' + (disabled ? 'disabled' : '') + ' style="width:100px;padding:.12rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.7rem">'
          + '<button onclick="warRecruitShip(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Build 1 Ship</button>'
          + '</div>';
      } else {
        recHtml += '<div style="display:flex;align-items:center;gap:.4rem;padding:.28rem 0;border-bottom:1px solid var(--border);' + opStyle + '">'
          + '<span style="flex:1;font-size:.8rem">' + u.lbl + '</span>'
          + '<span style="font-size:.65rem;color:var(--text-dim);text-align:right">' + costText + '</span>'
          + '<select id="wrc-' + u.id + '" ' + (disabled ? 'disabled' : '') + ' style="width:54px;padding:.12rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.75rem">'
          + [10,20,30,40,50,60,70,80,90,100].map(n => '<option value="' + n + '"' + (n===10?' selected':'') + '>' + n + '</option>').join('')
          + '</select>'
          + '<button onclick="warRecruitFromUI(\'' + u.id + '\')" ' + (disabled ? 'disabled' : '') + ' style="padding:.2rem .45rem;border:1px solid var(--border);background:var(--sandstone);font-size:.7rem;cursor:pointer;white-space:nowrap">Train</button>'
          + '</div>';
      }
    }
    recHtml += '</div><div id="war-recruit-res" style="font-size:.78rem;margin:.4rem .6rem;min-height:1rem"></div>';

    const abandonable = mySettlements.filter(s => !s.is_capital);
    if (abandonable.length) {
      recHtml += '<div class="dsec"><div class="dsec-title" style="color:var(--accent)">Abandon settlement</div>'
        + abandonable.map(s =>
          '<div class="stat-row"><span class="sr-label">' + esc(s.name) + '</span>'
          + '<span class="sr-val"><button class="btn-small btn-danger" onclick="warAbandon(\'' + s.settlement_id + '\',\'' + esc(s.name) + '\')">Abandon</button></span></div>'
        ).join('')
        + '<div id="war-abandon-res" style="font-size:.72rem;margin-top:.2rem;min-height:.9rem"></div></div>';
    }
    document.getElementById('wtab-recruit').innerHTML = recHtml;
    document.getElementById('wtab-recruit').innerHTML += await renderLockedActions('military');

  } catch(e) {
    console.error('war drawer', e);
    document.getElementById('wtab-army').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
    document.getElementById('wtab-recruit').innerHTML = '<p class="empty-state" style="padding:.5rem">Could not load.</p>';
  }
}

function renderWarMovements(capital) {
  const el = document.getElementById('wtab-movements');
  if (!el || !capital) return;
  const ownPos = new Set(State.provinceData.filter(p => p.own).map(p => p.q + ',' + p.r));
  const outgoing = State.marchData.filter(m => ownPos.has(m.origin_q + ',' + m.origin_r));
  const incoming = State.marchData.filter(m => {
    const t = State.provinceData.find(p => p.q === m.target_q && p.r === m.target_r && p.own);
    return t && !ownPos.has(m.origin_q + ',' + m.origin_r);
  });
  let html = '';
  if (outgoing.length) {
    html += '<div class="dsec"><div class="dsec-title">Outgoing</div>';
    html += outgoing.map(m => {
      const target = State.provinceData.find(p => p.q === m.target_q && p.r === m.target_r);
      const tname = target ? esc(target.name) : '(' + m.target_q + ',' + m.target_r + ')';
      return '<div class="obj-card">'
        + '<div class="obj-icon">⚔</div>'
        + '<div class="obj-info"><div class="obj-name">' + m.intent.charAt(0).toUpperCase() + m.intent.slice(1) + ' → ' + tname + '</div><div class="obj-sub">Arrives ' + arrivalHTML(m.arrives_at) + ' · recall/redirect in the Army tab</div></div>'
        + '</div>';
    }).join('');
    html += '</div>';
  } else {
    html += '<div class="dsec"><p class="empty-state">No armies in the field.</p></div>';
  }
  if (incoming.length) {
    html += '<div class="dsec"><div class="dsec-title" style="color:var(--accent)">⚠ Incoming</div>';
    html += incoming.map(m => {
      const origin = State.provinceData.find(p => p.q === m.origin_q && p.r === m.origin_r);
      const oname = origin ? esc(origin.name) : '(' + m.origin_q + ',' + m.origin_r + ')';
      return '<div class="obj-card">'
        + '<div class="obj-icon" style="color:var(--accent)">⚔</div>'
        + '<div class="obj-info"><div class="obj-name">' + m.intent.charAt(0).toUpperCase() + m.intent.slice(1) + ' from ' + oname + '</div><div class="obj-sub">Arrives ' + arrivalHTML(m.arrives_at) + '</div></div>'
        + '</div>';
    }).join('');
    html += '</div>';
  }
  el.innerHTML = html || '<p class="empty-state" style="padding:.5rem">No movements.</p>';
}

export function warRecruitFromUI(unitType) {
  const sel = document.getElementById('war-recruit-city');
  const pid = sel ? sel.value : null;
  if (!pid) return;
  warRecruit(pid, unitType);
}

async function warRecruit(provinceID, unitType) {
  const el = document.getElementById('wrc-' + unitType);
  const men = el ? (parseInt(el.value, 10) || 10) : 10;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + provinceID + '/recruit', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ unit_type: unitType, men }),
  });
  const data = await res.json().catch(() => ({}));
  const resEl = document.getElementById('war-recruit-res');
  if (res.ok) {
    // Reload war drawer to show updated training queue
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = data.error || 'failed';
  }
}

// Ship-build overhaul: builds exactly one vessel (no men field — crew is
// fixed per type); an empty name input means "let the game suggest one".
export async function warRecruitShip(unitType) {
  const sel = document.getElementById('war-recruit-city');
  const pid = sel ? sel.value : null;
  if (!pid) return;
  const nameEl = document.getElementById('wrc-name-' + unitType);
  const name = nameEl ? nameEl.value.trim() : '';
  const body = { unit_type: unitType };
  if (name) body.name = name;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + pid + '/recruit', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  const resEl = document.getElementById('war-recruit-res');
  if (res.ok) {
    if (resEl) {
      const built = (data.names && data.names[0]) ? data.names[0] : unitType;
      resEl.style.color = 'var(--text-dim)';
      resEl.textContent = 'Building "' + built + '" — ready ' + fmtArrival(data.complete_at);
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = data.error || 'failed';
  }
}

export async function warDisband(provinceID) {
  const inf = parseInt(document.getElementById('wdb-inf')?.value || '0');
  const cha = parseInt(document.getElementById('wdb-cha')?.value || '0');
  if (inf + cha === 0) return;
  const res = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + provinceID + '/disband', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ spearman: inf, war_chariot: cha }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    await loadCityDrawer();
  } else {
    const resEl = document.getElementById('war-disband-res');
    if (resEl) resEl.textContent = data.error || 'failed';
  }
}

// ── Discrete unit helpers (C8-web) ───────────────────────────────────────
const UNIT_LABELS = {
  spearman:'Spearmen', elite_infantry:'Elite Infantry', war_chariot:'War Chariot',
  ship:'Galley', galley:'Galley', war_galley:'War Galley', merchantman:'Emporos',
};

// Map → drawer bridge: clicking a hex with an own positioned/marching unit
// selects it — the War drawer opens with that unit's card highlighted and
// scrolled into view, so orders (march, stance, recall) are given from the
// card. Called from render/map.js via the window bridge.
let _focusUnitID = null;
export function warFocusUnit(unitID) {
  _focusUnitID = unitID;
  window.openDrawer('war');
}

function applyUnitFocus() {
  if (!_focusUnitID) return;
  const card = document.getElementById('ucard-' + _focusUnitID);
  _focusUnitID = null;
  if (!card) return;
  card.classList.add('ucard-focus');
  card.scrollIntoView({ block: 'center' });
}

// State for the march panel — module-local, only ever read/written from the
// functions in this file.
let _marchUnitID = null;

function renderUnitCard(u) {
  const lbl = (UNIT_LABELS[u.type] || u.type) + (u.name ? ' "' + esc(u.name) + '"' : '');
  const isNaval = u.category === 'naval';
  const isForming = u.status === 'forming';
  const isTraining = u.status === 'training';
  const isGarrison = u.status === 'garrison';
  const isPositioned = u.status === 'positioned';
  const isMarching = u.status === 'marching';
  const isEmbarked = u.status === 'embarked';

  // Location string
  let loc = '';
  if (isGarrison || isForming || isTraining || isEmbarked) {
    const prov = State.provinceData.find(p => p.settlement_id === u.settlement_id || p.id === u.settlement_id);
    loc = prov ? esc(prov.name) : 'city';
  } else if (isMarching && u.target_q != null) {
    // arrival_tick is the authoritative arrival (K4) — the stored arrives_at
    // stamp lies across server downtime; the tick self-corrects.
    loc = '→ (' + u.target_q + ',' + u.target_r + ') arrives ' + arrivalHTML(u.arrives_at, u.arrival_tick);
  } else if (u.q != null) {
    loc = '(' + u.q + ',' + u.r + ')';
  }

  // Progress: naval forming shows the build ETA (size is always 1); land forming
  // shows the men/100 gathering bar; land training shows a full bar + ready ETA
  // (it has all 100 men and is maturing to a deployable garrison).
  const bar = (pct) => '<div style="margin:.2rem 0;background:var(--border);height:4px;border-radius:2px"><div style="background:var(--accent-war);height:4px;width:' + pct + '%"></div></div>';
  const dim = (txt) => '<span style="font-size:.65rem;color:var(--text-dim)">' + txt + '</span>';
  let progress = '';
  if (isNaval && isForming) {
    progress = dim('building — ready ' + fmtArrival(u.build_complete_at));
  } else if (isForming) {
    progress = bar(u.size) + dim(u.size + '/100 · forming');
  } else if (isTraining) {
    progress = bar(100) + dim('100/100 · training — ready ' + fmtArrival(u.build_complete_at));
  }

  // Pending order (Fas 5): a hemerodromos is running to this unit — the order
  // executes only on delivery; surface the courier ETA on the card.
  const runner = (State.messengerData || []).find(m => m.own && m.kind === 'order' && m.order_unit_id === u.id);
  const pendingOrder = runner
    ? '<div style="font-size:.65rem;color:var(--text-dim)">🏃 Hemerodromos en route — order arrives ' + arrivalHTML(runner.arrives_at) + '</div>'
    : '';

  // Stance badge
  const stanceBadge = u.stance
    ? '<span style="font-size:.6rem;padding:.1rem .25rem;border:1px solid var(--border);color:var(--text-dim);margin-left:.2rem">' + u.stance + '</span>'
    : '';

  // Crew badge for naval
  const crewBadge = isNaval && u.crew
    ? '<span style="font-size:.6rem;color:var(--text-dim);margin-left:.3rem">crew ' + u.crew + '</span>'
    : '';

  // Cargo badge
  const cargoBadge = u.cargo_unit_id
    ? '<span style="font-size:.6rem;color:var(--accent-city);margin-left:.3rem">carrying unit</span>'
    : '';

  // Action buttons
  let actions = '';

  // March button: land size==100 garrison (non-priest), or naval garrison
  // Positioned units (out on the map, e.g. a ship that finished a plain march)
  // must be orderable too — otherwise they're stranded (the map right-click reads
  // the unit's own hex as the target). The server already allows marching a
  // positioned unit; this just surfaces the button.
  const canMarch = (isGarrison || isPositioned) && u.type !== 'priest' && (isNaval || u.size === 100);
  if (canMarch) {
    actions += '<button onclick="unitMarch(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">March</button> ';
  }

  // Stance buttons: garrison or positioned, non-priest, land only — naval
  // units carry no stance (ship-build overhaul 2026-07-09).
  if ((isGarrison || isPositioned) && u.type !== 'priest' && !isNaval) {
    actions += '<select id="ustance-' + u.id + '" style="font-size:.65rem;padding:.1rem;border:1px solid var(--border);background:var(--warm-white)">'
      + '<option value="none">stance…</option>'
      + '<option value="fortify">fortify</option>'
      + '<option value="storm">storm</option>'
      + '<option value="sentry">sentry</option>'
      + (u.stance ? '<option value="none">— clear</option>' : '')
      + '</select> '
      + '<button onclick="unitStance(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Set</button> ';
  }

  // Load button: naval garrison without cargo — pick from co-located garrison land units
  if (isNaval && isGarrison && !u.cargo_unit_id && u.settlement_id) {
    actions += '<button onclick="unitLoadPrompt(\'' + u.id + '\',\'' + (u.settlement_id||'') + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Load</button> ';
  }

  // Unload button: naval garrison with cargo
  if (isNaval && isGarrison && u.cargo_unit_id) {
    actions += '<button onclick="unitUnload(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Unload</button> ';
  }

  // Recall/redirect: marching units only. The order travels by messenger —
  // it does not apply instantly (temenos_settlement.md load-bearing pillar).
  let redirectRow = '';
  if (isMarching) {
    actions += '<button onclick="unitRecall(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Recall</button> ';
    actions += '<button onclick="unitRedirectToggle(\'' + u.id + '\')" style="padding:.15rem .35rem;border:1px solid var(--border);background:var(--bg-raised);font-size:.65rem;cursor:pointer">Redirect</button> ';
    redirectRow = '<div id="uredir-' + u.id + '" style="display:none;margin-top:.2rem;gap:.25rem;align-items:center;font-size:.65rem">'
      + '<label>Q <input id="uredir-q-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
      + '<label>R <input id="uredir-r-' + u.id + '" type="number" value="0" style="width:40px;padding:.1rem .2rem;border:1px solid var(--border);background:var(--warm-white);font-family:var(--mono);font-size:.65rem"></label>'
      + '<button onclick="unitRedirect(\'' + u.id + '\')" style="padding:.1rem .3rem;border:1px solid var(--border);background:var(--accent-war);color:#fff;font-size:.65rem;cursor:pointer">Send order →</button>'
      + '</div>';
  }
  const orderStatus = '<div id="uorder-' + u.id + '" style="font-size:.65rem;color:var(--text-dim);margin-top:.15rem"></div>';

  return '<div id="ucard-' + u.id + '" style="padding:.3rem .2rem;border-bottom:1px solid var(--border)">'
    + '<div style="display:flex;align-items:center;gap:.3rem;flex-wrap:wrap">'
    + '<span style="font-size:.8rem;font-weight:bold">' + lbl + '</span>'
    + stanceBadge + crewBadge + cargoBadge
    + '<span style="font-size:.68rem;color:var(--text-dim);margin-left:auto">' + u.status + '</span>'
    + '</div>'
    + progress
    + (loc ? '<div style="font-size:.65rem;color:var(--text-dim)">' + loc + '</div>' : '')
    + pendingOrder
    + (actions ? '<div style="margin-top:.2rem;display:flex;gap:.2rem;flex-wrap:wrap;align-items:center">' + actions + '</div>' : '')
    + redirectRow + (isMarching ? orderStatus : '')
    + '</div>';
}

export async function warAbandon(settlementID, name) {
  if (!confirm('Abandon ' + name + '? This cannot be undone.')) return;
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/settlements/${settlementID}/abandon`, { method: 'POST' });
  const resEl = document.getElementById('war-abandon-res');
  if (res.ok) {
    await loadMap();
    loadWarDrawer();
  } else {
    const d = await res.json().catch(() => ({}));
    if (resEl) resEl.textContent = d.error || 'Abandon failed';
  }
}

export async function unitRecall(unitID) {
  const statusEl = document.getElementById('uorder-' + unitID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/recall`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: '{}',
  });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    if (statusEl) { statusEl.style.color = 'var(--safe)'; statusEl.textContent = 'Recall order sent by messenger — reaches the unit ' + fmtArrival(d.messenger_arrives_at) + '.'; }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Recall failed';
  }
}

export function unitRedirectToggle(unitID) {
  const row = document.getElementById('uredir-' + unitID);
  if (row) row.style.display = row.style.display === 'none' ? 'flex' : 'none';
}

export async function unitRedirect(unitID) {
  const q = parseInt(document.getElementById('uredir-q-' + unitID)?.value || '0', 10);
  const r = parseInt(document.getElementById('uredir-r-' + unitID)?.value || '0', 10);
  const statusEl = document.getElementById('uorder-' + unitID);
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/recall`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ target_q: q, target_r: r }),
  });
  const d = await res.json().catch(() => ({}));
  if (res.ok) {
    if (statusEl) { statusEl.style.color = 'var(--safe)'; statusEl.textContent = 'Redirect order sent by messenger — reaches the unit ' + fmtArrival(d.messenger_arrives_at) + '.'; }
  } else if (statusEl) {
    statusEl.style.color = 'var(--accent)';
    statusEl.textContent = d.error || 'Redirect failed';
  }
}

export function unitMarch(unitID) {
  _marchUnitID = unitID;
  const panel = document.getElementById('war-march-panel');
  if (panel) {
    // The panel renders once at the bottom of the army tab — below the fold as
    // soon as the unit list fills the drawer, so opening it looked like a dead
    // button. Move it directly under the clicked unit's card instead.
    const card = document.getElementById('ucard-' + unitID);
    if (card) card.after(panel);
    panel.style.display = '';
    document.getElementById('wmp-err').textContent = '';
    panel.scrollIntoView({ block: 'nearest' });
    document.getElementById('wmp-q')?.focus();
  }
}

export function closeMarchPanel() {
  _marchUnitID = null;
  const panel = document.getElementById('war-march-panel');
  if (panel) panel.style.display = 'none';
}

export async function unitMarchSend() {
  if (!_marchUnitID) return;
  const q = parseInt(document.getElementById('wmp-q').value, 10);
  const r = parseInt(document.getElementById('wmp-r').value, 10);
  const stance = document.getElementById('wmp-stance').value || undefined;
  const errEl = document.getElementById('wmp-err');
  errEl.textContent = '';
  const body = { target_q: q, target_r: r };
  if (stance) body.stance = stance;
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${_marchUnitID}/march`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    closeMarchPanel();
    loadWarDrawer();
  } else {
    errEl.textContent = data.error || 'March failed';
  }
}

export async function unitStance(unitID) {
  const sel = document.getElementById('ustance-' + unitID);
  if (!sel) return;
  const stance = sel.value;
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${unitID}/stance`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ stance }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    // Field unit: 202 order_dispatched — the stance travels by hemerodromos
    // and applies on delivery (temenos_orderlopare_plan.md Fas 5). Refresh
    // messengers so the runner + the card's pending-order line show at once.
    if (data.status === 'order_dispatched') {
      if (resEl) {
        resEl.style.color = 'var(--text-dim)';
        resEl.textContent = '🏃 Hemerodromos carries the stance order — applies on delivery';
      }
      fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/messengers`).then(r => r.ok && r.json().then(d => { State.messengerData = d; State.dirty = true; }));
    }
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = data.error || 'Stance change failed';
  }
}

export function unitLoadPrompt(shipID, settlementID) {
  // Find co-located garrison land units from allUnits cache
  const shipEl = document.getElementById('war-unit-res');
  // Re-fetch units to build select
  fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units`).then(async r => {
    if (!r.ok) return;
    const units = (await r.json()).units || [];
    const candidates = units.filter(u =>
      u.category === 'land' && u.status === 'garrison' &&
      u.settlement_id === settlementID && u.size === 100 && u.type !== 'priest'
    );
    if (!candidates.length) {
      if (shipEl) { shipEl.style.color='var(--accent)'; shipEl.textContent='No eligible land unit at this city to load.'; }
      return;
    }
    if (candidates.length === 1) {
      unitLoad(shipID, candidates[0].id);
      return;
    }
    // Multiple: show a quick alert-style select — rare case
    const choice = candidates.find(u => u.id) || candidates[0];
    unitLoad(shipID, choice.id);
  });
}

async function unitLoad(shipID, cargoUnitID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/load`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ unit_id: cargoUnitID }),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = data.error || 'Load failed';
  }
}

export async function unitUnload(shipID) {
  const resEl = document.getElementById('war-unit-res');
  if (resEl) resEl.textContent = '';
  const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/units/${shipID}/unload`, {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({}),
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) {
    loadWarDrawer();
  } else if (resEl) {
    resEl.style.color = 'var(--accent)';
    resEl.textContent = data.error || 'Unload failed';
  }
}
