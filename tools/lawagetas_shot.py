#!/usr/bin/env python3
"""Lawagetas-kortet på den riktiga kartan i acceptansvärlden (Firefox, Playwright).

    python3 tools/lawagetas_shot.py UT_KATALOG [SPELARE]

Mintar en access-token för en befintlig spelare med acceptansvärldens
(offentliga) JWT_SECRET, laddar /map i Firefox vid 1440x900 och 390x844 och
sparar kort + kontoruta. SPELARE = användarnamn (default: första grundade).
Rör ingen data; skriver bara bilder.
"""
import base64, hashlib, hmac, json, subprocess, sys, time, pathlib, urllib.request

from playwright.sync_api import sync_playwright

BASE = "http://localhost:8097"
SECRET = b"acceptance-world-only-not-a-secret"
ROOT = pathlib.Path(__file__).resolve().parents[1]


def psql(sql):
    out = subprocess.run([str(ROOT / "tools/acceptance.sh"), "psql", sql], capture_output=True, text=True, check=True).stdout
    return [l for l in out.splitlines() if "|" in l]


def b64(b):
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()


def mint(pid, name):
    head = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
    now = int(time.time())
    body = b64(json.dumps({"pid": pid, "username": name, "sub": pid, "exp": now + 3600, "iat": now}).encode())
    sig = b64(hmac.new(SECRET, f"{head}.{body}".encode(), hashlib.sha256).digest())
    return f"{head}.{body}.{sig}"


def main(out, who=None):
    out = pathlib.Path(out); out.mkdir(parents=True, exist_ok=True)
    where = f"p.username = '{who}'" if who else "pwr.founded_tick IS NOT NULL"
    pid, name = psql(f"select p.id, p.username from players p join player_world_records pwr on pwr.player_id=p.id where {where} order by p.username limit 1;")[0].split("|")[:2]
    token = mint(pid, name)
    for label, (w, h) in {"desktop-1440x900": (1440, 900), "phone-390x844": (390, 844)}.items():
        with sync_playwright() as pw:
            br = pw.firefox.launch()
            ctx = br.new_context(viewport={"width": w, "height": h}, has_touch=(w < 500))
            ctx.add_cookies([{"name": "poleia_token", "value": token, "url": BASE}])
            ctx.add_init_script(f"try{{localStorage.setItem('poleia_token','{token}')}}catch(e){{}}")
            page = ctx.new_page()
            errs = []
            page.on("pageerror", lambda e: errs.append(str(e)))
            page.goto(BASE + "/play", wait_until="networkidle")
            page.wait_for_timeout(2500)
            card = page.locator("#lawagetas-card")
            print(label, "card visible:", card.is_visible(), "|", card.inner_text() if card.is_visible() else "", "| js errors:", errs)
            page.screenshot(path=str(out / f"card-{label}.png"))
            if card.is_visible():
                card.screenshot(path=str(out / f"card-only-{label}.png"))
            page.click("#gt-wanax")
            page.wait_for_timeout(1200)
            page.screenshot(path=str(out / f"account-{label}.png"))
            br.close()


if __name__ == "__main__":
    main(*sys.argv[1:3])
