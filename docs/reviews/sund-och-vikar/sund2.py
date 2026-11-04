import glob,statistics as st
from sund import *
def straits(T,lid,R,minlen,dist=True):
    ch=chan(T,lid,R)
    # keep hexes whose two land sides are different landmasses (dist) and that are not bays
    keep=[c for c,(g,a,b) in ch.items() if (a!=b or not dist) and not isbay(T,c)]
    chains=[x for x in comps(keep,None) if len(x)>=minlen]
    return chains,ch
if __name__=='__main__':
    import sys
    print("config: R=gap-ray reach (gap<=2R-1), minlen")
    for R,ml in [(1,1),(1,2),(2,1),(2,2),(2,3),(3,2),(3,3)]:
        per=[];lens=[];gaps=[]
        for f in sorted(glob.glob('maps/*.json')):
            d,T=load(f); lc,lid=analyse(T)
            ch,chm=straits(T,lid,R,ml)
            per.append(len(ch)); lens+= [len(c) for c in ch]
        print(f"R={R} minlen={ml}: strait/map min {min(per)} med {st.median(per)} max {max(per)} | chain len med {st.median(lens) if lens else 0} max {max(lens) if lens else 0} | live={per[0]}")
