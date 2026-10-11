#!/usr/bin/env python3
"""Physical CSS/probe mutations, named red and byte-identical restoration."""
import os,pathlib,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'docs/reviews/paritet-20261011'
cases=[
 ('clip-guard',ROOT/'tools/browser_parity.py','centreExposed(e,r) && ','',['python3','tools/browser_parity_probe.py'],'scrolled-away control wrongly counted as covered'),
 ('field-gap',ROOT/'web/static/map.html','column-gap: .4rem; ','',['python3','tools/parity_field_check.py',str(OUT/'mutation-css')],'river label and value touch'),
]
for name,path,before,after,command,expected in cases:
 saved=path.read_bytes()
 try:
  text=saved.decode();assert text.count(before)==1;path.write_text(text.replace(before,after))
  result=subprocess.run(command,cwd=ROOT,capture_output=True,text=True)
  (OUT/('mutation-'+name+'.log')).write_text(result.stdout+result.stderr)
  assert result.returncode!=0 and expected in result.stderr,(name,result.stdout,result.stderr)
  print(name+': named red',flush=True)
 finally:path.write_bytes(saved)
for name,command in [('probe',['python3','tools/browser_parity_probe.py']),('fields',['python3','tools/parity_field_check.py',str(OUT/'restored-fields')])]:
 result=subprocess.run(command,cwd=ROOT,capture_output=True,text=True)
 (OUT/('restored-'+name+'.log')).write_text(result.stdout+result.stderr);assert result.returncode==0,result.stderr
 print(name+': restored green',flush=True)
