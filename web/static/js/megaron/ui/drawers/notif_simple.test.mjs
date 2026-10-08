import test from 'node:test';
import assert from 'node:assert/strict';
import { notifDateHeader, hiddenNotifLabel } from './notif.js';
import { notifText } from '../format.js';
import { lockedActionsHTML, LAWAGETAS_BRIEFS, drawerHelpHTML } from '../misc.js';
test('K: calendar and forming state survive without world speed',()=>{
 const cal={day:3,month:2,monthName:'Antherion',year:1};
 const html=notifDateHeader(cal,{TICK_SECONDS:360,WORLD_STATE:'forming',WANAXES_NEEDED:10,WANAXES_JOINED:7});
 assert.match(html,/Day three of Antherion \(two\), Year one/);assert.match(html,/waiting for three more Wanaxes/);
 assert.doesNotMatch(html,/World speed|normal|per hour|×/,'world speed must be absent');assert.equal(notifDateHeader(null,{}),'');
});
test('K: quiet kinds remain readable both in group and historic archive',()=>{
 for(const kind of ['SitosIntervention','SitosFundLow']){
  assert.match(hiddenNotifLabel(kind,3),/^three food/);assert.doesNotMatch(hiddenNotifLabel(kind,3),/Sitos|\+3/,'group must use player words');
  assert.doesNotMatch(notifText(kind,{}),/Sitos/,'historic row must use player words');
 }
});
test('K: every short advisory has a drawer help link; locked reasons stay authoritative in tooltip',()=>{
 for(const [name,text] of Object.entries(LAWAGETAS_BRIEFS)){assert.ok(text.length<85);assert.match(drawerHelpHTML(name),new RegExp("openCodexForDrawer\\('"+name+"'\\)"));}
 const html=lockedActionsHTML('trade',[{name:'trade-offer',requirements:[{satisfied:true,hint:'irrelevant'},{satisfied:false,hint:'Send a messenger <first>'}]}]);
 assert.match(html,/Unavailable: /);assert.match(html,/title="Send a messenger &lt;first&gt;"/,'keep server reason');assert.match(html,/openCodexForDrawer\('economy'\)/);assert.doesNotMatch(html,/irrelevant/);assert.equal(lockedActionsHTML('trade',[]),'');
});
