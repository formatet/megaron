import glob,math,re
from sund4 import *
maps=sorted(glob.glob('maps/*.json'))
for W in (2,3,4):
    ok=0;tot=0;rows=[]
    for f in maps:
        if 'live' in f: continue
        m=re.search(r'm_(\d+)x(\d+)_',f);w,h=int(m[1]),int(m[2])
        floor=max(3,round(math.sqrt(w*h)/15))
        d,T=load(f);lc,lid=analyse(T);n=len(straits3(T,lid,lc,landdist(T,lid),W,1))
        tot+=1;ok+= n>=floor
    print(f"W<={W}: maps meeting today's floor {ok}/{tot}")
