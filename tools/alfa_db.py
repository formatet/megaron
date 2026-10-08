"""Delad, skrivskyddad DB-åtkomst för alfa_funnel.py och alfa_economy.py.

Inga Python-beroenden: allt går genom `psql`.
  --dsn <url>            lokalt `psql <url>` (kräver psql i PATH)
  --ssh root@host        `ssh host su - postgres -c "psql -d poleia"` (databasen heter poleia)
  --psql-cmd "<kommando>" eget psql-kommando, t.ex. "docker exec -i alfa-pg psql -U poleia -d poleia"
SQL skickas på stdin (ingen quoting genom ssh/su). Varje fråga körs i sin egen
`BEGIN READ ONLY; ... COMMIT;` — servern vägrar då alla skrivningar, oavsett
vad frågan innehåller. Skripten skriver aldrig till spel-DB:n (megaron_plan_alfaanalys, Invarianter).
"""
import json
import shlex
import subprocess
import sys


class Db:
    def __init__(self, dsn=None, ssh=None, psql_cmd=None):
        if ssh:
            inner = "psql -d poleia -X -q -A -t -v ON_ERROR_STOP=1"
            self.cmd = ["ssh", ssh, 'su - postgres -c "%s"' % inner]
        elif psql_cmd or dsn:
            self.cmd = shlex.split(psql_cmd or "psql") + ["-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1"]
            if dsn:
                self.cmd += ["-d", dsn]
        else:
            raise SystemExit("ange --dsn, --ssh eller --psql-cmd")

    def raw(self, sql):
        """Kör SQL i read-only-transaktion, returnerar stdout. Fel -> SystemExit."""
        script = "BEGIN READ ONLY;\n%s;\nCOMMIT;\n" % sql.strip().rstrip(";")
        p = subprocess.run(self.cmd, input=script, capture_output=True, text=True)
        if p.returncode != 0 or "ERROR" in p.stderr:
            raise SystemExit("psql-fel: " + (p.stderr.strip() or "returkod %d" % p.returncode))
        return p.stdout

    def rows(self, sql):
        """sql ska vara en SELECT; resultatet returneras som lista av dict (via json_agg)."""
        wrapped = "SELECT coalesce(jsonb_agg(to_jsonb(t)), '[]'::jsonb) FROM (%s) t" % sql.strip().rstrip(";")
        out = self.raw(wrapped).strip()
        # jsonb::text är en enda rad; psql -q -t skriver bara värdet
        return json.loads(out) if out else []


def add_args(ap):
    ap.add_argument("--dsn", help="postgres-URL, t.ex. postgres://u:pw@host:5432/poleia (kräver psql lokalt)")
    ap.add_argument("--ssh", help="t.ex. root@10.0.1.92 — kör su - postgres -c psql -d poleia där")
    ap.add_argument("--psql-cmd", help='eget psql-kommando, t.ex. "docker exec -i alfa-pg psql -U poleia -d poleia"')
    ap.add_argument("--world", help="världens uuid (standard: den aktiva världen)")


def connect(args):
    return Db(dsn=args.dsn, ssh=args.ssh, psql_cmd=args.psql_cmd)


def pick_world(db, world=None):
    if world:
        w = db.rows("SELECT id, name, state, current_tick, last_tick_at, now() AS db_now, "
                    "created_at FROM worlds WHERE id = '%s'" % world.replace("'", ""))
    else:
        w = db.rows("SELECT id, name, state, current_tick, last_tick_at, now() AS db_now, created_at "
                    "FROM worlds ORDER BY (status = 'active') DESC, created_at DESC LIMIT 1")
    if not w:
        raise SystemExit("ingen värld funnen")
    return w[0]


def md_table(header, rows):
    out = ["| " + " | ".join(header) + " |", "|" + "|".join("---" for _ in header) + "|"]
    for r in rows:
        out.append("| " + " | ".join("" if c is None else str(c) for c in r) + " |")
    return "\n".join(out)
