#!/usr/bin/env python3
"""Compare the observed stone journey with current planning; live DB READ ONLY."""
from pathlib import Path
import json
import subprocess
import tempfile
ROOT=Path(__file__).resolve().parents[1]
sql="""BEGIN READ ONLY;
SELECT jsonb_build_object('tiles',(SELECT jsonb_agg(jsonb_build_object('Q',q,'R',r,'Terrain',terrain)) FROM map_tiles WHERE world_id=t.world_id),'origin',jsonb_build_object('Q',origin_q,'R',origin_r),'dest',jsonb_build_object('Q',dest_q,'R',dest_r),'old',journey) FROM transports t WHERE id='954453c6-e533-4b8b-be62-cfd79baf3a7e';
COMMIT;
"""
data=json.loads(subprocess.check_output(['ssh','root@10.0.1.92','runuser -u postgres -- psql -qAt -v ON_ERROR_STOP=1 -d poleia'],input=sql,text=True))
source='''package main
import("os";"encoding/json";"formatet/megaron/server/internal/province")
func main(){var input struct{Tiles []struct{Q,R int;Terrain string};Origin,Dest province.MapPosition};if e:=json.NewDecoder(os.Stdin).Decode(&input);e!=nil{panic(e)};g:=province.TileGraph{};for _,t:=range input.Tiles{g[[2]int{t.Q,t.R}]=t.Terrain};j,e:=g.PlanTradeJourney(input.Origin,input.Dest,"naval",nil);if e!=nil{panic(e)};json.NewEncoder(os.Stdout).Encode(j)}
'''
with tempfile.TemporaryDirectory(prefix='.port-proof-',dir=ROOT/'server') as directory:
 p=Path(directory)/'main.go';p.write_text(source)
 new=json.loads(subprocess.check_output(['go','run',str(p)],cwd=ROOT/'server',input=json.dumps(data),text=True))
old=data['old']
assert new['distance']<old['distance'],(old,new)
assert min(p['R'] for p in new['path'])>=16,new
print(json.dumps({'old_distance':old['distance'],'new_distance':new['distance'],'old_travel_ticks':old['travel_ticks'],'new_travel_ticks':new['travel_ticks'],'new_path':new['path'],'live_mutations':0},indent=2))
