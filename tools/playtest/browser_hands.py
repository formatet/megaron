#!/usr/bin/env python3
"""Hands and eyes for a playtester that may only look at the screen: one Firefox page kept open, driven over HTTP.

  python3 tools/playtest/browser_hands.py OUT_DIR [PORT=8791] [WIDTH=1280] [HEIGHT=800]

  curl -s localhost:8791/goto  -d 'http://localhost:8097'
  curl -s localhost:8791/shot                      -> path of a fresh PNG (the only way to see the page)
  curl -s localhost:8791/click -d '640 400'        (add 'right' for a right-click)
  curl -s localhost:8791/type  -d 'some text'      (into whatever has focus)
  curl -s localhost:8791/key   -d 'Enter'
  curl -s localhost:8791/scroll -d '640 400 300'   (x y dy)
  curl -s localhost:8791/drag  -d '600 400 500 300'
  curl -s localhost:8791/size  -d '390 844'
  curl -s localhost:8791/quit

Every action answers with the path of a screenshot taken after it. No DOM, no page text: pictures only.
"""
import sys, time
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from playwright.sync_api import sync_playwright

out = Path(sys.argv[1]); out.mkdir(parents=True, exist_ok=True)
port = int(sys.argv[2]) if len(sys.argv) > 2 else 8791
w, h = (int(sys.argv[3]), int(sys.argv[4])) if len(sys.argv) > 4 else (1280, 800)
n = 0

with sync_playwright() as p:
    browser = p.firefox.launch()
    page = browser.new_context(viewport={'width': w, 'height': h}, has_touch=False).new_page()

    def shot():
        global n
        n += 1
        f = out / f'{n:04d}.png'
        page.screenshot(path=str(f))
        return str(f)

    def act(cmd, arg):
        a = arg.split()
        if cmd == 'goto': page.goto(arg.strip(), wait_until='load')
        elif cmd == 'click': page.mouse.click(float(a[0]), float(a[1]), button='right' if 'right' in a else 'left')
        elif cmd == 'type': page.keyboard.type(arg)
        elif cmd == 'key': page.keyboard.press(arg.strip())
        elif cmd == 'scroll': page.mouse.move(float(a[0]), float(a[1])); page.mouse.wheel(0, float(a[2]))
        elif cmd == 'drag':
            page.mouse.move(float(a[0]), float(a[1])); page.mouse.down()
            page.mouse.move(float(a[2]), float(a[3]), steps=8); page.mouse.up()
        elif cmd == 'size': page.set_viewport_size({'width': int(a[0]), 'height': int(a[1])})
        elif cmd == 'shot': pass
        else: return 'unknown command'
        page.wait_for_timeout(700)
        return shot()

    class H(BaseHTTPRequestHandler):
        def log_message(self, *_): pass
        def handle_cmd(self):
            cmd = self.path.strip('/')
            body = self.rfile.read(int(self.headers.get('Content-Length') or 0)).decode()
            if cmd == 'quit':
                self.send_response(200); self.end_headers(); self.wfile.write(b'bye\n'); raise KeyboardInterrupt
            try: res = act(cmd, body)
            except Exception as e: res = 'error: ' + str(e).splitlines()[0]
            self.send_response(200); self.end_headers(); self.wfile.write((res + '\n').encode())
        do_GET = do_POST = handle_cmd

    try: HTTPServer(('127.0.0.1', port), H).serve_forever()
    except KeyboardInterrupt: pass
    browser.close()
