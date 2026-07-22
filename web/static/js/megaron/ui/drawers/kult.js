import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { track } from '../../telemetry.js';
import { esc } from '../format.js';
import { fmtEta } from '../time.js';
import { serverNow } from '../../clock.js';
import { renderLockedActions } from '../misc.js';

// ── Kult drawer ───────────────────────────────────────────────────────────
export async function loadKultDrawer() {
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
    const [sr, pr, gr] = await Promise.all([
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID),
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + capital.id),
      // Stock for the offering composer — the province GET's settlement object
      // carries the prayers but not the goods, so the composer needs its own read.
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + capital.id + '/goods'),
    ]);
    if (!sr.ok) throw new Error();
    const sd = await sr.json();
    const pd = pr.ok ? (await pr.json()).settlement : null;
    const prayers = (pd && pd.available_prayers) || [];
    // Stock, so the composer can only offer what the city actually holds and can
    // price the gift as the gods will (local price × the god's affinity is the
    // closest a client can get to the server's reckoning).
    const goods = gr && gr.ok ? await gr.json() : [];

    const CULT_LEVELS = ['forsummad','enkel','vardig','praktfull','overdadig'];
    const CULT_LABELS = { forsummad:'Neglected', enkel:'Simple', vardig:'Worthy', praktfull:'Magnificent', overdadig:'Lavish' };
    const MOOD_LABELS = { Favorable:'Favorable', Indifferent:'Indifferent', Suspicious:'Suspicious', Wrathful:'Wrathful' };
    const MOOD_COLORS = { Favorable:'var(--safe)', Indifferent:'var(--text-dim)', Suspicious:'var(--border)', Wrathful:'var(--danger)' };

    const mood      = sd.divine_mood  || 'Indifferent';
    const cultLevel = sd.cult_level   || 'enkel';

    let html = '<div class="dsec"><div class="dsec-title">Divine favour</div>' +
      '<div class="stat-row"><span class="sr-label">Mood</span>' +
      '<span class="sr-val" style="color:' + (MOOD_COLORS[mood] || 'inherit') + '">' + (MOOD_LABELS[mood] || mood) + '</span></div>';
    // Kharis is DAILY-maintenance-driven — show the passive geographic rate
    // per game-day (keryx `status` parity), never a per-tick figure.
    if (pd && pd.kharis_per_day != null) {
      html += '<div class="stat-row"><span class="sr-label">Passive</span><span class="sr-val">' +
        (pd.kharis_per_day >= 0 ? '+' : '') + pd.kharis_per_day.toFixed(1) + ' kharis/day</span></div>';
    }
    html += '</div>';

    // Cult level — derived from kharis (read-only; set by daily temple tick)
    html += '<div class="dsec"><div class="dsec-title">Cult level</div>' +
      '<div class="stat-row"><span class="sr-label">Level</span>' +
      '<span class="sr-val">' + (CULT_LABELS[cultLevel] || cultLevel) + '</span></div></div>';

    // Temple offerings — read-only mirror of the daily offer gate (keryx
    // `status` parity): answers "will my kharis climb today" per temple city.
    // Fed status is ✓/✗ against the oil/wine requirement — never a percent.
    if (pd && Array.isArray(pd.temple_offers)) {
      html += '<div class="dsec"><div class="dsec-title">Temple offerings</div>';
      if (!pd.temple_offers.length) {
        html += '<p class="empty-state">No temples — kharis will not climb without a temple and offerings.</p>';
      } else {
        pd.temple_offers.forEach(t => {
          const mark = t.fed ? '<span style="color:var(--safe)">✓</span>' : '<span style="color:var(--accent)">✗</span>';
          html += '<div class="stat-row" style="align-items:flex-start"><span class="sr-label">' + esc(t.name || '') + '</span>' +
            '<span class="sr-val">needs ' + (t.oil_needed || 0).toFixed(0) + ' oil + ' + (t.wine_needed || 0).toFixed(0) +
            ' wine/day — has oil ' + Math.floor(t.oil || 0) + ', wine ' + Math.floor(t.wine || 0) + ' ' + mark + '</span></div>';
        });
      }
      html += '</div>';
    }

    // Prayer catalog
    html += '<div class="dsec"><div class="dsec-title">Prayers</div>';
    if (!prayers.length) {
      html += '<p class="empty-state">No prayers available — build a temple first.</p>';
    } else {
      const frenzyUntil = sd.battle_frenzy_until ? new Date(sd.battle_frenzy_until) : null;
      const frenzyActive = frenzyUntil && frenzyUntil.getTime() > serverNow();
      prayers.forEach(p => {
        const offeringStr = Object.entries(p.offering || {}).map(([g,q]) => `${q} ${g}`).join(' + ') || '—';
        const onCooldown = p.cooldown_remaining_minutes > 0;
        let statusHtml;
        if (p.effect_type === 'battle_frenzy' && frenzyActive) {
          statusHtml = `<span style="color:var(--safe);font-size:.72rem">active — expires ${fmtEta(sd.battle_frenzy_until)}</span>`;
        } else if (onCooldown) {
          statusHtml = `<span style="color:var(--text-dim);font-size:.72rem">on cooldown — ${Math.ceil(p.cooldown_remaining_minutes)}m</span>`;
        } else if (!p.affordable) {
          statusHtml = `<span style="color:var(--text-dim);font-size:.72rem">not affordable</span>`;
        } else {
          statusHtml = `<button class="obj-cta btn-small" onclick="okRite('${p.id}')">Perform →</button>`;
        }

        // The priests' reading: an offering is composed now, and its worth turns
        // on world scarcity a Wanax cannot see through the fog. The temple knows
        // what this god favours and what it expects — say both here, where the
        // choice is made, or composing is guesswork.
        const favoured = Object.entries(p.favours || {})
          .filter(([, w]) => w > 1)
          .sort((a, b) => b[1] - a[1])
          .slice(0, 4)
          .map(([g, w]) => `${g} ×${w.toFixed(1)}`)
          .join(', ');
        let tasteHtml = '';
        if (favoured) {
          tasteHtml = `<br><span style="color:var(--text-dim);font-size:.68rem">${esc(p.god)} favours ${favoured}`
            + (p.offering_baseline > 0 ? ` · expects worth ~${Math.round(p.offering_baseline)}` : '')
            + `</span>`;
        }

        html += `<div class="stat-row" style="align-items:flex-start;flex-wrap:wrap">
          <span class="sr-label">${esc(p.name)} <span style="color:var(--text-dim);font-size:.68rem">(${esc(p.god)}, ≥${p.min_kharis} kharis)</span><br>
          <span style="color:var(--text-dim);font-size:.7rem">traditional offering: ${offeringStr}</span>${tasteHtml}</span>
          <span class="sr-val">${statusHtml}</span>
        </div>`;
        // Composer: bring something other than the traditional recipe. Collapsed
        // by default — the traditional offering stays the one-click path, and a
        // Wanax who does not care about composition never has to see this.
        if (!onCooldown && p.affordable) {
          html += `<details class="offer-composer" data-prayer="${esc(p.id)}">
            <summary>Compose your own offering</summary>
            <div class="offer-goods">${offerGoodRows(p, goods)}</div>
            <div class="offer-worth" id="ow-${esc(p.id)}">—</div>
            <button class="obj-cta btn-small" onclick="okRiteComposed('${p.id}')">Offer this →</button>
          </details>`;
        }
      });
    }
    html += '</div>';

    html += await renderLockedActions('cult');
    el.innerHTML = html;
  } catch(_) {
    el.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load.</p>';
  }
}

