#!/usr/bin/env python3
"""Read-only player language guards for four surfaces. Exceptions are exact literals.
Comments, interpolation identifiers, SQL, JSON tags, import paths and command options
are infrastructure, not player prose. New prose has no automatic exemption.
"""
from pathlib import Path
import argparse,collections,json,re
ROOT=Path(__file__).resolve().parents[1]
TOKEN=re.compile(r'//[^\n]*|/\*[\s\S]*?\*/|"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\'|`(?:\\.|[^`\\])*`')
FORBIDDEN=re.compile(r'game[ -]days?\b|(?<![\w-])ticks?\b|/tick|\bturns?\s+(?:\d|\$\{|%\d)|(?:\d+|%[-+.0-9]*[df])\s+turns?\b|/turns?\b|\bturns?\s+(?:remaining|left|to go)\b',re.I)
WALL=re.compile(r'≈.*?\bdays?\b|(?:≈|~).*?\bdays?\b.*?real time|\bin ~.*?\bd(?:ays?)?\b|\bnine hours\b|%d[d]\b|\bd ago\b',re.I)

def sources(surface):
 if surface=='web':return list((ROOT/'web/static/js/megaron').rglob('*.js'))+list((ROOT/'web/static').glob('*.html'))+list((ROOT/'web/templates').glob('*.html'))
 if surface=='codex':return list((ROOT/'web/static/codex').glob('*.md'))+[ROOT/'web/static/codex/index.json']
 if surface=='keryx':return [p for p in (ROOT/'server/cmd/keryx').glob('*.go') if not p.name.endswith('_test.go')]
 if surface=='server':return [p for d in ('api','internal') for p in (ROOT/'server'/d).rglob('*.go') if not p.name.endswith('_test.go')]
 raise ValueError(surface)
def js_strings(s):
 """Scan JS literals, including strings inside nested template expressions.
 Regexes and comments are code, not player prose. Template identifiers are not
 emitted; nested string literals still are, so new prose cannot hide in ${...}.
 """
 out=[]
 def emit(start,val):out.append((s.count('\n',0,start)+1,val))
 def quoted(i,quote):
  start=i;i+=1
  while i<len(s):
   if s[i]=='\\':i+=2;continue
   if s[i]==quote:emit(start,s[start+1:i]);return i+1
   i+=1
  return i
 def template(i):
  start=i;i+=1;chunk=i
  while i<len(s):
   if s[i]=='\\':i+=2;continue
   if s[i]=='`':emit(chunk,s[chunk:i]);return i+1
   if s.startswith('${',i):
    emit(chunk,s[chunk:i]);i=code(i+2,True);chunk=i;continue
   i+=1
  return i
 def code(i,expression=False):
  braces=0
  while i<len(s):
   if s.startswith('//',i):
    n=s.find('\n',i);i=len(s) if n<0 else n+1;continue
   if s.startswith('/*',i):
    n=s.find('*/',i+2);i=len(s) if n<0 else n+2;continue
   if s[i] in ('"',"'"):i=quoted(i,s[i]);continue
   if s[i]=='`':i=template(i);continue
   if s[i]=='/':
    before=s[:i].rstrip()
    if before and (before[-1] in '(=,:![&|?' or before.endswith('return')):
     j=i+1;inclass=False
     while j<len(s) and s[j]!='\n':
      if s[j]=='\\':j+=2;continue
      if s[j]=='[':inclass=True
      if s[j]==']':inclass=False
      if s[j]=='/' and not inclass:i=j+1;break
      j+=1
     else:i+=1
     continue
   if expression:
    if s[i]=='{':braces+=1
    if s[i]=='}':
     if braces==0:return i+1
     braces-=1
   i+=1
  return i
 code(0)
 return out

def literals(path):
 s=path.read_text()
 if path.suffix in ('.md','.html'):
  s=re.sub(r'<!--.*?-->',lambda m:'\n'*m[0].count('\n'),s,flags=re.S)
  return [(i,line) for i,line in enumerate(s.splitlines(),1)]
 if path.suffix=='.js':
  return [(line,val) for line,val in js_strings(s) if not re.search(r'/api/|/ticklog|/static/',val)]
 out=[]
 for m in TOKEN.finditer(s):
  val=m[0]
  if val.startswith(('//','/*')):continue
  # Interpolations hold identifiers, never the world-unit spelling.
  val=re.sub(r'\$\{[^}]*\}', '', val[1:-1])
  if re.search(r'\b(?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP)\b|json:|formatet/megaron/|/api/|/ticklog|/static/',val):continue
  out.append((s.count('\n',0,m.start())+1,val))
 return out

def violations(surface):
 path=ROOT/'tools/day_guard_exceptions.json'
 exceptions=json.loads(path.read_text()) if path.exists() else {}
 bad=[]
 for p in sources(surface):
  name=str(p.relative_to(ROOT))
  seen=collections.Counter()
  for line,val in literals(p):
   if not (FORBIDDEN.search(val) or WALL.search(val)):continue
   seen[val]+=1
   if seen[val] <= exceptions.get(name,{}).get('counts',{}).get(val,0):continue
   bad.append((name,line,val))
 return bad

def main():
 ap=argparse.ArgumentParser();ap.add_argument('surface',choices=['web','keryx','codex','server']);args=ap.parse_args()
 bad=violations(args.surface)
 for name,line,val in bad:print(f'{name}:{line}: forbidden player time: {val}')
 if bad:raise SystemExit(1)
 print(f'Day guard {args.surface}: PASS')
if __name__=='__main__':main()
