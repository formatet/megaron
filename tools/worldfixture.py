#!/usr/bin/env python3
"""Dumpar en hel världs klientpayload till en riggfixtur.

    python3 tools/worldfixture.py full                       # omniscient (utan auth)
    python3 tools/worldfixture.py fow --user Sthenelos       # en spelares FOW-vy

Skriver `web/static/fixtures/world-<etikett>.json` — exakt de payloads
`loadMap()` hämtar, ingenting härlett. Riggen `showcase-world.html` läser filen
och fyller `State` med den, så helvyn som bedöms är den riktiga renderaren på
riktig världsdata i riktig världsstorlek.

Två fixturer behövs och mäter olika saker:

  full  — hämtad UTAN token. `/map` svarar då `tier="live"` för varje tile
          (world.go: `case !authenticated`), alltså hela geografin utan dimma.
          Det är prestandans värsta fall (varje tile har textur) och den enda
          vy där kartans komposition som EN geografi går att bedöma.
  fow   — hämtad som en spelare som spelat. Live/remembered/fog i verkliga
          proportioner: det spelaren faktiskt ser, med dimmans vidder och
          etiketterna glesa. Prestandans bästa fall.

Fixturen sorteras på (q, r). Serverns `/map`-query saknar ORDER BY, så
arrayordningen varierar mellan hämtningar; utan sorteringen här skulle två
dumpar av samma värld ge olika pixlar i lövverkspasset (som ritas i
arrayordning) och riggen vore inte deterministisk.
"""
import argparse
import json
import pathlib
import sys
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT_DIR = ROOT / "web" / "static" / "fixtures"

ENDPOINTS = {
    "tiles": "map",
    "provinces": "provinces",
    "units": "units",
    "rural": "rural-projections",
    "marches": "marches",
    "messengers": "messengers",
    "trades": "trades",
}


def get(url, token=None):
    req = urllib.request.Request(url)
    if token:
        req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def login(server, user, password):
    body = json.dumps({"username_or_email": user, "password": password}).encode()
    req = urllib.request.Request(server + "/api/v1/auth/login", data=body,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as r:
        d = json.load(r)
    return d["access_token"], d.get("player_id")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("label")
    ap.add_argument("--server", default="http://10.0.1.92:8080")
    ap.add_argument("--world", default="c4384909-94a7-476e-a3ae-7ae3e768a32d")
    ap.add_argument("--user")
    ap.add_argument("--password", default="playtest2026")
    args = ap.parse_args()

    token = player_id = None
    if args.user:
        token, player_id = login(args.server, args.user, args.password)

    base = f"{args.server}/api/v1/worlds/{args.world}"
    out = {"world_id": args.world, "player_id": player_id, "user": args.user}
    for key, path in ENDPOINTS.items():
        try:
            d = get(f"{base}/{path}", token)
        except urllib.error.HTTPError as e:
            # Utan token svarar bara /map och /provinces. Att sakna enheter är
            # inte ett fel i omniscient-fixturen — det är vad "ingen spelare"
            # betyder. En tyst tom lista hade däremot dolt att fixturen är
            # ofullständig, så skälet skrivs in i filen.
            out.setdefault("missing", {})[key] = e.code
            d = {"units": []} if key == "units" else []
        out[key] = d.get("units", []) if key == "units" and isinstance(d, dict) else d

    out["tiles"].sort(key=lambda t: (t["q"], t["r"]))
    out["provinces"].sort(key=lambda p: (p["q"], p["r"]))

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    dst = OUT_DIR / f"world-{args.label}.json"
    dst.write_text(json.dumps(out, separators=(",", ":"), sort_keys=True))

    tiers = {}
    for t in out["tiles"]:
        tiers[t.get("tier")] = tiers.get(t.get("tier"), 0) + 1
    print(f"{dst}  {dst.stat().st_size // 1024} kB  "
          f"tiles={len(out['tiles'])} {tiers}  provinces={len(out['provinces'])}  "
          f"units={len(out['units'])}  missing={out.get('missing', {})}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
