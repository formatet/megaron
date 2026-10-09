#!/usr/bin/env python3
"""Webbläsarparitet: huvudytorna i Firefox/Chromium/WebKit, desktop + 390 px.

    python3 tools/browser_parity.py [--out DIR] [--browsers firefox chromium webkit]

Loggar in i acceptansvärlden (tools/acceptance.sh) som en riktig spelare, klickar
fram varje yta via sin riktiga knapp och MÄTER i stället för att jämföra bilder.
En andra spelare UTAN stad (PANEL_HOST_USER, skapas med `tools/acceptance.sh player
Nomad`) mäter Host-panelen — det första en ny Wanax möter — öppnad genom ett klick
på hosten mitt på kartan, med Details stängd och öppen. Mätningen:

  - horisontell scroll på sidan
  - ytans ruta utanför viewporten
  - synliga kontroller/text som sticker ut ur ytans ruta (klippta)
  - knappar vars mitt täcks av något annat (går inte att trycka på)
  - JS-fel (pageerror) per yta
  - ytans huvudknapp (t.ex. "Found the metropolis here") utanför bild eller täckt
  - ytans innehållshöjd per motor — avviker en motor > 15 % från medianen flaggas den

Skärmdumpar sparas per motor/läge/yta så att en flaggad rad kan ses sida vid sida.
Exitkod 1 om någon fynd-rad finns. Allt som sägs om ytorna gäller den commit
`tools/acceptance.sh provenance` skriver ut.
"""
import argparse
import json
import os
import pathlib
import statistics
import sys
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright

HOST = os.environ.get("PANEL_HOST", "http://localhost:8097")
USER = os.environ.get("PANEL_USER", "Agamemnon")
PW = os.environ.get("PANEL_PW", "acceptance-pw-123")
HOST_USER = os.environ.get("PANEL_HOST_USER", "Nomad")

# yta → (hur den öppnas, selektorn för dess ruta)
SURFACES = {
    "map": (None, "#hex-canvas"),
    "city": ("#trig-city", "#drawer-city"),
    "war": ("#trig-war", "#drawer-war"),
    "diplomacy": ("#trig-diplomacy", "#drawer-diplomacy"),
    "economy": ("#trig-economy", "#drawer-economy"),
    "kult": ("#trig-kult", "#drawer-kult"),
    "report": ("#trig-report", "#drawer-report"),
    "notif": ("#gt-notif-btn", "#drawer-notif"),
    "codex": ("#gt-codex-btn", "#codex-panel"),
    "account": ("#gt-wanax", "#account-window-overlay .dispatch-window"),
}
# Host-fasen: kartan öppnar centrerad på hosten, så ett klick mitt på kartan är
# spelarens eget klick på sin host. Tredje fältet: knappen som MÅSTE gå att trycka på.
HOST_SURFACES = {
    "host": ("#hex-canvas", "#inspect-panel", "#ip-settle-btn"),
    "host-details": ("#ip-host-details > summary", "#inspect-panel", "#ip-settle-btn"),
}
MODES = {"desktop": (1280, 900), "mobile": (390, 844)}

