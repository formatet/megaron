#!/usr/bin/env python3
"""Run isolated, bounded Luna-low CLI rounds on the existing six-player world."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time
import urllib.request

from luna_command import atomic_json, refresh_config

HERE = Path(__file__).resolve().parent
ROOT = Path.home() / ".local/state/megaron-luna"
WORLD = "2178e795-1303-4cc3-865a-6386847d68d1"
SERVER = "http://10.0.1.92:8080"
PLAYERS = ("hephaistos", "talthybios", "pheidon", "gpt-damon", "gpt-kleio", "gpt-lykon")
stop = threading.Event()
children = {}


def current_tick():
    with urllib.request.urlopen(SERVER + "/api/v1/worlds/" + WORLD, timeout=10) as response:
        world = json.load(response)
    if world.get("id") != WORLD:
        raise RuntimeError("Configured world no longer exists; stop rather than rejoin")
    return int(world["current_tick"])


def setup():
    ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    for name in PLAYERS:
        profile = ROOT / name
        profile.mkdir(exist_ok=True, mode=0o700)
        (profile / "rounds").mkdir(exist_ok=True, mode=0o700)
        dest = profile / "config.json"
        if not dest.exists():
            cfg = json.loads((Path.home() / ".config/megaron-ai" / (name + ".json")).read_text())
            assert cfg["world_id"] == WORLD and cfg["server"].rstrip("/") == SERVER
            atomic_json(dest, cfg)
        # Only this player's working directory enters this player's context.
        kx = profile / "kx"
        kx.write_text("#!/usr/bin/env python3\nimport sys\nfrom pathlib import Path\nsys.path.insert(0," + repr(str(HERE)) + ")\nfrom luna_command import main\nmain(Path(__file__).resolve().parent)\n")
        kx.chmod(0o700)
        (profile / "AGENTS.md").write_text("Only play this Wanax via ./kx. Never inspect configs, other directories, databases or project source. Do not alter any files or run external tools. The final response is saved automatically.\n")


def run_round(name):
    with (ROOT / name / "round.lock").open("a") as guard:
        try:
            fcntl.flock(guard, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return {"player": name, "state": "already_running"}
        return _run_round(name)


def _run_round(name):
    profile = ROOT / name
    status_file = profile / "status.json"
    old = json.loads(status_file.read_text()) if status_file.exists() else {}
    number = int(old.get("round", 0)) + 1
    begin_tick = current_tick()
    round_dir = profile / "rounds" / f"{number:05d}"
    round_dir.mkdir(mode=0o700)
    base = dict(player=name, round=number, begin_tick=begin_tick, next_tick=begin_tick + 30,
                state="running", model="claude-haiku-5-5", started=time.time())
    atomic_json(status_file, base)
    try:
        refresh_config(profile / "config.json")
        note_path = profile / "last-summary.md"
        note = note_path.read_text()[-8000:] if note_path.exists() else "First independent round: inspect your own realm; do not assume others' locations."
        prompt = (HERE / "luna_prompt.md").read_text() + f"\nYour player account: {name}. Current game day at start: {begin_tick}.\nYour own previous state card:\n{note}\n"
        # Claude Haiku plays (Timothy 2026-10-21) so Codex quota goes to development.
        command = ["claude", "-p", "--model", "claude-haiku-5-5", "--permission-mode", "dontAsk",
                   "--allowedTools", "Bash(./kx:*)", "--strict-mcp-config", "--no-session-persistence",
                   "--output-format", "text"]
        env = dict(os.environ, MEGARON_LUNA_ROUND=str(round_dir))
        # No API-key billing or inherited player identity for this workload.
        for key in ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "CODEX_API_KEY", "POLEIA_CONFIG", "POLEIA_TOKEN", "POLEIA_SERVER"):
            env.pop(key, None)
        with (round_dir / "summary.md").open("w") as out, (round_dir / "stderr.txt").open("w") as err:
            proc = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=out, stderr=err,
                                    text=True, env=env, cwd=profile, start_new_session=True)
            children[name] = proc
            try:
                proc.communicate(prompt, timeout=480)
                code = proc.returncode
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGTERM)
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.wait()
                code = 124
            finally:
                children.pop(name, None)
        summary = round_dir / "summary.md"
        audit = round_dir / "commands.jsonl"
        rows = [json.loads(l) for l in audit.read_text().splitlines()] if audit.exists() else []
        if code == 0 and summary.exists() and any(row["code"] == 0 for row in rows):
            note_path.write_text(summary.read_text()[-8000:])
            base.update(state="waiting", exit_code=code, commands=len(rows),
                        successful_commands=sum(r["code"] == 0 for r in rows), failures=0)
        else:
            base.update(state="error", exit_code=code, commands=len(rows), failures=old.get("failures", 0) + 1)
        base["finished"] = time.time()
    except Exception as exc:
        base.update(state="error", error=type(exc).__name__, failures=old.get("failures", 0) + 1,
                    finished=time.time())
    # A completed round doesn't immediately trigger another after a slow call.
    try:
        base["next_tick"] = current_tick() + (120 if base.get("failures", 0) >= 3 else 30)
    except Exception:
        pass
    atomic_json(status_file, base)
    print(json.dumps(base), flush=True)
    return base


def run():
    with (ROOT / "fleet.lock").open("a") as guard:
        fcntl.flock(guard, fcntl.LOCK_EX | fcntl.LOCK_NB)
        signal.signal(signal.SIGTERM, lambda *_: stop.set())
        signal.signal(signal.SIGINT, lambda *_: stop.set())
        futures = {}
        with ThreadPoolExecutor(max_workers=2) as pool:
            while not stop.is_set():
                try:
                    tick = current_tick()
                    for name in PLAYERS:
                        if name in futures:
                            if not futures[name].done():
                                continue
                            futures.pop(name).result()
                        path = ROOT / name / "status.json"
                        state = json.loads(path.read_text()) if path.exists() else {}
                        if tick >= state.get("next_tick", -1):
                            futures[name] = pool.submit(run_round, name)
                except Exception as exc:
                    print(json.dumps({"fleet_error": type(exc).__name__}), flush=True)
                stop.wait(5)
            for proc in list(children.values()):
                try:
                    os.killpg(proc.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("setup", "run", "once", "status"))
    parser.add_argument("player", choices=PLAYERS, nargs="?")
    args = parser.parse_args()
    if args.command == "setup":
        setup()
    elif args.command == "run":
        run()
    elif args.command == "once":
        if not args.player:
            parser.error("once needs one player")
        run_round(args.player)
    else:
        for name in PLAYERS:
            path = ROOT / name / "status.json"
            print(path.read_text().strip() if path.exists() else json.dumps({"player": name, "state": "not_started"}))


if __name__ == "__main__":
    main()
