#!/usr/bin/env python3
"""Copy new player reports into the vault, one Markdown file per month.

The server appends every report to REPORTS_DIR/reports.jsonl (outside the
database, survives a reseed). This script reads that file -- over SSH by
default -- and appends every report whose id is not already in the vault to
megaron_buggrapporter_YYYY-MM.md. Nothing already in the vault is rewritten,
so triage notes written under a report stay put.

Written for an LLM reader: one report = one section with fixed `key: value`
lines, so `grep -n "triage: open" megaron_buggrapporter_*.md` lists the
untriaged ones.

Runs daily from a systemd user timer on i9arch (tools/systemd/). Usage:
  tools/reports_to_vault.py                       # ssh source, real vault
  tools/reports_to_vault.py --source f.jsonl --vault /tmp/v   # local test
"""

import argparse
import glob
import json
import os
import re
import subprocess
import sys
from datetime import datetime
from zoneinfo import ZoneInfo

DEFAULT_SSH = "root@10.0.1.92"
DEFAULT_REMOTE = "/var/lib/poleia/reports/reports.jsonl"
DEFAULT_VAULT = os.path.expanduser("~/Dokument/myltavault")
TZ = ZoneInfo("Europe/Stockholm")
CONTEXT_CAP = 3000  # characters of compact context JSON kept per report

ID_LINE = re.compile(r"^- id: ([0-9a-f-]{36})$", re.M)

HEADER = """---
title: "Megaron — spelarrapporter {month}"
description: "Spelarnas bugg-/design-/förvirringsrapporter för {month}, kopierade dagligen ur serverns reports.jsonl av tools/reports_to_vault.py. Läses av Claude; triage skrivs in under varje rapport."
metadata:
  type: project
---
# Spelarrapporter {month}

**Format.** En rapport = en `##`-sektion, äldst först. Fasta rader: `id`, `triage`, `world`, `wanax`,
`where`, `context` (kompakt JSON från klienten), sedan spelarens egen text som citat.
**Triage:** byt `triage: open` mot `triage: YYYY-MM-DD → <vart det tog vägen>` (todo-rad, plan,
"avvisad: <skäl>", "dubblett av <id>"). Öppna rapporter: `grep -n "triage: open" megaron_buggrapporter_*.md`.
Skriptet lägger bara TILL rapporter (efter id) och skriver aldrig om befintlig text.
Rutin och kända mönster: [[temenos_buggrapporter]].
"""


def read_source(args):
    if args.source:
        with open(args.source, encoding="utf-8") as f:
            return f.read()
    res = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=15", args.ssh,
         f"cat {args.remote} 2>/dev/null || true"],
        capture_output=True, text=True, timeout=120,
    )
    if res.returncode != 0:
        sys.exit(f"ssh failed ({res.returncode}): {res.stderr.strip()}")
    return res.stdout


def known_ids(vault):
    ids = set()
    for path in glob.glob(os.path.join(vault, "megaron_buggrapporter_*.md")):
        with open(path, encoding="utf-8") as f:
            ids.update(ID_LINE.findall(f.read()))
    return ids


def render(rep):
    created = datetime.fromisoformat(rep["created_at"].replace("Z", "+00:00")).astimezone(TZ)
    who = rep.get("wanax") or rep.get("username") or "?"
    world = rep.get("world_name") or "?"
    where = []
    if rep.get("q") is not None and rep.get("r") is not None:
        where.append(f"hex {rep['q']},{rep['r']}")
    if rep.get("view"):
        where.append(f"view {rep['view']}")
    ctx = rep.get("context")
    ctx_s = json.dumps(ctx, ensure_ascii=False, separators=(",", ":")) if ctx else "–"
    if len(ctx_s) > CONTEXT_CAP:
        ctx_s = ctx_s[:CONTEXT_CAP] + f"…(+{len(ctx_s) - CONTEXT_CAP} tecken)"
    body = "\n".join("> " + line for line in rep.get("body", "").splitlines()) or "> (tom)"
    return (
        f"## {created:%Y-%m-%d %H:%M} · {rep.get('kind', '?')} · {who}\n"
        f"- id: {rep['id']}\n"
        f"- triage: open\n"
        f"- world: {world} ({rep.get('world_id', '?')[:8]}) · tick {rep.get('tick', '?')}\n"
        f"- wanax: {rep.get('wanax') or '–'} · user: {rep.get('username') or '–'}\n"
        f"- where: {' · '.join(where) or '–'}\n"
        f"- context: {ctx_s}\n"
        f"\n{body}\n\n"
    ), created


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--source", help="local reports.jsonl instead of ssh")
    ap.add_argument("--ssh", default=DEFAULT_SSH)
    ap.add_argument("--remote", default=DEFAULT_REMOTE)
    ap.add_argument("--vault", default=DEFAULT_VAULT)
    args = ap.parse_args()

    seen = known_ids(args.vault)
    new = []
    for n, line in enumerate(read_source(args).splitlines(), 1):
        if not line.strip():
            continue
        try:
            rep = json.loads(line)
        except json.JSONDecodeError as e:
            print(f"skipping line {n}: {e}", file=sys.stderr)
            continue
        if rep.get("id") and rep["id"] not in seen:
            seen.add(rep["id"])
            new.append(rep)

    new.sort(key=lambda r: r["created_at"])
    by_month = {}
    for rep in new:
        text, created = render(rep)
        by_month.setdefault(f"{created:%Y-%m}", []).append(text)

    for month, texts in sorted(by_month.items()):
        path = os.path.join(args.vault, f"megaron_buggrapporter_{month}.md")
        fresh = not os.path.exists(path)
        with open(path, "a", encoding="utf-8") as f:
            if fresh:
                f.write(HEADER.format(month=month) + "\n")
            f.writelines(texts)
        print(f"{path}: +{len(texts)}")
    if not new:
        print("inga nya rapporter")


if __name__ == "__main__":
    main()
