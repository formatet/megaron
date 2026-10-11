#!/usr/bin/env python3
"""Actual named-river card must separate labels and values at both widths."""
import pathlib,sys
import browser_parity as b
from playwright.sync_api import sync_playwright
out=pathlib.Path(sys.argv[1]);out.mkdir(parents=True,exist_ok=True)
with sync_playwright() as pw:
 rows=b.run_engine(pw,'firefox',b.login('Eirene'),out,{'river':b.SURFACES['river']})
 for r in rows:
  assert r['field_spacing'],'no actual river fields measured'
  for field in r['field_spacing']:
   assert field['gap']>=1,f"river label and value touch: {r['mode']} {field}"
  assert not r['errors'],r['errors']
print('PASS: actual river labels separated, Firefox 1280/390')
