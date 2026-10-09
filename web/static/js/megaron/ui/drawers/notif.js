import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { updateNotifBadge } from '../chips.js';
import { fmtAgo, notifText, notifIcon, colonyFoundedGrainLine } from '../format.js';
import { currentCalendarDate, monthLabel } from '../misc.js';
import { fmtNum } from '../fmt_num.js';
import { openDispatchWindow } from '../dispatch_window.js';
import { renderRumourRowHTML } from './gossip.js';

// ── Notifications drawer ──────────────────────────────────────────────────
// Mirrors keryx `notifications` (DEL B/D): the default view excludes the noisy
// Sitos kinds (~99% of the feed) so real events stay visible, with a "+N —
// click to view" drill-down per hidden kind. SubsistenceWarning is never
// collapsed; its critical tier floats to the very top of the feed.
const NOISY_NOTIF_KINDS = ['SitosIntervention', 'SitosFundLow'];

// Exact date, top of the drawer — the celestial widget only shows the month
// name now (Timothy 2026-08-06); this is where the precise day/month/year
// reading lives, off the same tick-anchor math (currentCalendarDate). The
// month carries its 1..12 ordinal (monthLabel) so a Wanax can count days
// between two notifications without a name-only calendar lookup.
export function notifDateHeader(cal = currentCalendarDate(), state = State, showMonthNumber = true) {
  if (!cal) return '';
  // A world whose clock has not started yet. Without this the world simply
  // looks frozen, which reads as a broken game rather than as a lobby.
  let waiting = '';
  if (state.WORLD_STATE === 'forming') {
    const need = Math.max(0, (state.WANAXES_NEEDED || 0) - (state.WANAXES_JOINED || 0));
    waiting = `<div class="notif-world-waiting">⏳ The world has not begun — waiting for ${fmtNum(need)} more ${need === 1 ? 'Wanax' : 'Wanaxes'}.` +
      ` Time stands still until then; you may look around, but orders can be given only once it starts.</div>`;
  }
  const month = showMonthNumber ? monthLabel(cal) : monthLabel(cal).replace(/\s*\(\d+\)/, '');
  return `<div class="notif-date-header">Day ${fmtNum(cal.day)} of ${month.replace(/\((\d+)\)/, (_, value) => `(${fmtNum(Number(value))})`)}, Year ${fmtNum(cal.year)}${waiting}</div>`;
}


export function hiddenNotifLabel(kind, count) {
  const label = kind === 'SitosIntervention' ? 'food support reports' : 'food reserve warnings';
  return `${fmtNum(count)} ${label} — show reports`;
}

// One list, newest first: what happened to you and what you have only heard.
// A rumour is NOT an event — it is blurred and may be old — so it keeps its own
// marked row ("Rumour"), never the plain event look. Critical subsistence
// warnings still float above everything (the caller sorts them first).
export function mergeFeed(notifs, rumours) {
  const stamp = x => { const t = new Date(x).getTime(); return Number.isFinite(t) ? t : 0; };
  const rows = [
    ...notifs.map((n, i) => ({ type: 'notif', idx: i, n, t: stamp(n.created_at) })),
    ...(rumours || []).map(g => ({ type: 'rumour', g, t: stamp(g.generated_at) })),
  ];
  // Stable: events keep the server's order among equal timestamps.
  return rows.map((r, order) => ({ ...r, order })).sort((a, b) => (b.t - a.t) || (a.order - b.order));
}

export function notifShowKind(kind) { loadNotifDrawer(kind || null); }

// "Clear all" was REMOVED here 2026-08-22 (Timothy): "notislistan ska vara ett
// arkiv, allt ska finnas där." This drawer is the permanent record a Wanax
// returning after nine hours reads to learn what happened — a control whose only
// effect is to destroy that record has no place on it. The DELETE endpoint is
// left standing server-side (nothing calls it from the client any more).
// Dismissing belongs to the TRANSIENT list instead: the chips that slide in from
// the right (ui/chips.js dismissAllChips) — those are a stream, not an archive.

