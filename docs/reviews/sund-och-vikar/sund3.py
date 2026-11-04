import glob,statistics as st,collections,sys
from sund import *
def landdist(T,lid):
    """per sea hex: {landmass_id: hex distance} via BFS from each landmass through sea only (<=maxd)"""
    MAXD=7
    out=collections.defaultdict(dict)
    n=max(lid.values())+1 if lid else 0
    for L in range(n):
        front=[c for c,i in lid.items() if i==L]
        seen={c:0 for c in front}
        for dist in range(1,MAXD+1):
            nxt=[]
            for x in front:
                for d in DIRS:
                    y=(x[0]+d[0],x[1]+d[1])
                    if y in seen or y not in T or not issea(T[y]): continue
                    seen[y]=dist; nxt.append(y)
                    out[y][L]=dist
            front=nxt
    return out
def straits2(T,lid,W,minlen,ld=None):
    ld=ld or landdist(T,lid)
    keep=[]
    for c,m in ld.items():
        if not isopen(T[c]): continue
        v=sorted(m.values())
        if len(v)>=2 and v[0]+v[1]<=W+1: keep.append(c)
    return [x for x in comps(keep,None) if len(x)>=minlen]
if __name__=='__main__':
    maps=sorted(glob.glob('maps/*.json'))
    cache={}
    for f in maps:
        d,T=load(f);lc,lid=analyse(T);cache[f]=(T,lid,landdist(T,lid))
    for W,ml in [(1,1),(2,1),(2,2),(3,1),(3,2),(3,3),(4,2),(4,3),(5,3)]:
        per=[];lens=[]
        for f in maps:
            T,lid,ld=cache[f]; ch=straits2(T,lid,W,ml,ld); per.append(len(ch)); lens+=[len(c) for c in ch]
        print(f"W<={W} minlen={ml}: per map min {min(per)} med {st.median(per)} max {max(per)} | chain len med {st.median(lens) if lens else 0} p90 {sorted(lens)[int(len(lens)*.9)] if lens else 0} max {max(lens) if lens else 0} | live={per[0]}")
