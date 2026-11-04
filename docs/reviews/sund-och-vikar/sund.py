import json,glob,sys,collections
DIRS=[(1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1)]
AX=[((1,0),(-1,0)),((0,1),(0,-1)),((1,-1),(-1,1))]
def load(f):
    d=json.load(open(f)); T={(q,r):t for q,r,t in d['tiles']}; return d,T
def issea(t): return t in('deep_sea','coastal_sea','lake')
def isopen(t): return t in('deep_sea','coastal_sea')
def comps(cells,adj):
    cells=set(cells); out=[]; seen=set()
    for c in sorted(cells):
        if c in seen: continue
        st=[c]; seen.add(c); comp=[]
        while st:
            x=st.pop(); comp.append(x)
            for d in DIRS:
                n=(x[0]+d[0],x[1]+d[1])
                if n in cells and n not in seen: seen.add(n); st.append(n)
        out.append(comp)
    return out
def analyse(T):
    land=[c for c,t in T.items() if not issea(t)]
    lc=comps(land,None); lid={}
    for i,c in enumerate(lc):
        for x in c: lid[x]=i
    return lc,lid
def landn(T,c):
    return sum(1 for d in DIRS if (c[0]+d[0],c[1]+d[1]) in T and not issea(T[(c[0]+d[0],c[1]+d[1])]))
def isbay(T,c): return T[c]=='coastal_sea' and landn(T,c)>=4
def cur(T):
    s=[]
    for c,t in T.items():
        if not isopen(t): continue
        for a,b in AX:
            A=(c[0]+a[0],c[1]+a[1]);B=(c[0]+b[0],c[1]+b[1])
            if A in T and B in T and not issea(T[A]) and not issea(T[B]): s.append(c);break
    return s
def ray(T,c,d,R):
    x=c
    for k in range(1,R+1):
        x=(x[0]+d[0],x[1]+d[1])
        if x not in T: return None
        if not issea(T[x]): return x,k
        if T[x]=='lake': return None
    return None
def chan(T,lid,R):
    """channel hexes: open sea with land within R both ways on some axis; returns dict hex->(gap,landA,landB)"""
    out={}
    for c,t in T.items():
        if not isopen(t): continue
        best=None
        for a,b in AX:
            ra=ray(T,c,a,R); rb=ray(T,c,b,R)
            if ra and rb:
                gap=ra[1]+rb[1]-1
                if best is None or gap<best[0]: best=(gap,lid[ra[0]],lid[rb[0]])
        if best: out[c]=best
    return out
if __name__=='__main__':
    for f in sorted(glob.glob('maps/*.json')):
        d,T=load(f); lc,lid=analyse(T)
        cs=cur(T); chains=comps(cs,None)
        bays=[c for c in T if isbay(T,c)]
        print(f.split('/')[-1], 'tiles',len(T),'land comps',len(lc),'| current strait hexes',len(cs),'chains',len(chains),'1-hex',sum(1 for c in chains if len(c)==1),'chainhexes that are bays',sum(1 for c in cs if isbay(T,c)),'| bay hexes',len(bays))