export async function loadNotifDrawer(kindFilter) {
  const body = document.getElementById('notif-body');
  body.innerHTML = '<div class="loading" style="padding:.5rem">Loading…</div>';
  try {
    const base = `/api/v1/worlds/${State.WORLD_ID}/notifications`;
    const url = kindFilter
      ? `${base}?kind=${encodeURIComponent(kindFilter)}`
      : `${base}?exclude=${encodeURIComponent(NOISY_NOTIF_KINDS.join(','))}`;
    const r = await fetchAuth(url);
    if (!r.ok) {
      body.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load notifications.</p>';
      return;
    }
    const data = await r.json();
    // Rumours join the default view only; a kind drill-down stays events-only.
    // A failed rumour fetch must never hide the events.
    let rumours = [];
    if (!kindFilter) {
      try {
        const gr = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/gossip`);
        if (gr && gr.ok) rumours = (await gr.json().catch(() => [])) || [];
      } catch (_) {}
    }
    if (!kindFilter) {
      updateNotifBadge(0);
      fetchAuth(`${base}/read-all`, { method: 'POST' });
    }

    // Critical SubsistenceWarnings first — a starving city must never scroll
    // past. Stable within each partition (server orders created_at DESC).
    const notifs = (data.notifications || []).slice();
    const tierOf = n => {
      if (n.kind !== 'SubsistenceWarning') return '';
      const b = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
      return b.tier || '';
    };
    notifs.sort((a, b) => (tierOf(b) === 'critical' ? 1 : 0) - (tierOf(a) === 'critical' ? 1 : 0));

    let html = '';
    if (kindFilter) {
      html += `<div class="notif-list-item" style="cursor:pointer;color:var(--text-dim)" onclick="notifShowKind()">
        <span class="nli-kind">←</span><span class="nli-text">All notifications</span><span class="nli-time"></span></div>`;
    }
    html += mergeFeed(notifs, rumours).map(row => {
      if (row.type === 'rumour') return renderRumourRowHTML(row.g);
      const n = row.n, i = row.idx;
      const ago = fmtAgo(n.created_at);
      const body_obj = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
      const text = notifText(n.kind, body_obj);
      const lvlClass = n.level <= 2 ? 'nl-urgent' : n.level === 3 ? 'nl-info' : 'nl-routine';
      const unread  = !n.read_at ? ' nl-unread' : '';
      const tier = tierOf(n);
      const tierClass = tier ? ` notif-tier-${tier}` : '';
      const grainLine = n.kind === 'ColonyFounded' ? colonyFoundedGrainLine(body_obj) : '';
      // data-idx: the second door (megaron_plan_dispatches.md §1) — every
      // archive row opens the SAME window a dispatch chip does. Wired below
      // via addEventListener (not inline onclick) since the payload doesn't
      // survive HTML-attribute escaping cleanly.
      return `<div class="notif-list-item ${lvlClass}${unread}${tierClass}" data-idx="${i}">
        <span class="nli-kind">${notifIcon(n.kind)}</span>
        <span class="nli-text">${text}</span>
        <span class="nli-time">${ago}</span>
        ${grainLine ? `<span class="nli-grain">${grainLine}</span>` : ''}
      </div>`;
    }).join('');

    // Hidden-kind drill-down (default view only): best-effort counts of what
    // the exclude filter hid, mirroring keryx's "+N — --kind X för alla" line.
    if (!kindFilter) {
      for (const kind of NOISY_NOTIF_KINDS) {
        try {
          const cr = await fetchAuth(`${base}?kind=${encodeURIComponent(kind)}`);
          if (!cr.ok) continue;
          const cd = await cr.json();
          const count = (cd.notifications || []).length;
          if (count > 0) {
            html += `<div class="notif-list-item" style="cursor:pointer;color:var(--text-dim)" onclick="notifShowKind('${kind}')">
              <span class="nli-kind">◉</span><span class="nli-text">${hiddenNotifLabel(kind, count)}</span><span class="nli-time"></span></div>`;
          }
        } catch (_) {}
      }
    }

    body.innerHTML = notifDateHeader() + (html || '<p class="empty-state" style="padding:1rem">No notifications yet.</p>');

    // Second door (megaron_plan_dispatches.md §1): every real notification
    // row opens the same window a dispatch chip opens. The two meta-rows
    // above ("← All notifications", "+N … click to view") have their own
    // inline onclick and carry no data-idx, so this selector never touches them.
    body.querySelectorAll('.notif-list-item[data-idx]').forEach(el => {
      el.addEventListener('click', () => {
        const n = notifs[Number(el.dataset.idx)];
        if (!n) return;
        const body_obj = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
        openDispatchWindow(n.kind, body_obj, fmtAgo(n.created_at));
      });
    });
  } catch (_) {
    body.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load notifications.</p>';
  }
}
