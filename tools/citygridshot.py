#!/usr/bin/env python3
"""Skärmdump av citygriden (stadslådans catchment-rutnät) i en LEVANDE acceptansvärld.

    PANEL_USER=hexB123 python3 tools/citygridshot.py etikett [ORDINAL [take]]

Loggar in, öppnar stadslådan för spelarens första stad och klickar hex nummer
ORDINAL i rutnätet (utelämnat: ingen klick). Med `take` klickas sedan
hexens Take-knapp (delad catchment) och bilden tas efter tagningen. Skriver <etikett>.png, beskuren
till rutnätet och dess detaljpanel vid 1:1. Inloggning och miljö som panelshot.py.
"""
import os
import pathlib
import sys

from playwright.sync_api import sync_playwright

from panelshot import HOST, PW, OUT, VIEWPORT, api

USER = os.environ.get("PANEL_USER", "Wanax1")


def main():
    label = sys.argv[1] if len(sys.argv) > 1 else "citygrid"
    ordinal = sys.argv[2] if len(sys.argv) > 2 else None
    take = len(sys.argv) > 3 and sys.argv[3] == "take"

    auth = api("/auth/login", payload={"username_or_email": USER, "password": PW})
    token = auth.get("access_token") or auth["token"]

    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport={"width": VIEWPORT[0], "height": VIEWPORT[1]})
        page.goto(f"{HOST}/", wait_until="domcontentloaded")
        page.evaluate("t => localStorage.setItem('poleia_token', t)", token)
        page.context.add_cookies([{"name": "poleia_token", "value": token, "url": HOST}])
        page.goto(f"{HOST}/play", wait_until="networkidle")
        page.wait_for_function("() => window.openDrawer !== undefined", timeout=20000)
        page.evaluate("() => window.openDrawer('city')")
        page.wait_for_selector("#city-gubbe-grid .gubbe-hex-g", timeout=20000)
        if ordinal:
            page.locator(f'#city-gubbe-grid .gubbe-hex-g[data-ordinal="{ordinal}"]').click()
            page.wait_for_timeout(300)
            if take:
                page.locator("#city-gubbe-grid .gubbe-take").first.click()
                page.wait_for_timeout(1500)
                page.locator(f'#city-gubbe-grid .gubbe-hex-g[data-ordinal="{ordinal}"]').click()
                page.wait_for_timeout(300)
        OUT.mkdir(parents=True, exist_ok=True)
        path = OUT / f"{label}.png"
        page.locator("#city-gubbe-grid").screenshot(path=str(path))
        browser.close()
    print(path)


if __name__ == "__main__":
    main()
