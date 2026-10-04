#!/usr/bin/env python3
"""Real Element login acceptance with private JSON credentials, no screenshots."""
import argparse,json
from pathlib import Path
from playwright.sync_api import sync_playwright

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--fixture',required=True,type=Path);p.add_argument('--old-password',action='store_true',help='Also verify matrix_old_password is rejected');a=p.parse_args();f=json.loads(a.fixture.read_text())
 assert f['element_url'].startswith('http://127.0.0.1:')
 with sync_playwright() as playwright:
  browser=playwright.chromium.launch()
  def attempt(password,success):
   context=browser.new_context();page=context.new_page();page.goto(f['element_url']+'/#/login');page.locator('#mx_LoginForm_password').wait_for()
   assert 'agora.test' in page.locator('body').inner_text()
   page.locator('#mx_LoginForm_username').fill(f['matrix_user']);page.locator('#mx_LoginForm_password').fill(password)
   with page.expect_response(lambda r:'/_matrix/client/' in r.url and r.url.endswith('/login') and r.request.method=='POST',timeout=30000) as response:
    page.get_by_role('button',name='Sign in',exact=True).click()
   if success:
    assert response.value.status==200
    try:
     page.get_by_role('button',name='Skip verification for now',exact=True).click(timeout=5000)
     page.get_by_role('button',name="I'll verify later",exact=True).click(timeout=5000)
    except Exception:pass
    page.wait_for_url('**/#/home',timeout=30000);assert page.get_by_text('No chats yet',exact=True).count() or page.get_by_role('button',name='Home',exact=True).count() or page.get_by_text('Welcome '+f['matrix_user'],exact=False).count()
   else:
    assert response.value.status==403;assert page.locator('#mx_LoginForm_password').count()==1
   context.close()
  if a.old_password:attempt(f['matrix_old_password'],False);print('PASS: previous password rejected by actual Element login.')
  attempt(f['matrix_password'],True);print('PASS: current password logs into actual Element Web 1.12.30 home page.')
  browser.close()
if __name__=='__main__':
 try:main()
 except Exception as e:raise SystemExit('Element acceptance failed: '+type(e).__name__+' (raw details withheld to protect credentials)') from None
