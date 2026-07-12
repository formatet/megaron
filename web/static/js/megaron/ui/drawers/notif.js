import { State } from '../../state.js';
import { fetchAuth } from '../../api.js';
import { updateNotifBadge } from '../chips.js';
import { fmtAgo, notifText, notifIcon, colonyFoundedGrainLine } from '../format.js';

// ── Notifications drawer ──────────────────────────────────────────────────
// Mirrors keryx `notifications` (DEL B/D): the default view excludes the noisy
// Sitos kinds (~99% of the feed) so real events stay visible, with a "+N —
// click to view" drill-down per hidden kind. SubsistenceWarning is never
// collapsed; its critical tier floats to the very top of the feed.
const NOISY_NOTIF_KINDS = ['SitosIntervention', 'SitosFundLow'];

export function notifShowKind(kind) { loadNotifDrawer(kind || null); }

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
    html += notifs.map(n => {
      const ago = fmtAgo(n.created_at);
      const body_obj = typeof n.body === 'string' ? JSON.parse(n.body) : (n.body || {});
      const text = notifText(n.kind, body_obj);
      const lvlClass = n.level <= 2 ? 'nl-urgent' : n.level === 3 ? 'nl-info' : 'nl-routine';
      const unread  = !n.read_at ? ' nl-unread' : '';
      const tier = tierOf(n);
      const tierClass = tier ? ` notif-tier-${tier}` : '';
      const grainLine = n.kind === 'ColonyFounded' ? colonyFoundedGrainLine(body_obj) : '';
      return `<div class="notif-list-item ${lvlClass}${unread}${tierClass}">
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
              <span class="nli-kind">◉</span><span class="nli-text">+${count} ${kind} — click to view</span><span class="nli-time"></span></div>`;
          }
        } catch (_) {}
      }
    }

    body.innerHTML = html || '<p class="empty-state" style="padding:1rem">No notifications yet.</p>';
  } catch (_) {
    body.innerHTML = '<p class="empty-state" style="padding:1rem">Could not load notifications.</p>';
  }
}
