import {esc} from './format.js';
import {unitTypeLabel} from './actornames.js';
const types=new Set(['spearman','elite_infantry','war_chariot','galley','war_galley','merchantman']);
export function disbandPeople(u) { return u.status==='laid_up'?0:u.category==='naval'?(u.crew ?? 0):u.size; }
export function disbandUnitsHTML(units,settlementID,provinceID) {
 const here=units.filter(u=>u.settlement_id===settlementID && ['garrison','laid_up'].includes(u.status) && types.has(u.type));
 if(!here.length)return '<p class="empty-state">No whole units available to disband here.</p>';
 return here.map(u=>`<div class="disband-unit"><svg class="disband-figure" viewBox="0 0 8 12" aria-hidden="true"><path fill="currentColor" d="M3 0h2v3H3zM2 4h4v4H5v4H4V9H3v3H2V8H1V5h1z"/></svg><div class="disband-info"><strong>${esc(u.display_name || u.name || unitTypeLabel(u.type))}</strong><div>${u.status==='laid_up'?'Laid-up hull · ':''}Returns ${disbandPeople(u)} people</div></div><button class="btn-small" data-disband-unit="${esc(u.id)}" onclick="warDisband('${esc(provinceID)}','${esc(u.id)}',this)">Disband</button></div>`).join('');
}
