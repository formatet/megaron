import glob,statistics as st
from sund3 import *
def straits3(T,lid,lc,ld,W,minland,minlen=1):
    size={i:len(c) for i,c in enumerate(lc)}
    keep=[]
    for c,m in ld.items():
        if not isopen(T[c]): continue
        v=sorted((dd,L) for L,dd in m.items() if size[L]>=minland)
        if len(v)>=2 and v[0][0]+v[1][0]<=W+1: keep.append(c)
    return [x for x in comps(keep,None) if len(x)>=minlen]
if __name__=='__main__':
    maps=sorted(glob.glob('maps/*.json')); C={}
    for f in maps:
        d,T=load(f);lc,lid=analyse(T);C[f]=(T,lid,lc,landdist(T,lid))
    print("landmass size distribution (all maps): ",end='')
    sizes=sorted(len(c) for f in maps for c in C[f][2]); print("n",len(sizes),"p10",sizes[len(sizes)//10],"p25",sizes[len(sizes)//4],"med",sizes[len(sizes)//2],"max",sizes[-1], " <=3 hex:",sum(1 for s in sizes if s<=3))
    for W in (2,3):
        for ml in (1,3,6,10):
            per=[len(straits3(*[C[f][i] for i in (0,1,2,3)][:0] or (C[f][0],C[f][1],C[f][2],C[f][3],W,ml))) for f in maps]
            print(f"W<={W} landmass>={ml}: per map min {min(per)} med {st.median(per)} max {max(per)} zero-maps {sum(1 for p in per if p==0)}/{len(per)} live={per[0]}")
