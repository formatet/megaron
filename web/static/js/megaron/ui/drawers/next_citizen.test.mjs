import test from 'node:test';
import assert from 'node:assert/strict';
import {State} from '../../state.js';
import {nextCitizenText,nextCitizenHTML,populationHTML} from './city_production.js';
const pd=(state='growing',ticks=4)=>({population:1064,next_citizen_progress:64,next_citizen_growth_per_tick:9,next_citizen_in_ticks:ticks,food:{growth_state:state}});
test('next citizen uses server progress and runtime cadence, ten whole steps',()=>{
 State.TICK_SECONDS=1800;
 assert.equal(nextCitizenText(pd()),'64 / 100 · in about 4 game days (about 2 hours)');
 const html=nextCitizenHTML(pd());
 assert.equal((html.match(/class="citizen-step(?: filled)?"/g)||[]).length,10);
 assert.equal((html.match(/class="citizen-step filled"/g)||[]).length,6);
 assert.match(html,/aria-valuenow="64"/);
 assert.doesNotMatch(html,/style=|gradient/);
 const full=populationHTML(pd(),0,0,'p');assert.ok(full.indexOf('Citizens')<full.indexOf('Next citizen'));assert.ok(full.indexOf('Next citizen')<full.indexOf('Free to work'));
 State.TICK_SECONDS=3600;assert.match(nextCitizenText(pd()),/about 4 hours/);
});
test('next citizen names stalled states and next game day, never invents ETA',()=>{
 for(const [state,text] of [['holding','not growing'],['capped','the walls are full'],['shrinking','shrinking']]) {
  assert.equal(nextCitizenText(pd(state,null)),`64 / 100 · ${text}`);
 }
 assert.match(nextCitizenText(pd('growing',1)),/next game day/);
 assert.doesNotMatch(nextCitizenText(pd('growing',null)),/in about|next game day/);
 assert.equal(nextCitizenHTML({population:1064}), '');
});
