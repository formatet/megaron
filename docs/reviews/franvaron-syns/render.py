"""Explicit synthetic output replay; does not prove backend state transitions."""
def replay_notices(page, OUT):
 screens={}
 # Explicit output fixture: renderer proof only. Backend transitions are tested separately in PostgreSQL.
 def notice(kind,tick,name,city,stamp,**body):
  return {'id':'synthetic-'+kind+'-'+city,'kind':kind,'world_tick':tick,'level':2 if kind=='GarrisonStarvationStarted' or (kind=='KharisBandChanged' and body.get('to_band') in ('Suspicious','Wrathful')) else 3,'created_at':f'2026-10-18T{stamp}:00:00Z','body':{'name':name,'settlement_id':city,'world_tick':tick,**body}}
 fixture=[notice('BuildComplete',None,'Unknown archive','old','09',building_type='farm'),notice('KharisBandChanged',8,'Realm','realm','08',from_band='Indifferent',to_band='Wrathful',kharis=3),notice('GarrisonStarvationEnded',7,'Recovered City','recovered','07'),notice('GarrisonStarvationStarted',6,'Recovered City','recovered','06'),notice('GarrisonStarvationStarted',5,'Hungry City','hungry','05',food_net_per_tick=24)]
 fixture[-1]['body'].update(q=1,r=1)
 def replay(route):
  url=route.request.url
  if '/notifications' not in url:return route.continue_()
  if route.request.method!='GET':return route.fulfill(status=200,json={})
  rows=[] if 'kind=' in url else fixture
  route.fulfill(status=200,json={'notifications':rows,'unread':len(rows)})
 page.route('**/api/v1/worlds/*/notifications**',replay)
 for size in [{'width':1280,'height':900},{'width':390,'height':844}]:
  page.set_viewport_size(size);suffix=str(size['width'])
  page.evaluate("async()=>{const {loadNotifDrawer}=await import('/static/js/megaron/ui/drawers/notif.js');await loadNotifDrawer();}")
  rows=page.locator('#notif-body .notif-list-item[data-idx]').all_text_contents()
  assert 'Hungry City' in rows[0] and 'Day #5' in rows[0],rows
  assert 'Day unknown' in rows[1],rows
  assert 'no longer losing men' in rows[3] and 'Recovered City' in rows[4],rows
  page.screenshot(path=str(OUT/('synthetic-archive-'+suffix+'.png')))
  page.locator('#notif-body .notif-list-item[data-idx]').first.click();page.wait_for_selector('#dispatch-window-overlay.open')
  detail=page.locator('#dw-body').inner_text();assert 'Day #5' in detail and 'Soldiers need grain' in detail and 'Read about this' in detail,detail
  page.screenshot(path=str(OUT/('synthetic-dispatch-'+suffix+'.png')))
  page.evaluate("closeDispatchWindow()")
  page.evaluate("n=>addDispatch({kind:n.kind,payload:n.body,world_tick:n.world_tick,id:n.id,level:n.level})",fixture[-1])
  assert page.locator('.dispatch-chip').last.locator('.dc-time').inner_text()=='Day #5'
  screens[suffix]={'synthetic_archive_rows':rows,'synthetic_dispatch':detail}
 return screens
