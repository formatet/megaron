#!/usr/bin/env python3
"""Luminans per terräng, mätt på FÄRDIGRENDERAD yta i terrängriggen.

    python3 tools/terrainstats.py                 # forest-fixturen
    python3 tools/terrainstats.py fixture=relief  # valfri query till riggen

Skriver en tabell med medelluminans (L) och spridning (sd) per terräng, samt
varje par av terränger som ligger närmare än 12 L isär.

VARFÖR DET HÄR FINNS. `megaron_terrangrendering` princip 17: en tonändring på
en stor yta kan nolla ut en kontrast någon annanstans — slättens nya ton lämnade
kustlinjen med 0,2 i valörkontrast, och ögat missade det helt eftersom
hue-skillnaden dolde valörbristen. Regeln är därför att hela tabellen vägs om
efter varje tonändring.

Att läsa `TERRAIN_BASE` räcker inte längre. När en terräng bär egen textur är
det inte basfärgen ögat läser utan den ritade ytans medelvärde — bergens skree
täcker hexen helt, så deras basfärg syns numera bara genom klippkantens
antialiasing. Den här mätaren läser pixlarna som faktiskt hamnade på skärmen.

TVÅ KOLUMNER, OCH PARNINGEN GÅR PÅ DEN ANDRA. `L` är hexens medelvärde och
blandar in objekten; `mark` är p75 och är den ton marken faktiskt har. För en
terräng som bär objekt skiljer de sig kraftigt, och det är `mark` som ska
jämföras mot grannen — se kommentaren vid parningen nedan för fallet som
avslöjade det (olivlunden, 2026-07-28).

Vad den INTE mäter (princip 6 gäller): medelluminans är inte mänsklig
diskriminerbarhet. Två terränger med samma medelvärde kan vara omöjliga att
förväxla om den ena är platt och den andra modellerad — spridningen i `sd`
antyder det, men bara ögat vid 1:1 avgör. Använd siffran för att hitta
misstänkta par, inte för att godkänna eller underkänna dem.

Andra kända avgränsningen: provcirkeln tar ALLT som ligger i hexen, inklusive
catchment-tint, väg, stad och fyndighetsmarkör. Det höjer `sd` för terränger
som råkar ligga under stadens catchment (kullarna i reliefixturen mätte sd 22
redan på orörd master, där marken var platt färg plus tre ellipser). Siffrorna
är därför jämförbara FÖRE/EFTER på samma fixtur, men `sd` är inte ett rent mått
på terrängens egen textur.
"""
import math
import os
import sys
from collections import defaultdict

from PIL import Image
from playwright.sync_api import sync_playwright

HOST = os.environ.get("SHOT_HOST", "http://localhost:8099")
NEAR = 12.0  # princip 17:s tröskel för "misstänkt nära"


def luminance(px):
    return 0.299 * px[0] + 0.587 * px[1] + 0.114 * px[2]


def sample(img, cx, cy, rin):
    """Pixlarna innanför hexens inradie, med marginal för kantens antialiasing."""
    out = []
    rad = int(rin * 0.72)
    for dy in range(-rad, rad + 1, 2):
        for dx in range(-rad, rad + 1, 2):
            if dx * dx + dy * dy > rad * rad:
                continue
            x, y = int(cx + dx), int(cy + dy)
            if 0 <= x < img.width and 0 <= y < img.height:
                out.append(luminance(img.getpixel((x, y))))
    return out


def main(query=""):
    url = f"{HOST}/static/showcase-forest.html" + (f"?{query}" if query else "")
    shot = "/tmp/terrainstats.png"
    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page(viewport={"width": 940, "height": 820}, device_scale_factor=1)
        page.goto(url, wait_until="networkidle")
        page.wait_for_function("window.SHOWCASE && window.SHOWCASE.ready", timeout=5000)
        tiles = page.evaluate("window.SHOWCASE.tiles()")
        page.locator("#map-root").screenshot(path=shot)
        b.close()

    img = Image.open(shot).convert("RGB")
    vals = defaultdict(list)
    for t in tiles:
        if t["terrain"] == "fog":
            continue
        vals[t["terrain"]].extend(sample(img, t["sx"], t["sy"], t["rin"]))

    stats = {}
    for terrain, xs in vals.items():
        if not xs:
            continue
        mean = sum(xs) / len(xs)
        sd = math.sqrt(sum((x - mean) ** 2 for x in xs) / len(xs))
        xs.sort()
        ground = xs[min(len(xs) - 1, int(len(xs) * 0.75))]
        stats[terrain] = (mean, sd, len(xs), ground)

    print(f"{'terräng':22} {'L':>7} {'mark':>7} {'sd':>7} {'px':>7}")
    for terrain, (mean, sd, n, g) in sorted(stats.items(), key=lambda kv: -kv[1][3]):
        print(f"{terrain:22} {mean:7.1f} {g:7.1f} {sd:7.1f} {n:7d}")

    # PARNING SKER PÅ `mark`, INTE PÅ `L`. Hexmedelvärdet blandar in objekten,
    # och för varje terräng som bär sådana ljuger det: olivlunden mätte L 133,2
    # mot slättens 131,7 och rapporterades som en kollision på 1,4 — men det
    # jämförde lund-MED-TRÄD mot ren slätt. Markerna ligger 20 L isär (154,0 mot
    # 133,7) och kolliderar inte alls. Kalkstenen har samma spann (p25 105 →
    # p90 148). Ett falskt par är dyrare än ett missat: det skickar en hel slice
    # på att laga något som inte är trasigt.
    #
    # p75 är markens ton därför att objekten är MÖRKA mot sin mark i det här
    # bildspråket (kronor, skree, siluetter — princip 11). Vänds det någon gång,
    # vänds den här percentilen med det.
    print(f"\nPar närmare än {NEAR:.0f} L i MARKTON (princip 17 — misstänkta, inte dömda):")
    keys = list(stats)
    found = False
    for i in range(len(keys)):
        for j in range(i + 1, len(keys)):
            d = abs(stats[keys[i]][3] - stats[keys[j]][3])
            if d < NEAR:
                found = True
                print(f"  {d:5.1f}  {keys[i]} ↔ {keys[j]}"
                      f"   (sd {stats[keys[i]][1]:.0f} / {stats[keys[j]][1]:.0f})")
    if not found:
        print("  inga")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else ""))
