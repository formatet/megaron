import { showInlineResult } from '../inline_result.js';
import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc } from '../format.js';
import { fmtEta } from '../time.js';
import { fmtDays } from '../fmt_num.js';
import { serverNow } from '../../clock.js';
import { renderLockedActions } from '../misc.js';
import { divineValuesHTML } from './kult_values.js';
import { kharisNetView, kharisDecayHTML } from './kult_kharis.js';

// ── Kult drawer ───────────────────────────────────────────────────────────
// See cityLoadSeq: an older reply must not replace what a newer load rendered.
let kultLoadSeq = 0;
export async function loadKultDrawer() {
  const seq = ++kultLoadSeq;
  const el = document.getElementById('kult-body');
  el.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';

  if (!State.MY_SETTLEMENT_ID) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">No settlement.</p>';
    return;
  }
  const capital = ownCapital();
  if (!capital) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">No settlement.</p>';
    return;
  }

  try {
    const [sr, pr] = await Promise.all([
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID),
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + capital.id),
    ]);
    if (!sr.ok) throw new Error();
    const sd = await sr.json();
    const pd = pr.ok ? (await pr.json()).settlement : null;
    const prayers = (pd && pd.available_prayers) || [];
    const MOOD_LABELS = { Favorable:'Favorable', Indifferent:'Indifferent', Suspicious:'Suspicious', Wrathful:'Wrathful' };
    const MOOD_COLORS = { Favorable:'var(--safe)', Indifferent:'var(--text-dim)', Suspicious:'var(--border)', Wrathful:'var(--danger)' };

    const mood      = sd.divine_mood  || 'Indifferent';
    const cultLevel = sd.cult_level   || 'enkel';

    let html = '<div class="dsec"><div class="dsec-title">Divine favour</div>' +
      '<div class="stat-row"><span class="sr-label">Mood</span>' +
      '<span class="sr-val" style="color:' + (MOOD_COLORS[mood] || 'inherit') + '">' + (MOOD_LABELS[mood] || mood) + '</span></div>';
    // Net kharis (temple gain − decay) — the figure that answers "did raising
    // devotion actually help", never computed client-side. See kult_kharis.js.
    const { netHtml, idleHtml } = kharisNetView(pd);
    html += netHtml + kharisDecayHTML(pd) + idleHtml;
    html += '</div>';

    // Cult level — derived from kharis (read-only; set by daily temple tick)
    html += '<div class="dsec"><div class="dsec-title">Cult level</div>' +
      '<div class="stat-row"><span class="sr-label">Level</span>' +
      '<span class="sr-val">' + cultLevelLabel(cultLevel) + '</span></div></div>';

    // The server plans the entire local gift; render its plan without repricing.
    if (pd && Array.isArray(pd.temple_offers)) {
      html += templeOfferingsHTML(pd.temple_offers);
    }

    html += divineValuesHTML(pd?.divine_values);

    // Prayer catalog
    html += '<div class="dsec"><div class="dsec-title">Prayers</div>';
    if (!prayers.length) {
      html += '<p class="empty-state">No prayers available — build a temple first.</p>';
    } else {
      const frenzyUntil = sd.battle_frenzy_until ? new Date(sd.battle_frenzy_until) : null;
      const frenzyActive = frenzyUntil && frenzyUntil.getTime() > serverNow();
      prayers.forEach(p => {
        const offeringStr = Object.entries(p.offering || {}).map(([g,q]) => `${q} ${g}`).join(' + ') || '—';
        const statusHtml = prayerStatusHTML(p, frenzyActive, sd.battle_frenzy_until);

        html += `<div class="stat-row" style="align-items:flex-start;flex-wrap:wrap">
          <span class="sr-label">${esc(p.name)} <span style="color:var(--text-dim);font-size:.68rem">(${esc(p.god)}, ≥${p.min_kharis} kharis)</span><br>
          <span style="color:var(--text-dim);font-size:.7rem">${esc(p.effect || '')}<br>traditional offering: ${offeringStr}</span></span>
          <span class="sr-val">${statusHtml}</span>
        </div>`;
      });
    }
    html += '</div>';

    html += await renderLockedActions('cult');
    if (seq !== kultLoadSeq) return;
    el.innerHTML = html;
  } catch(_) {
    if (seq !== kultLoadSeq) return;
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>';
  }
}

