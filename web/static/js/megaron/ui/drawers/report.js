import { State, ownCapital } from '../../state.js';
import { fetchAuth } from '../../api.js';

// ── Report drawer (B1, megaron_mvp_mandag.md §B1) ─────────────────────────
// Deliberately primitive: kind + free text, sent to POST /reports. Position
// and "what were you looking at" are attached automatically — State.previousDrawer
// (set by main.js's closeDrawer, the one choke point every drawer transition
// passes through) names whichever drawer was open the moment Report was
// opened, and State.selectedHex/ownCapital() supply a hex when nothing was
// explicitly clicked.
export async function submitReport() {
  const textEl = document.getElementById('report-text');
  const kindEl = document.getElementById('report-kind');
  const statusEl = document.getElementById('report-status');
  const text = (textEl?.value || '').trim();
  if (!text) {
    if (statusEl) statusEl.textContent = 'Write something first.';
    return;
  }

  const body = { kind: kindEl?.value || 'bug', body: text };
  const hex = State.selectedHex || ownCapital();
  if (hex && hex.q !== undefined && hex.r !== undefined) {
    body.q = hex.q;
    body.r = hex.r;
  }
  if (State.previousDrawer) body.view = State.previousDrawer;

  if (statusEl) statusEl.textContent = 'Sending…';
  try {
    const res = await fetchAuth(`/api/v1/worlds/${State.WORLD_ID}/reports`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      if (statusEl) statusEl.textContent = 'Could not send — try again.';
      return;
    }
    if (textEl) textEl.value = '';
    if (statusEl) statusEl.textContent = 'Sent. Thank you.';
  } catch (_) {
    if (statusEl) statusEl.textContent = 'Could not send — try again.';
  }
}
