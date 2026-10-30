// foreign_city.js — another ruler's city as you know it
// (megaron_kontrakt_frammande_stad). Words only, each with when and from whom;
// the server sends nothing more, so nothing here derives a number.
import { esc } from './format.js';
import { fmtTickTime } from './fmt_clock.js';

const SOURCE = {
  messenger: 'Your messenger was there',
  caravan: 'Your caravan was there',
  trade: 'Your traders were there',
  market_board: 'Your people read its market board',
};

function buildingList(list, withLevel) {
  return (list || []).map(b => withLevel && b.level != null ? `${b.name} L${b.level}` : b.name).join(', ');
}

export function foreignIntelHTML(sett) {
  const intel = sett?.intel;
  if (!intel) return '';
  const seen = [];
  if (sett.size) seen.push(sett.size);
  if (sett.besieged) seen.push('under siege');
  const blds = buildingList(sett.buildings, false);
  if (blds) seen.push(blds);
  let html = '';
  if (intel.seen_at) {
    const label = intel.seen_live ? 'Seen now' : `Seen ${fmtTickTime(null, intel.seen_at)}`;
    html += `<p class="foreign-intel">${esc(label)}: ${esc(seen.join(', '))}.</p>`;
  } else if (seen.length) {
    html += `<p class="foreign-intel">Known: ${esc(seen.join(', '))}.</p>`;
  }
  const c = intel.contact;
  if (c) {
    const parts = [c.size, c.food, ...(c.goods || []).map(g => `${g.word} ${g.good}`)].filter(Boolean);
    const who = SOURCE[intel.contact_source] || 'Your people were there';
    html += `<p class="foreign-intel">${esc(who)} ${esc(fmtTickTime(null, intel.contact_at))}: ${esc(parts.join(', '))}.</p>`;
    const then = buildingList(c.buildings, true);
    if (then) html += `<p class="empty-state">Buildings then: ${esc(then)}.</p>`;
  } else {
    html += '<p class="empty-state">No one of yours has been inside. Send a messenger, a caravan or a gift to learn more.</p>';
  }
  return html;
}
