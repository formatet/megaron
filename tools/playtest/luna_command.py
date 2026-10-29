#!/usr/bin/env python3
"""Audited Keryx entry point for one Luna player; no shell interpolation."""
import base64
import datetime
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.request

ALLOWED = set("status me founding join map city cities goods army unit notifications codex actions brief build cancel-build place staff idle roster recruit reinforce disband demolish rite allocate message messenger inbox outbox runners reply trade-offer trade-accept trade-decline trade-cancel transfer cargo diplomacy sightings storms places ticklog passage call-back recall redirect march settlements roads build-road route routes route-pause route-resume route-delete retreat-default occupation abandon rise leave help".split())
LIMIT = 24


def atomic_json(path, data):
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(data, indent=2) + "\n")
    tmp.chmod(0o600)
    tmp.replace(path)


def expires_at(token):
    try:
        part = token.split(".")[1]
        return float(json.loads(base64.urlsafe_b64decode(part + "=" * (-len(part) % 4)))["exp"])
    except (ValueError, KeyError, IndexError, TypeError):
        return 0


def refresh_config(path, *, force=False, opener=urllib.request.urlopen):
    cfg = json.loads(path.read_text())
    if not force and expires_at(cfg.get("token", "")) > time.time() + 900:
        return cfg
    if not cfg.get("refresh_token"):
        raise RuntimeError("authentication needs renewal; no refresh token")
    req = urllib.request.Request(cfg["server"].rstrip("/") + "/api/v1/auth/refresh",
        data=json.dumps({"refresh_token": cfg["refresh_token"]}).encode(),
        headers={"Content-Type": "application/json"})
    try:
        with opener(req, timeout=15) as resp:
            tokens = json.load(resp)
        cfg.update(token=tokens["access_token"], refresh_token=tokens["refresh_token"])
    except (OSError, ValueError, KeyError):
        raise RuntimeError("token renewal failed") from None
    atomic_json(path, cfg)
    return cfg


def validate_args(args):
    if args and args[0] in ("--json", "-j"):
        args = args[1:]
    if args in (["--help"], ["-h"]):
        return ["help"]
    if not args or args[0] not in ALLOWED:
        raise ValueError("Use an ordinary player Keryx verb; account/admin/shell commands are unavailable")
    if any(a.startswith(("--server", "--token", "--config")) for a in args):
        raise ValueError("Connection overrides are unavailable")
    if args[0] == "unit" and len(args) > 1 and args[1] not in ("list", "show", "cancel-training", "lay-up", "fit-out", "march", "stance", "load", "unload", "recall", "redirect", "reinforce", "build-road", "help", "--help"):
        raise ValueError("Unknown player unit command")
    return args


def main(profile):
    profile = Path(profile).resolve()
    round_dir = Path(os.environ.get("MEGARON_LUNA_ROUND", "")).resolve()
    if not round_dir.is_relative_to(profile / "rounds"):
        raise SystemExit("No active round for this player")
    audit = round_dir / "commands.jsonl"
    lock = round_dir / "commands.lock"
    with lock.open("a") as guard:
        fcntl.flock(guard, fcntl.LOCK_EX)
        rows = [json.loads(line) for line in audit.read_text().splitlines()] if audit.exists() else []
        if len(rows) >= LIMIT:
            raise SystemExit(f"Round command limit ({LIMIT}) reached; finish your report")
        args = sys.argv[1:]
        start = datetime.datetime.now().astimezone().isoformat()
        try:
            args = validate_args(args)
            refresh_config(profile / "config.json")
            env = dict(os.environ, POLEIA_CONFIG=str(profile / "config.json"))
            env.pop("POLEIA_TOKEN", None)
            env.pop("POLEIA_SERVER", None)
            proc = subprocess.run(["/home/tk/go/bin/poleia", *args], env=env,
                                  capture_output=True, text=True, timeout=40)
            if "HTTP 401" in proc.stderr:
                refresh_config(profile / "config.json", force=True)
                proc = subprocess.run(["/home/tk/go/bin/poleia", *args], env=env,
                                      capture_output=True, text=True, timeout=40)
            code, stdout, stderr = proc.returncode, proc.stdout, proc.stderr
        except (ValueError, RuntimeError) as exc:
            code, stdout, stderr = 2, "", str(exc) + "\n"
        except subprocess.TimeoutExpired:
            code, stdout, stderr = 124, "", "Keryx timed out; do not assume an order failed or retry it without checking state\n"
        row = dict(n=len(rows) + 1, start=start, args=args, code=code, stdout=stdout, stderr=stderr)
        with audit.open("a") as out:
            out.write(json.dumps(row, ensure_ascii=False) + "\n")
        audit.chmod(0o600)
    print(stdout, end="")
    print(stderr, end="", file=sys.stderr)
    raise SystemExit(code)
