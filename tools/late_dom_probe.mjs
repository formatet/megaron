// Real Chromium DOM and actual production handlers. Only HTTP timing/data is
// substituted. Invoked by late_dom_probe.py; fresh page/module cache per case.
export async function run(name) {
 document.body.innerHTML='<div id="map-root"><canvas id="hex-canvas"></canvas></div><div id="city-body"></div><div id="war-body"></div><div id="economy-body"></div><div id="diplomacy-body"></div><div id="march-ctx"></div><div id="grid"></div><div id="kult-body"></div><div id="search-overlay"><input id="search-input"><div id="search-results"></div></div>';
 const {State}=await import('/web/static/js/megaron/state.js');
 Object.assign(State,{WORLD_ID:'W',MY_SETTLEMENT_ID:'S',provinceData:[{id:'P',settlement_id:'S',name:'Nostos',own:true,is_capital:true,q:0,r:0},{id:'P2',settlement_id:'S2',name:'Kyme',own:true,q:2,r:0},{id:'P3',settlement_id:'contact',name:'Troy',own:false}],tileData:[]});
 const pd={id:'S',population:500,labor_pool:100,army:{Spearman:100},buildings:[{type:'shipyard',level:1}],resources:{},can_recruit:[{unit:'galley',can_recruit:true}],loyalty:100,available_prayers:[{id:"prayer",name:"Prayer",god:"Zeus",min_kharis:0,affordable:true,favours:{grain:2}}]};
 const placement={total_gubbar:100,pool_size:100,buildings:[],hexes:[1,2].map(n=>({hex_ordinal:n,hex_q:n,hex_r:0,terrain:'grassland',goods:[{good_key:'grain',placed:0,cap:4,marginal_yield:1,rate_per_tick:0}]})),valid_hexes_for_building:{farm:[{q:1,r:0},{q:2,r:0}],mine:[{q:3,r:0}]}};
 const posts=[];let hook=null;const timers=[];
 const realTimeout=window.setTimeout;window.setTimeout=(fn,ms,...args)=>ms===1200?(timers.push(fn),0):realTimeout(fn,ms,...args);
 function response(data,status=200){return new Response(JSON.stringify(data),{status});}
 window.fetch=async(url,opts={})=>{
  if(hook){const result=hook(url,opts);if(result)return result;}
  if(opts.method==='POST'){posts.push({url,body:JSON.parse(opts.body||'{}')});return response({id:'sent',arrives_at:new Date(Date.now()+60000).toISOString()},201);}
  if(url==='/api/v1/buildings')return response([{type:'farm',hex_bound:true},{type:'mine',hex_bound:true},{type:'barracks',hex_bound:false}]);
  if(url==='/api/v1/goods')return response([{key:'grain',name:'Grain'},{key:'fish',name:'Fish'}]);
  if(url==='/api/v1/units')return response([{type:'galley',batch_men:40,costs:{grain:20},pop_cost:10}]);
  if(url.endsWith('/placement-options'))return response(placement);
  if(url.endsWith('/actions'))return response([{category:'province',available:false,name:'help',requirements:[]},{category:'military',available:false,name:'help',requirements:[]}]);
  if(url.endsWith('/goods'))return response([{key:'grain',name:'Grain',amount:300},{key:'fish',name:'Fish',amount:100}]);
  if(url.endsWith('/units'))return response({units:[]});
  if(/\/provinces\/P2?$/.test(url))return response({settlement:pd});
  if(url.endsWith('/retreat-default'))return response({threshold:50});
  return response([]);
 };
 const city=await import('/web/static/js/megaron/ui/drawers/city.js');
 const war=await import('/web/static/js/megaron/ui/drawers/war.js');
 const economy=await import('/web/static/js/megaron/ui/drawers/economy.js');
 const dip=await import('/web/static/js/megaron/ui/drawers/diplomacy.js');
 const grid=await import('/web/static/js/megaron/ui/citygrid.js');
 const kult=await import('/web/static/js/megaron/ui/drawers/kult.js');
 const search=await import('/web/static/js/megaron/ui/search.js');
 Object.assign(window,{onCityBuildTypeChange:city.onCityBuildTypeChange,dipToggleKind:dip.dipToggleKind});
 const el=id=>document.getElementById(id);
 const check=(condition,message)=>{if(!condition)throw new Error(name+': '+message);};
 const turn=()=>new Promise(r=>realTimeout(r,0));
 async function reached(predicate){for(let n=0;n<100&&!predicate();n++)await turn();check(predicate(),'delayed actual handler not reached');}
 function delay(suffix){let release;hook=(url,opts)=>!opts.method&&url.endsWith(suffix)?new Promise(r=>{release=data=>r(response(data));}):null;return{get release(){return release;}};}
 let pending;
 if(['city-reload','war-reload','cult-reload'].includes(name)) {
  const load=name==='city-reload'?city.loadCityDrawer:name==='war-reload'?war.loadWarDrawer:kult.loadKultDrawer;
  const d=delay('/provinces/P');pending=load();await reached(()=>d.release);hook=null;await load();
  await reached(()=>name==='city-reload'?el('wdb-inf'):name==='war-reload'?el('wrc-name-ship'):el('kult-body').querySelector('.offer-goods input'));
  const input=name==='city-reload'?el('wdb-inf'):name==='war-reload'?el('wrc-name-ship'):el('kult-body').querySelector('.offer-goods input');input.closest('.city-tab')&&(input.closest('.city-tab').style.display='');input.closest('details')&&(input.closest('details').open=true);input.value=name==='war-reload'?'Thalassa':'19';input.focus();
  d.release({settlement:pd});await pending;
  check(input.isConnected&&document.activeElement===input,'obsolete drawer load replaced active input');
 } else if(name==='search') {
  const d=delay('/inbox');search.toggleSearch();await reached(()=>d.release);
  const input=el('search-input');input.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true}));input.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true}));
  check(State.searchFocusIdx===1,'second city selected before reply');d.release([]);await turn();await turn();
  check(State.searchFocusIdx===1&&el('search-results').querySelector('.focused .sr-name')?.textContent==='Kyme','late letters reset highlighted city');
 } else if(name==='garrison'||name==='recruit') {
  const d=delay('/actions');pending=name==='garrison'?city.loadCityDrawer():war.loadWarDrawer();await reached(()=>d.release);
  const id=name==='garrison'?'wdb-inf':'wrc-name-ship';const input=el(id);check(input&&!input.disabled,'genuine editable control mounted');input.closest('.city-tab').style.display='';
  input.value=name==='garrison'?'17':'Thalassa';input.focus();if(name==='recruit')input.setSelectionRange(2,5);
  const value=input.value;d.release([{category:name==='garrison'?'province':'military',available:false,name:'help',requirements:[]}]);await pending;
  check(el(id)===input,'late hint replaced the input node');check(input.value===value,'typed value lost');check(document.activeElement===input,'focus lost');
  if(name==='recruit')check(input.selectionStart===2&&input.selectionEnd===5,'text selection lost');
 } else if(name==='transfer') {
  document.body.insertAdjacentHTML('beforeend','<select id="ec-tr-good"></select>');
  const d=delay('/P/goods');pending=economy.loadTransferGoods('P');await reached(()=>d.release);
  await economy.loadTransferGoods('P2');el('ec-tr-good').value='fish';
  d.release([{key:'grain',amount:300}]);await pending;
  check(el('ec-tr-good').value==='fish','older From reply replaced selected good');
 } else if(name==='build-hex') {
  document.body.insertAdjacentHTML('beforeend','<select id="city-build-select"><option>farm</option><option>mine</option><option>barracks</option></select><select id="city-build-hex"><option value="1,0">one</option><option value="2,0">two</option></select>');
  const d=delay('/placement-options');pending=city.onCityBuildTypeChange();await reached(()=>d.release);
  el('city-build-hex').value='2,0';d.release(placement);await pending;
  check(el('city-build-hex').value==='2,0','hex choice lost while refresh pending');
 } else if(name==='build-stale') {
  document.body.insertAdjacentHTML('beforeend','<select id="city-build-select"><option>farm</option><option>barracks</option></select><select id="city-build-hex"></select>');
  const d=delay('/placement-options');pending=city.onCityBuildTypeChange();await reached(()=>d.release);
  el('city-build-select').value='barracks';await city.onCityBuildTypeChange();
  d.release(placement);await pending;check(el('city-build-hex').style.display==='none','older farm response reopened hex picker for barracks');
 } else if(name==='build-refresh') {
  await city.loadCityDrawer();await reached(()=>el('city-build-select')?.options.length>0);await turn();
  el('city-build-select').value='farm';await city.onCityBuildTypeChange();
  const d=delay('/provinces/P');pending=city.startBuild();await reached(()=>d.release);
  el('city-build-hex').value='2,0';d.release({settlement:pd});await pending;await reached(()=>el('city-build-hex')?.options.length>0);
  check(el('city-build-hex').value==='2,0','build completion refresh lost next build hex');
 } else if(name==='grid') {
  await grid.renderGubbeGrid(el('grid'),'P',0,0);
  el('grid').querySelector('[data-ordinal="1"]').dispatchEvent(new MouseEvent('click'));
  const d=delay('/placement-options');pending=grid.renderGubbeGrid(el('grid'),'P',0,0,{selected:'hex:1',silent:true});await reached(()=>d.release);
  el('grid').querySelector('[data-ordinal="2"]').dispatchEvent(new MouseEvent('click'));
  d.release(placement);await pending;check(el('grid').querySelector('.selected')?.parentElement.dataset.ordinal==='2','silent refresh restored an obsolete hex');
 } else if(name==='correspondence') {
  await dip.loadDiplomacyDrawer();await dip.dipWrite('contact');const cid='dip-thread-Troy-compose';
  check(el(cid+'-text'),'composer exists');el(cid+'-text').value='First letter';await dip.dipSendInThread(cid,'contact');
  const d=delay('/inbox');pending=timers.shift()();await reached(()=>d.release);
  const text=el(cid+'-text');text.value='Second unfinished letter';text.focus();text.setSelectionRange(3,8);
  el(cid).querySelector('details').open=true;el(cid+'-qty').value='37';el(cid+'-good').value='fish';
  el(cid).querySelector('input[value="sell"]').checked=true;dip.dipToggleKind(cid);el(cid+'-offer-good').value='grain';el(cid+'-offer-qty').value='12';el(cid+'-want-silver').value='7';
  d.release([]);await pending;
  check(el(cid+'-text').value==='Second unfinished letter','delayed send refresh erased new draft');
  check(el(cid+'-qty').value==='37'&&el(cid+'-good').value==='fish','trade fields erased');
  check(el(cid).querySelector('details').open,'trade details collapsed');check(el(cid).querySelector('input[value="sell"]').checked,'direction reset');
  check(el(cid+'-sell-fields').style.display!=='none','sell fields hidden');check(el(cid+'-offer-good').value==='grain'&&el(cid+'-offer-qty').value==='12'&&el(cid+'-want-silver').value==='7','sell fields erased');
  check(document.activeElement===el(cid+'-text')&&el(cid+'-text').selectionStart===3&&el(cid+'-text').selectionEnd===8,'draft focus/selection lost');
  await dip.dipSendInThread(cid,'contact');check(posts.at(-1).body.message==='Second unfinished letter','preserved draft dispatch');check(posts.at(-1).body.trade_offer.kind==='sell','preserved trade dispatch');
 } else if(name==='automation') {
  await economy.loadEconomyDrawer();const tab=el('economy-body').querySelector('[data-tab="automation"]');
  const d=delay('/P/goods');tab.click();await reached(()=>d.release);
  hook=null;tab.click();await reached(()=>el('ec-so-out')?.querySelector('input'));
  el('ec-so-out').querySelector('input').value='127';d.release([{key:'grain',name:'Grain',amount:300}]);await turn();await turn();
  check(el('ec-so-out').querySelector('input').value==='127','older Automation load erased entered minimum');
 } else throw new Error('unknown probe');
 return {name,passed:true,posts};
}
