#!/usr/bin/env python3
from pathlib import Path
import json
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
ROOT=Path(__file__).resolve().parent
scenarios=json.loads((ROOT/'simulation.json').read_text())['scenarios']
fig,axes=plt.subplots(4,3,figsize=(14,11),sharex=True,sharey=True)
styles=[('current_actual_start25','Current: start25, grace144','#666666','--'),('proposed_existing_caps','Proposed: start40, existing cap','#b62828','-'),('approved_no_temple_cap40','Approved: no-temple cap40','#1767a0',':')]
for i,n in enumerate([1,3,6,10]):
 for j,label in enumerate(['none','one','each_city']):
  ax=axes[i,j];ax.axhspan(1,5,color='#d96b6b',alpha=.16);ax.axhspan(5,30,color='#ddbc66',alpha=.13)
  for rule,title,color,linestyle in styles:
   s=next(s for s in scenarios if s['rule']==rule and s['cities']==n and s['temples_label']==label)
   ax.plot([x['tick'] for x in s['samples']],[x['kharis'] for x in s['samples']],label=title,color=color,linestyle=linestyle,lw=1.7)
  ax.axhline(30,color='#777777',lw=.5);ax.axhline(5,color='#777777',lw=.5)
  ax.set_title(f"{n} cities · {'0 temples' if label=='none' else '1 temple' if label=='one' else str(n)+' temples'}",fontsize=10)
  ax.set_ylim(0,55);ax.set_xlim(0,300);ax.grid(alpha=.2)
  if j==0:ax.set_ylabel('Kharis')
  if i==3:ax.set_xlabel('Ticks')
fig.suptitle('Slice19 preflight · fully fed L1, 15% devotion, scarcity1, passive0\nApproved no-temple cap40 preserves start40; rejected cap25 shown for diagnosis',fontsize=13)
handles,labels=axes[0,0].get_legend_handles_labels();fig.legend(handles,labels,loc='lower center',ncol=3,fontsize=9)
fig.tight_layout(rect=[0,.035,1,.94]);fig.savefig(ROOT/'curves.svg');fig.savefig(ROOT/'curves.png',dpi=130)
print('12 scenario panels exported to curves.svg/png')
