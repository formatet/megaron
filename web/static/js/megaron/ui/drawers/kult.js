import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { esc, fmtEta } from '../format.js';
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
    const [sr, pr] = await Promise.all([
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID),
      fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/provinces/' + capital.id),
    ]);
    if (!sr.ok) throw new Error();
    const sd = await sr.json();
    const pd = pr.ok ? (await pr.json()).settlement : null;
    const prayers = (pd && pd.available_prayers) || [];

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
      const frenzyActive = frenzyUntil && frenzyUntil > new Date();
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
        html += `<div class="stat-row" style="align-items:flex-start;flex-wrap:wrap">
          <span class="sr-label">${esc(p.name)} <span style="color:var(--text-dim);font-size:.68rem">(${esc(p.god)}, ≥${p.min_kharis} kharis)</span><br>
          <span style="color:var(--text-dim);font-size:.7rem">offering: ${offeringStr}</span></span>
          <span class="sr-val">${statusHtml}</span>
        </div>`;
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
  const r = await fetchAuth('/api/v1/worlds/' + State.WORLD_ID + '/settlements/' + State.MY_SETTLEMENT_ID + '/rite', {
    method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ prayer: prayerID || '' }),
  });
  const d = await r.json().catch(function(){return {};});
  if (r.ok) {
    alert(d.message || (d.success ? 'The gods answered!' : 'The gods are silent.'));
    loadKultDrawer();
  } else {
    alert(d.error || 'Rite failed');
  }
}
