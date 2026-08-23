#!/usr/bin/env python3
"""Väckarklockan: en pollande ström av spelarhändelser, för att väcka
speldygnsagenter på HÄNDELSER i stället för på en fast femminutersklocka.

Bakgrund (2026-08-23-körningen): 57% av alla agentdrag var uttryckligen tomma
("väntoläge, inga nya fynd") eftersom kadensen var en klocka, inte händelser.
Idén: låt agenten reagera när något faktiskt hänt.

En Claude Code-subagent är turbaserad och kan inte hålla en öppen anslutning —
den kan alltså inte bli "puttad" till. Det här skriptet löser det genom att
vara en extern poller som skriver en rad på stdout per nyhet; en bevakare
(Claude Codes Monitor-verktyg) läser strömmen och väcker rätt agent.

Pollar via `keryx notifications --json`, INTE databasen — servern vet saker
spelaren inte får veta (fog of war). keryx ser exakt det spelaren ser, så
väckningen läcker aldrig mer info än spelaren redan har tillgång till.

Två radtyper på stdout, flushade direkt (en bevakare läser detta som en
ström, inte en fil som stängs):

  <wall> <player> EVENT kind=<kind> id=<id> level=<n> created=<ts> body=<json>
  <wall> <player> FLOOR elapsed=<n>s

EVENT = en avisering som inte fanns förra pollen (spelaren såg något nytt).
FLOOR = ingen avisering väckte spelaren på --floor sekunder — dags att agera
ändå (bygga vidare, placera gubbar, skicka en budbärare).

Första pollen per spelare emitterar ingenting — den bara registrerar
nuläget som baslinje, annars väcks alla agenter direkt på hela historiken.

    tools/speldygn_vackarklocka.py Talos=~/.config/poleia/talos.json
    tools/speldygn_vackarklocka.py Talos=~/.config/poleia/talos.json Daedalos=~/.config/poleia/daedalos.json \\
        --interval 60 --floor 1200
"""
import argparse, json, os, subprocess, sys, time
from datetime import datetime

KERYX = os.path.expanduser("~/go/bin/keryx")


def poll(config_path):
    """Kör `keryx notifications --json` mot en spelares config. Returnerar
    listan av notifikations-dicts (servern sorterar created_at DESC), eller
    None vid fel (loggat till stderr, aldrig till stdout — stdout är
    händelseströmmen och ska inte förorenas med felmeddelanden)."""
    env = dict(os.environ, POLEIA_CONFIG=config_path)
    try:
        out = subprocess.run([KERYX, "notifications", "--json"],
                              env=env, capture_output=True, text=True, timeout=30)
    except Exception as e:
        print(f"vackarklocka: keryx-anrop kraschade ({config_path}): {e}", file=sys.stderr)
        return None
    if out.returncode != 0:
        print(f"vackarklocka: keryx gav fel ({config_path}): {out.stderr.strip()}", file=sys.stderr)
        return None
    try:
        data = json.loads(out.stdout)
    except json.JSONDecodeError as e:
        print(f"vackarklocka: kunde inte tolka JSON ({config_path}): {e}", file=sys.stderr)
        return None
    return data.get("notifications", [])


def emit_event(player, n):
    wall = datetime.now().isoformat(timespec="seconds")
    body = json.dumps(n.get("body", {}), separators=(",", ":"))
    print(f"{wall} {player} EVENT kind={n.get('kind')} id={n.get('id')} "
          f"level={n.get('level')} created={n.get('created_at')} body={body}", flush=True)


def emit_floor(player, elapsed):
    wall = datetime.now().isoformat(timespec="seconds")
    print(f"{wall} {player} FLOOR elapsed={int(elapsed)}s", flush=True)


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                  formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("players", nargs="+", metavar="NAMN=CONFIGSOKVAG",
                     help="t.ex. Talos=~/.config/poleia/talos.json")
    ap.add_argument("--interval", type=int, default=60,
                     help="sekunder mellan varje poll (default 60)")
    ap.add_argument("--floor", type=int, default=1200,
                     help="sekunder utan avisering innan en FLOOR-rad ändå skrivs (default 1200 = 20 min)")
    args = ap.parse_args()

    configs = {}
    for spec in args.players:
        if "=" not in spec:
            sys.exit(f"vackarklocka: väntade NAMN=configsökväg, fick {spec!r}")
        name, path = spec.split("=", 1)
        configs[name] = os.path.expanduser(path)

    seen = {name: set() for name in configs}
    last_wake = {name: time.monotonic() for name in configs}
    baseline_done = {name: False for name in configs}

    print(f"vackarklocka: bevakar {', '.join(configs)} · interval={args.interval}s floor={args.floor}s",
          file=sys.stderr)

    while True:
        for name, config_path in configs.items():
            notifications = poll(config_path)
            if notifications is None:
                continue  # felet redan loggat, gå vidare till nästa spelare

            current_ids = {n["id"] for n in notifications if "id" in n}

            if not baseline_done[name]:
                seen[name] = current_ids
                baseline_done[name] = True
                last_wake[name] = time.monotonic()
                continue

            new = [n for n in notifications if n.get("id") not in seen[name]]
            seen[name] |= current_ids

            if new:
                # servern ordnar created_at DESC — emittera äldst-först
                for n in reversed(new):
                    emit_event(name, n)
                last_wake[name] = time.monotonic()
            else:
                elapsed = time.monotonic() - last_wake[name]
                if elapsed >= args.floor:
                    emit_floor(name, elapsed)
                    last_wake[name] = time.monotonic()

        time.sleep(args.interval)


if __name__ == "__main__":
    main()