MEASURE = r"""
([sel, reach]) => {
  const box = document.querySelector(sel);
  const out = {found: !!box, page_hscroll: document.documentElement.scrollWidth > innerWidth + 1};
  if (!box) return out;
  const b = box.getBoundingClientRect();
  out.box = {left: Math.round(b.left), right: Math.round(b.right), top: Math.round(b.top), bottom: Math.round(b.bottom)};
  out.box_outside = b.left < -1 || b.right > innerWidth + 1;
  // rutan har fast höjd; det är innehållet i dess scrollande kropp som kan skilja mellan motorer
  out.content_height = (box.querySelector('.drawer-body, .dw-body, #codex-body, .inspect-body') || box).scrollHeight;
  // en canvas ritar i sina egna pixlar: de måste följa behållaren, annars blir kartan en remsa
  if (box.tagName === 'CANVAS') out.canvas_mismatch = box.width !== box.parentElement.clientWidth || box.height !== box.parentElement.clientHeight
    ? `canvas ${box.width}×${box.height}, behållare ${box.parentElement.clientWidth}×${box.parentElement.clientHeight}` : null;
  const visible = e => { const s = getComputedStyle(e); const r = e.getBoundingClientRect();
    return s.visibility !== 'hidden' && s.display !== 'none' && r.width > 0 && r.height > 0; };
  const label = e => (e.id ? '#' + e.id : e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.trim().split(/\s+/).join('.') : '')) + ' «' + (e.textContent || e.value || '').trim().slice(0, 40) + '»';
  const sticking = [], clipped = [], covered = [];
  for (const e of box.querySelectorAll('button, input, select, textarea, a, span, td, th, label, div, p, h1, h2, h3, h4, li')) {
    if (!visible(e)) continue;
    const r = e.getBoundingClientRect();
    // utanför rutan i sidled (vertikalt hanteras rutan med scroll)
    if (r.right > b.right + 1 || r.left < b.left - 1) {
      // räkna bara det yttersta elementet som sticker ut
      const p = e.parentElement; const pr = p && p.getBoundingClientRect();
      if (!pr || !(pr.right > b.right + 1 || pr.left < b.left - 1)) sticking.push(label(e) + ` [${Math.round(r.left)}..${Math.round(r.right)}]`);
    }
    // text som klipps utan avsiktlig ellips
    const s = getComputedStyle(e);
    if (e.children.length === 0 && (e.textContent || '').trim() && e.scrollWidth > e.clientWidth + 1 &&
        s.overflowX !== 'visible' && s.textOverflow !== 'ellipsis' && s.overflowX !== 'auto' && s.overflowX !== 'scroll')
      clipped.push(label(e) + ` [${e.scrollWidth}>${e.clientWidth}]`);
    // knappar i viewporten som inte går att träffa
    if (e.matches('button, a, select, input') && r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth &&
        r.top >= b.top && r.bottom <= b.bottom) {
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      if (hit && !e.contains(hit) && !hit.contains(e)) covered.push(label(e) + ' ← ' + label(hit));
    }
  }
  // huvudknappen: helt i bild och träffbar i sin mitt — annars når spelaren inte handlingen
  if (reach) {
    const k = document.querySelector(reach);
    if (!k || !visible(k)) out.unreachable = reach + ' saknas';
    else {
      const r = k.getBoundingClientRect();
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      if (r.top < 0 || r.bottom > innerHeight || r.left < 0 || r.right > innerWidth)
        out.unreachable = label(k) + ` utanför bild [${Math.round(r.left)},${Math.round(r.top)}..${Math.round(r.right)},${Math.round(r.bottom)}]`;
      else if (!hit || !(k.contains(hit) || hit.contains(k))) out.unreachable = label(k) + ' täckt av ' + (hit ? label(hit) : 'inget');
    }
  }
  Object.assign(out, {sticking, clipped, covered});
  return out;
}
"""


