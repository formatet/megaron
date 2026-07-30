#!/usr/bin/env python3
"""Skärmdump av inspect-panelen i en LEVANDE acceptansvärld.

    python3 tools/panelshot.py host-baseline

Till skillnad från shot.py, som fotograferar frysta offline-riggar, loggar den
här in i acceptansvärlden och öppnar en riktig panel över riktig serverdata.
Det är den enda vägen till en ögonkoll på paneler vars innehåll kommer från
API:t — grundningsprognosen finns inte i någon handsatt fixtur.

Miljö (default = tools/acceptance.sh):
    PANEL_HOST   http://localhost:8097
    PANEL_USER   Wanax1
    PANEL_PW     acceptance-pw-123
    SHOT_DIR     .

Skriver <etikett>.png. Panelen är 220 px bred och sitter i högerkanten; bilden
beskärs till den plus lite marginal, eftersom det är panelen som bedöms och en
helskärmsbild gör klippningen omöjlig att se.
"""
import json
import os
import pathlib
import sys
import urllib.request

from playwright.sync_api import sync_playwright

HOST = os.environ.get("PANEL_HOST", "http://localhost:8097")
USER = os.environ.get("PANEL_USER", "Wanax1")
PW = os.environ.get("PANEL_PW", "acceptance-pw-123")
OUT = pathlib.Path(os.environ.get("SHOT_DIR", "."))

VIEWPORT = (int(os.environ.get("PANEL_W", 1280)), int(os.environ.get("PANEL_H", 800)))


def api(path, token=None, payload=None):
    req = urllib.request.Request(f"{HOST}/api/v1{path}")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    data = json.dumps(payload).encode() if payload is not None else None
    with urllib.request.urlopen(req, data, timeout=15) as r:
        return json.load(r)


def main():
    label = sys.argv[1] if len(sys.argv) > 1 else "panel"

    auth = api("/auth/login", payload={"username_or_email": USER, "password": PW})
    token = auth.get("access_token") or auth["token"]
    world = api("/worlds", token)
    world_id = (world["worlds"] if isinstance(world, dict) else world)[0]["id"]

    units = api(f"/worlds/{world_id}/units", token).get("units", [])
    host = next((u for u in units if u["type"] == "nomadic_host"), None)
    if not host:
        sys.exit("panelshot: ingen nomadic_host — kör tools/acceptance.sh player NAMN")

    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport={"width": VIEWPORT[0], "height": VIEWPORT[1]})
        # Bearer i localStorage för API-anropen, cookie för sidnavigeringen —
        # webben använder båda (CLAUDE.md §Auth G3).
        page.goto(f"{HOST}/", wait_until="domcontentloaded")
        page.evaluate("t => localStorage.setItem('poleia_token', t)", token)
        page.context.add_cookies([
            {"name": "poleia_token", "value": token, "url": HOST}
        ])
        page.goto(f"{HOST}/play", wait_until="networkidle")
        page.wait_for_function("() => window.centreOn !== undefined", timeout=20000)
        # Centrera på hostens hex och KLICKA den, i stället för att anropa
        # panelfunktionen direkt: openHexPanel är inte exporterad, och en riktig
        # klick är ändå det starkare beviset (megaron_arbetssatt §8, användargrinden).
        page.evaluate("h => window.centreOn(h.q, h.r)", {"q": host["q"], "r": host["r"]})
        page.wait_for_timeout(300)
        box = page.locator("#hex-canvas").bounding_box()
        page.mouse.click(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
        # Prognosen hämtas asynkront — utan den är panelen kort och buggen syns inte.
        page.wait_for_function(
            "() => { const e = document.getElementById('ip-found-preview');"
            "        return e && !e.textContent.includes('Hämtar'); }",
            timeout=20000)
        page.wait_for_timeout(400)

        OUT.mkdir(parents=True, exist_ok=True)
        path = OUT / f"{label}.png"
        page.locator("#inspect-panel").screenshot(path=str(path))

        # Mätvärdet, inte bara bilden: sticker fotens innehåll ut under panelen?
        overflow = page.evaluate("""() => {
            const p = document.getElementById('inspect-panel');
            const f = document.getElementById('ip-foot');
            const b = document.getElementById('ip-settle-btn');
            const pr = p.getBoundingClientRect();
            return {
              panel_bottom: Math.round(pr.bottom),
              foot_bottom: Math.round(f.getBoundingClientRect().bottom),
              foot_clipped_px: Math.round(f.scrollHeight - f.clientHeight),
              button_bottom: b ? Math.round(b.getBoundingClientRect().bottom) : null,
              button_reachable: b ? Math.round(b.getBoundingClientRect().bottom) <= Math.round(pr.bottom) + 1 : null,
            };
        }""")
        browser.close()

    print(f"{path}")
    for k, v in overflow.items():
        print(f"  {k:20} {v}")


if __name__ == "__main__":
    main()