const TEMPLE_GOOD_ORDER = ['grain', 'fish', 'timber', 'silver'];
function templeGoodsText(goods, separator) {
  const entries = Object.entries(goods || {}).sort(([a], [b]) => {
    const ai = TEMPLE_GOOD_ORDER.indexOf(a), bi = TEMPLE_GOOD_ORDER.indexOf(b);
    return (ai < 0 ? 4 : ai) - (bi < 0 ? 4 : bi) || a.localeCompare(b);
  });
  return entries.map(([good, amount]) => esc(String(Number(amount.toPrecision(4)))) + ' ' + esc(good)).join(separator) || '—';
}

export function templeOfferingsHTML(offers) {
  let html = '<div class="dsec"><div class="dsec-title">Temple offerings</div>';
  if (!offers.length) {
    return html + '<p class="empty-state">No temples — kharis will not climb without a temple and offerings.</p></div>';
  }
  for (const t of offers) {
    const mark = t.fed ? '<span style="color:var(--safe)">✓</span>' : '<span style="color:var(--accent)">✗</span>';
    const next = t.fed ? templeGoodsText(t.offering, ' + ') : 'unfunded (nothing taken)';
    html += '<div class="stat-row" style="align-items:flex-start;flex-wrap:wrap"><span class="sr-label">' + esc(t.name || '') + ': </span> ' +
      '<span class="sr-val">reference ' + templeGoodsText(t.required_goods, ' + ') + ' per day<br>' +
      'Alternatives: ' + templeGoodsText(t.alternatives, ' or ') + '<br>' +
      'Stock: ' + templeGoodsText(t.stock, ', ') + '<br>' +
      'Next gift: ' + next + ' ' + mark + '</span></div>';
  }
  return html + '</div>';
}

// sd.cult_level is NOT a five-tier cult-level progression — the server never
// runs one. internal/kharis/tick.go's deriveMood "replaces player-set
// cult_level" and writes ONE of the same four kharis-mood tiers as
// sd.divine_mood into this column instead, just spelled in Swedish:
// tveksam/vardig/overdadig/vredgad (religion.MoodSuspicious/Indifferent/
// Favorable thresholds). 'enkel' is settlement.go's fallback for a player
// whose first daily tick hasn't run yet (no mood derived at all) — also not
// a tier. 'forsummad'/'praktfull' were the old five-tier model's labels and
// the server can never send them (2026-08-26 audit, megaron_arbetssatt.md
// semantic gate — this table was leaking 'tveksam'/'vredgad' straight to the
// screen as raw Swedish).
const CULT_LABELS = { enkel:'Not yet assessed', tveksam:'Suspicious', vardig:'Indifferent', overdadig:'Favorable', vredgad:'Wrathful' };

// cultLevelLabel translates sd.cult_level for display. Pure — no DOM/State —
// so it can be unit-tested directly (economy.js/cargo.js pattern).
export function cultLevelLabel(cultLevel) {
  return CULT_LABELS[cultLevel] || cultLevel;
}

export async function okRite(prayerID) {
  return castRite(prayerID);
}

// Offer uses the prayer's traditional recipe on the server.
async function castRite(prayerID) {
  const body = { prayer: prayerID || '' };
  const r = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/rite', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const d = await r.json().catch(function(){return {};});
  if (r.ok) {
    await loadKultDrawer();
    showInlineResult('kult-body', d.message || (d.success ? 'The gods answered!' : 'The gods are silent.'));
  } else {
    showInlineResult('kult-body', d.error || 'Rite failed', true);
  }
}

// Server-owned waiting/active state gates the same Offer button the drawer uses.
export function prayerStatusHTML(p, frenzyActive = false, frenzyUntil = null) {
  const waitDays = p.cooldown_remaining_game_days || 0;
  if (p.departure_pending) {
    const waiting = waitDays > 0 ? `; god waits ${esc(fmtDays(waitDays))}` : '';
    return `<span class="empty-state">awaiting next departure${waiting}</span>`;
  }
  if (p.active_remaining_game_days > 0) {
    return `<span class="empty-state">active — ${esc(fmtDays(p.active_remaining_game_days))} left</span>`;
  }
  if (p.effect_type === 'battle_frenzy' && frenzyActive) {
    return `<span style="color:var(--safe);font-size:.72rem">active — expires ${fmtEta(frenzyUntil, undefined, 'expired')}</span>`;
  }
  if (waitDays > 0) {
    return `<span style="color:var(--text-dim);font-size:.72rem">on cooldown — ${esc(fmtDays(waitDays))}</span>`;
  }
  if (!p.affordable) {
    return '<span style="color:var(--text-dim);font-size:.72rem">not affordable</span>';
  }
  return `<button class="obj-cta btn-small" onclick="okRite('${p.id}')">Offer</button>`;
}
