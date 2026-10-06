import { esc } from './format.js';

// Use the same player-scoped sources as Correspondence and Gossip. Search is
// deliberately not an archive, nor a new source of map/contact information.
export async function loadSearchMessages(worldID, read) {
  const sources = [
    ['inbox', 'Received letters', 'messengers/inbox'],
    ['gossip', 'Rumours', 'gossip'],
  ];
  const responses = await Promise.allSettled(sources.map(async ([, , path]) => {
    const response = await read(`/api/v1/worlds/${worldID}/${path}`);
    if (!response.ok) throw new Error('Search source unavailable');
    const rows = await response.json();
    if (rows !== null && !Array.isArray(rows)) throw new Error('Invalid search response');
    return rows || [];
  }));
  const result = { inbox: [], gossip: [], failed: [] };
  responses.forEach((response, i) => {
    const [key, label] = sources[i];
    if (response.status === 'fulfilled') result[key] = response.value;
    else result.failed.push(label);
  });
  return result;
}

export function searchMessagesHTML(query, { inbox = [], gossip = [] } = {}) {
  const q = query.trim().toLowerCase();
  if (!q) return '';
  const matches = (...fields) => fields.some(s => String(s || '').toLowerCase().includes(q));
  const letters = inbox.filter(m => matches(m.message, m.from_name, m.to_name));
  const rumours = gossip.filter(g => matches(g.text, g.source_region, g.category));
  // No user text enters an inline event handler. Drawer targets are constants.
  const row = (drawer, icon, text, type) => `<div class="sr-item" data-search-drawer="${drawer}">
    <span class="sr-icon">${icon}</span><span class="sr-name">${esc(text)}</span>
    <span class="sr-type">${type}</span></div>`;
  let html = '';
  if (letters.length) {
    html += '<div class="sr-category">Received letters</div>';
    html += letters.map(m => row('diplomacy', '✉', `${m.from_name || 'Unknown'} → ${m.to_name || 'You'}: ${m.message || ''}`, 'Letter')).join('');
  }
  if (rumours.length) {
    html += '<div class="sr-category">Rumours</div>';
    html += rumours.map(g => row('gossip', '🗣', `${g.source_region || 'Unknown region'}${g.category ? ' · ' + g.category : ''}: ${g.text || ''}`, 'Rumour')).join('');
  }
  return html;
}
