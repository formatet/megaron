import { State } from '../state.js';
import { fetchAuth } from '../api.js';

let inbox = [];
export function updateDiplomacyBadge(messages) {
  inbox = messages;
  const count = inbox.filter(m => !m.read_at).length;
  const badge = document.getElementById('diplo-badge');
  if (badge) {
    badge.textContent = count ? String(count) : '';
    badge.style.display = count ? '' : 'none';
  }
}

// Acknowledge exact displayed IDs; a concurrent new letter stays unread.
export async function readDiplomacyLetters(ids) {
  const world = State.WORLD_ID;
  const results = await Promise.all(ids.map(async id => {
    try {
      const r = await fetchAuth(`/api/v1/worlds/${world}/messengers/${encodeURIComponent(id)}/read`, { method: 'POST' });
      return r.ok ? id : null;
    } catch { return null; }
  }));
  if (State.WORLD_ID !== world) return false;
  const acknowledged = new Set(results.filter(Boolean));
  updateDiplomacyBadge(inbox.map(m => acknowledged.has(m.id) ? { ...m, read_at: true } : m));
  return ids.every(id => acknowledged.has(id));
}
