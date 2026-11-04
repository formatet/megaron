import sys
from sund3 import *
from render import render
f=sys.argv[1];W=int(sys.argv[2]);q0,q1,r0,r1=map(int,sys.argv[3:7])
d,T=load(f);lc,lid=analyse(T);ch=straits2(T,lid,W,1)
print(f"{f} W<={W}: {len(ch)} straits, lens {sorted(len(c) for c in ch)}")
render(T,lid,ch,(q0,q1),(r0,r1))
