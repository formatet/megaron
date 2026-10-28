import {esc} from './format.js';
import {fetchAuth} from '../api.js';
import {State} from '../state.js';
import {fmtDays} from './fmt_num.js';
import {fmtTickTime} from './fmt_clock.js';

export function transportChoicesHTML(data,id,preferred='') {
 const options=data.options||[];
 const selected=[preferred,data.selected_by,...options.map(o=>o.by)].find(by=>options.some(o=>o.by===by&&o.available));
 return '<fieldset><legend>Send goods by</legend>'+options.map(o=>{
  const seconds=Number(o.travel_seconds)||0,minutes=Math.ceil(seconds/60);
  const wall=minutes>=60?`${Math.floor(minutes/60)}h ${minutes%60}m`:`${minutes}m`;
  const time=o.available?`${fmtDays(o.travel_ticks)} · about ${wall}${o.arrives_at?' · arrives '+fmtTickTime(null,o.arrives_at):''}`:o.reason||'Unavailable';
  const details=[o.risk,...(o.requirements||[])].filter(Boolean).join(' · ');
  return `<label style="display:block;margin:.4rem 0"><input type="radio" id="${esc(id)}-${esc(o.by)}" name="${esc(id)}-by" value="${esc(o.by)}"${o.by===selected?' checked':''}${o.available?'':' disabled'}> <strong>${o.by==='sea'?'Sea':'Land'}</strong> · ${esc(time)}<small style="display:block">${esc(details)}</small></label>`;
 }).join('')+'</fieldset>';
}
export function selectedTransportBy(id) {return document.getElementById(id)?.querySelector?.('input:checked')?.value||'';}
let sequence=0;
const requests=new WeakMap();
export async function loadTransportChoices(id,origin,destination,quantity=0,preferredBy='') {
 const el=document.getElementById(id);if(!el)return;
 const seq=++sequence,world=State.WORLD_ID,preferred=selectedTransportBy(id)||preferredBy;requests.set(el,seq);
 el.textContent='Checking land and sea routes…';
 if(!origin||!destination||origin===destination){el.textContent='Choose two different cities to compare routes.';return;}
 try{
  const r=await fetchAuth(`/api/v1/worlds/${world}/settlements/${encodeURIComponent(origin)}/transport-options?destination_id=${encodeURIComponent(destination)}&quantity=${encodeURIComponent(quantity)}`);
  const d=await r.json();
  if(requests.get(el)!==seq||State.WORLD_ID!==world||document.getElementById(id)!==el)return;
  el.innerHTML=r.ok?transportChoicesHTML(d,id,preferred):`<p>${esc(d.error||'Could not check routes.')}</p>`;
 }catch(_){if(requests.get(el)===seq&&document.getElementById(id)===el)el.textContent='Could not check routes.';}
}