export async function okRite(prayerID) {
  return castRite(prayerID, null);
}

// castRite is the one path both buttons take — traditional recipe (offering
// null) and composed offering alike, so the two can never drift apart.
async function castRite(prayerID, offering) {
  const body = { prayer: prayerID || '' };
  if (offering) body.offering = offering;
  const r = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/rite', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body),
  });
  const d = await r.json().catch(function(){return {};});
  if (r.ok) {
    track('rite_performed', { rite: prayerID || '' });
    alert(d.message || (d.success ? 'The gods answered!' : 'The gods are silent.'));
    loadKultDrawer();
  } else {
    alert(d.error || 'Rite failed');
  }
}


// offerGoodRows renders one number input per good the city holds enough of to be
// worth offering. Sorted by how much this god favours it: the answer to "what
// should I bring?" should be at the top, not buried alphabetically.
function offerGoodRows(prayer, goods) {
  const favours = prayer.favours || {};
  const usable = goods
    .filter(g => g.key !== 'silver' && (g.amount || 0) >= 1)
    .map(g => ({ key: g.key, amount: g.amount || 0, price: g.price || 0, affinity: favours[g.key] || 1 }))
    .sort((a, b) => (b.affinity - a.affinity) || (b.amount - a.amount));
  if (!usable.length) return '<div class="offer-empty">Nothing in store to offer.</div>';
  return usable.map(g => `
    <label class="offer-good">
      <span>${g.key}${g.affinity > 1 ? ` <b>×${g.affinity.toFixed(1)}</b>` : ''}</span>
      <input type="number" min="0" max="${Math.floor(g.amount)}" step="1" value="0"
             data-good="${g.key}" data-unit="${(g.price * g.affinity).toFixed(3)}"
             oninput="okOfferWorth('${prayer.id}', ${prayer.offering_baseline || 0})">
      <span class="offer-have">of ${Math.floor(g.amount)}</span>
    </label>`).join('');
}

// okOfferWorth keeps the running total honest as the Wanax types. The figure is
// an ESTIMATE — the server values the gift against world scarcity, which the
// client cannot see — so it is labelled as one rather than pretending to be the
// verdict.
export function okOfferWorth(prayerID, baseline) {
  const box = document.querySelector(`.offer-composer[data-prayer="${prayerID}"]`);
  const out = document.getElementById('ow-' + prayerID);
  if (!box || !out) return;
  let worth = 0;
  box.querySelectorAll('input[data-good]').forEach(i => {
    worth += (parseFloat(i.value) || 0) * (parseFloat(i.dataset.unit) || 0);
  });
  if (!baseline) { out.textContent = `estimated worth ${Math.round(worth)}`; return; }
  const pct = Math.round((worth / baseline) * 100);
  out.textContent = `estimated worth ${Math.round(worth)} of ~${Math.round(baseline)} expected (${pct}%)`;
  out.style.color = pct >= 100 ? 'var(--safe)' : (pct >= 50 ? 'var(--text-dim)' : 'var(--accent)');
}

// okRiteComposed casts with whatever the Wanax assembled. An empty composition
// falls through to the traditional recipe rather than sending an empty altar.
export async function okRiteComposed(prayerID) {
  const box = document.querySelector(`.offer-composer[data-prayer="${prayerID}"]`);
  if (!box) return;
  const offering = {};
  box.querySelectorAll('input[data-good]').forEach(i => {
    const v = parseFloat(i.value) || 0;
    if (v > 0) offering[i.dataset.good] = v;
  });
  if (!Object.keys(offering).length) { okRite(prayerID); return; }
  await castRite(prayerID, offering);
}