def login(user):
    req = urllib.request.Request(f"{HOST}/api/v1/auth/login",
                                 json.dumps({"username_or_email": user, "password": PW}).encode(),
                                 {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=15) as r:
        d = json.load(r)
    return d.get("access_token") or d["token"]


def run_engine(pw, engine, token, out, surfaces=SURFACES):
    rows = []
    browser = getattr(pw, engine).launch()
    for mode, (w, h) in MODES.items():
        ctx = browser.new_context(viewport={"width": w, "height": h}, locale="en-GB",
                                  timezone_id="Europe/Stockholm", has_touch=mode == "mobile")
        ctx.add_cookies([{"name": "poleia_token", "value": token, "url": HOST}])
        ctx.add_init_script(f"localStorage.setItem('poleia_token', {json.dumps(token)})")
        page = ctx.new_page()
        errors = []
        page.on("pageerror", lambda e: errors.append(str(e)))
        page.goto(f"{HOST}/play", wait_until="networkidle")
        page.wait_for_function("() => window.centreOn !== undefined", timeout=20000)
        page.wait_for_timeout(800)
        for name, (trigger, sel, *reach) in surfaces.items():
            before = len(errors)
            if trigger:
                # 390 px trycker med fingret, som en mobilspelare. (Playwrights Firefox med
                # has_touch skickar inga pointer-händelser för ett musklick — kartan, som
                # lyssnar på Pointer Events, ser då inget klick. Mätt 2026-10-09.)
                el = page.locator(trigger).first
                el.tap() if mode == "mobile" else el.click()
                page.wait_for_timeout(900)
            m = page.evaluate(MEASURE, [sel, reach[0] if reach else None])
            m.update(engine=engine, mode=mode, surface=name, errors=errors[before:])
            page.screenshot(path=str(out / f"{engine}-{mode}-{name}.png"))
            rows.append(m)
            if trigger and surfaces is SURFACES:  # stäng igen (Host-ytorna bygger på varandra) med samma knapp, sedan Escape som reserv
                page.keyboard.press("Escape")
                page.wait_for_timeout(200)
                if page.evaluate(f"(() => {{ const e = document.querySelector({json.dumps(sel)}); return e && e.getBoundingClientRect().width > 0 && getComputedStyle(e).visibility !== 'hidden' && e.classList.contains('open'); }})()"):
                    page.locator(trigger).first.click(force=True)
                    page.wait_for_timeout(300)
        ctx.close()
    browser.close()
    return rows


def findings(rows):
    out = []
    for r in rows:
        tag = f"{r['engine']:8} {r['mode']:7} {r['surface']:9}"
        if not r["found"]:
            out.append(f"{tag} ytan hittades inte")
            continue
        if r["page_hscroll"]: out.append(f"{tag} sidan scrollar i sidled")
        if r["box_outside"]: out.append(f"{tag} rutan utanför viewporten {r['box']}")
        if r.get("canvas_mismatch"): out.append(f"{tag} {r['canvas_mismatch']}")
        if r.get("unreachable"): out.append(f"{tag} går inte att trycka på: {r['unreachable']}")
        for k in ("sticking", "clipped", "covered"):
            for x in r[k]: out.append(f"{tag} {k}: {x}")
        for e in r["errors"]: out.append(f"{tag} JS-fel: {e[:160]}")
    # innehållshöjd per motor, samma yta och läge
    for mode in MODES:
        for s in [*SURFACES, *HOST_SURFACES]:
            hs = {r["engine"]: r.get("content_height") for r in rows if r["mode"] == mode and r["surface"] == s and r.get("content_height")}
            if len(hs) < 2: continue
            med = statistics.median(hs.values())
            for eng, v in hs.items():
                if med and abs(v - med) / med > 0.15:
                    out.append(f"{eng:8} {mode:7} {s:9} höjd {v} mot median {med:.0f} ({hs})")
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", type=pathlib.Path, default=pathlib.Path("browser-parity"))
    ap.add_argument("--browsers", nargs="+", default=["firefox", "chromium", "webkit"])
    a = ap.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    token = login(USER)
    try:
        host_token = login(HOST_USER)
    except urllib.error.HTTPError:
        sys.exit(f"ingen spelare {HOST_USER} — skapa en utan stad: tools/acceptance.sh player {HOST_USER}")
    rows = []
    with sync_playwright() as pw:
        for engine in a.browsers:
            rows += run_engine(pw, engine, token, a.out)
            rows += run_engine(pw, engine, host_token, a.out, HOST_SURFACES)
    (a.out / "measurements.json").write_text(json.dumps(rows, indent=1, ensure_ascii=False))
    f = findings(rows)
    (a.out / "findings.txt").write_text("\n".join(f) + "\n")
    print("\n".join(f) if f else "inga fynd")
    print(f"\n{len(rows)} mätningar, {len(f)} fynd → {a.out}")
    sys.exit(1 if f else 0)


if __name__ == "__main__":
    main()
