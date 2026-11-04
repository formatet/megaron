import { esc } from '../format.js';
import { kharisStandingHTML, kharisNetView, kharisDecayHTML, kharisRulesHTML, templeTitheHTML } from './kult_kharis.js';

// One button and a good to choose (nittiotal): turns the temple's rich offering on or off.
export function richOfferingControlHTML(t) {
 const chosen = t.offer?.rich_offering_good || '';
 if (chosen) return `<button class="btn-small" data-cult-rich-off="${esc(t.id)}">Stop rich offering</button> `;
 const held = Object.entries(t.offer?.stock || {}).filter(([, n]) => n > 0).map(([g]) => g).sort();
 if (!held.length) return '';
 return `<select class="btn-small" data-cult-rich-good="${esc(t.id)}">${held.map(g => `<option value="${esc(g)}">${esc(g)}</option>`).join('')}</select> <button class="btn-small" data-cult-rich-on="${esc(t.id)}">Add rich offering</button> `;
}

export const prayerActionID = p => p.settlement_id + '__' + p.id;
const goods = (value, separator = ' + ') => Object.entries(value||{}).map(([g,n])=>esc(String(Number(Number(n).toPrecision(4))))+' '+esc(g)).join(separator)||'—';

export function realmKultHTML(view,active='kharis',statusHTML=()=> '') {
 const pd=view.kharis||{}, net=kharisNetView(pd,'realm');
 const kharis=kharisStandingHTML(pd)+net.netHtml+kharisDecayHTML(pd)+net.idleHtml+kharisRulesHTML(pd);
 const prayers=(view.prayers||[]).map(p=>{
  const id=prayerActionID(p);
  return `<div class="dsec" data-cult-prayer="${esc(id)}"><div class="dsec-title">${esc(p.name)} — ${esc(p.city_name)}</div><p>${esc(p.temple||p.god)} · Required kharis: ${esc(String(p.min_kharis))}</p><p>${esc(p.effect||'')}</p><p>Offering: ${goods(p.offering)}</p>${statusHTML({...p,id})}<div class="offer-row" id="offer-row-${esc(id)}"></div></div>`;
 }).join('')||'<p class="empty-state">No cities</p>';
 const temples=(view.temples||[]).map(t=>`<div class="dsec" data-cult-temple="${esc(t.id)}"><div class="dsec-title">${esc(t.name)} — ${esc(t.city_name)}</div><p>Level ${esc(String(t.level))} · Temple workers ${esc(String(t.staffed))}/${esc(String(t.capacity))}</p><p>Reference: ${goods(t.offer?.required_goods)} per game day<br>Stock: ${goods(t.offer?.stock)}<br>Next gift: ${(t.offer?.paid_fraction>0 || t.offer?.fed)?goods(t.offer.offering):'unfunded (nothing taken)'}${templeTitheHTML(t.offer)}</p>${richOfferingControlHTML(t)}<button class="btn-small" data-cult-city="${esc(t.province_id)}">Manage in City</button> <button class="btn-small" data-cult-demolish="${esc(t.id)}">Demolish</button><div id="cult-demolish-${esc(t.id)}"></div></div>`).join('')||'<p class="empty-state">No temples</p>';
 const unbuilt=(view.unbuilt_temples||[]).map(t=>`<div class="stat-row"><span class="sr-label">${esc(t.name)} — ${esc(t.city_name)}</span><span class="sr-val"><button class="btn-small" data-cult-city="${esc(t.province_id)}">Open City to build</button></span></div>`).join('');
 const panels={kharis,prayers,temples:temples+(unbuilt?'<div class="dsec"><div class="dsec-title">Not built</div>'+unbuilt+'</div>':'')};
 return '<div class="drawer-tabs" role="tablist">'+Object.keys(panels).map(key=>`<button class="dtab${active===key?' active':''}" data-cult-tab="${key}" role="tab" aria-selected="${active===key}">${key[0].toUpperCase()+key.slice(1)}</button>`).join('')+'</div>'+Object.entries(panels).map(([key,html])=>`<div class="city-tab" data-cult-panel="${key}"${active!==key?' hidden':''}>${html}</div>`).join('');
}
