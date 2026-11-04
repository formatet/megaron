import sys
from sund import *
from sund2 import straits
def render(T,lid,chains,qr,rr,marks={}):
    sid={}
    for i,c in enumerate(chains):
        for x in c: sid[x]=chr(ord('A')+i%26)
    q0,q1=qr;r0,r1=rr
    for r in range(r0,r1+1):
        line=' '*(r-r0)
        for q in range(q0,q1+1):
            c=(q,r);t=T.get(c)
            if c in marks: ch=marks[c]
            elif c in sid: ch=sid[c]
            elif t is None: ch=' '
            elif t=='deep_sea': ch='.'
            elif t=='coastal_sea': ch='b' if isbay(T,c) else ','
            elif t=='lake': ch='~'
            elif 'river' in t: ch='='
            else: ch='#'
            line+=ch+' '
        print(f"{r:3d} "+line)
    print("    q",q0,"..",q1,"(each row shifted right half a cell per r)")
if __name__=='__main__':
    f=sys.argv[1];R=int(sys.argv[2]);ml=int(sys.argv[3]);q0,q1,r0,r1=map(int,sys.argv[4:8])
    d,T=load(f);lc,lid=analyse(T);ch,_=straits(T,lid,R,ml)
    render(T,lid,ch,(q0,q1),(r0,r1))
