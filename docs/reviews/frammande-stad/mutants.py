import subprocess, sys, re, os
ROOT='/home/tk/wt/frammande-stad'
S=ROOT+'/server/'
H='./api/handlers/'; HT='TestForeignCity'
M=[
 ('422 hint names a foreign city', 'api/handlers/province.go', ' AND world_id = $2 AND owner_id = $3`,\n\t\t\tprovinceID, worldID, viewer,', ' AND world_id = $2 AND $3::uuid IS NOT NULL`,\n\t\t\tprovinceID, worldID, viewer,', H, HT),
 ('unknown city says something else', 'api/handlers/province.go', 'writeError(w, http.StatusNotFound, "province not found")\n\t\t\t\treturn', 'writeError(w, http.StatusNotFound, "city not known")\n\t\t\t\treturn', H, HT),
 ('exact population leaks', 'api/handlers/foreign_city.go', 'body["size"] = province.SizeWord(sett.Population)', 'body["size"], body["population"] = province.SizeWord(sett.Population), sett.Population', H, HT),
 ('building level seen from outside', 'api/handlers/foreign_city.go', 'out = append(out, buildingWord(typ, god, nil))', 'one := 1\n\t\tout = append(out, buildingWord(typ, god, &one))', H, HT),
 ('siege shown out of memory', 'api/handlers/foreign_city.go', 'body["size"] = province.SizeWord(seen.Population)', 'body["size"], body["besieged"] = province.SizeWord(seen.Population), sett.Besieged', H, HT),
 ('memory shows size now', 'api/handlers/foreign_city.go', 'body["size"] = province.SizeWord(seen.Population)', 'body["size"] = province.SizeWord(sett.Population)', H, HT),
 ('memory lists buildings', 'api/handlers/foreign_city.go', 'intel["seen_at"], _ = lastSeen(seen.SeenTick, anchor)\n\tdefault:', 'intel["seen_at"], _ = lastSeen(seen.SeenTick, anchor)\n\t\tbody["buildings"] = []any{}\n\tdefault:', H, HT),
 ('starving reads hungry', 'internal/economy/contact.go', 'return "starving"', 'return "hungry"', H, HT),
 ('silver named', 'internal/economy/contact.go', 'if g == "silver" || n < 1 {', 'if n < 1 {', H, HT),
 ('plenty threshold', 'internal/economy/contact.go', 'all[i].n >= c/2', 'all[i].n >= c', H, HT),
 ('older contact overwrites', 'internal/economy/contact.go', '\n\t\t  WHERE player_contacted_settlements.contact_tick <= EXCLUDED.contact_tick`', '`', H, HT),
 ('raw stock stored', 'internal/economy/contact.go', 'json.Marshal(GoodsWords(stock, cap))', 'json.Marshal(stock)', H, HT),
 ('contact owner read live', 'internal/economy/contact.go', 'ON CONFLICT (player_id, settlement_id) DO UPDATE SET', 'ON CONFLICT (player_id, settlement_id) DO UPDATE SET owner_name = (SELECT \'Wanax Other\'),', H, HT),
 ('visitor sees live board', 'api/handlers/foreign_city.go', 'marketBoardView(ctx, h.pool, worldID, sett.ID, viewer, false)', 'marketBoardView(ctx, h.pool, worldID, sett.ID, viewer, true)', H, HT),
 ('extra key passes the whitelist', 'api/handlers/foreign_city.go', 'body["intel"] = intel', 'body["intel"], body["labor_pool"] = intel, sett.Population', H, HT),
 ('foreign buildings readable', 'api/handlers/province.go', 'if err != nil || !h.isOwnSettlement(r, settlementID) {', 'if err != nil {', H, HT, 1),
 ('foreign goods readable', 'api/handlers/province.go', 'if err != nil || !h.isOwnSettlement(r, settlementID) {', 'if err != nil {', H, HT, 2),
 ('foreign ticklog readable', 'api/handlers/province.go', 'if err != nil || !h.isOwnSettlement(r, sett.ID) {\n\t\twriteError(w, http.StatusNotFound, "no settlement here")', 'if err != nil {\n\t\twriteError(w, http.StatusNotFound, "no settlement here")', H, HT),
 ('remembered army readable', 'api/handlers/province.go', '!h.isOwnSettlement(r, sett.ID) && !h.cityInLiveSight(r, worldID, provinceID)', 'false', H, HT),
 ('deposits on unseen hexes', 'api/handlers/province.go', 'if known != nil && !known(q, rr, terrain) {', 'if false {', H, HT),
 ('contact at send, not delivery (no delivery write)', 'internal/messenger/handler.go', 'if err := economy.RecordContact(ctx, tx, e.WorldID, senderID, destinationID, economy.ContactMessenger, e.DueTick); err != nil {', 'if err := error(nil); err != nil {', './internal/messenger/', 'TestMessengerContact'),
]
only=sys.argv[1:] 
port=56800
for i,m in enumerate(M):
  name,f,old,new,pkg,run=m[:6]; occ=m[6] if len(m)>6 else None
  if only and str(i) not in only: continue
  p=S+f; s=open(p).read()
  if occ is None:
    if s.count(old)!=1: print(f'[{i}] SETUP {name}: count={s.count(old)}'); continue
    s2=s.replace(old,new)
  else:
    idx=-1
    for _ in range(occ): idx=s.index(old, idx+1)
    s2=s[:idx]+new+s[idx+len(old):]
  open(p,'w').write(s2)
  port+=1
  out=subprocess.run(['tools/gotest.sh',pkg,'-run',run,'-count=1'],cwd=ROOT,capture_output=True,text=True,env={**os.environ,'GOTEST_PORT':str(port)}).stdout
  subprocess.run(['git','checkout','--',S+f],cwd=ROOT)
  hits=sorted(set(re.findall(r'\w+_test\.go:\d+',out)))
  build=('build failed' in out) or ('[build failed]' in out)
  if build: verdict='BUILD-FAIL'
  elif hits: verdict='KILLED '+','.join(hits)
  elif 'panic' in out: verdict='KILLED (panic)'
  elif re.search(r'^ok\s',out,re.M): verdict='SURVIVED'
  else: verdict='UNCLEAR: '+out[-300:]
  print(f'[{i}] {name}: {verdict}', flush=True)
