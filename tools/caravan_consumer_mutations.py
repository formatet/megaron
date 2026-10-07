#!/usr/bin/env python3
"""Physical consumer mutations; each arm uses tools/gotest.sh's fresh database.

Run only while other agents have frozen production edits/tests. Originals are
restored byte-for-byte even when a mutated arm or verification fails.
"""
import argparse
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
CASES = {
    "commit-before-schedule": (
        "server/api/handlers/province.go",
        'if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,',
        '_ = tx.Commit(r.Context()) // MUTATION: makes ship/debit persist before job\n\tif err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,',
        "TestCaravanTransfer_ScheduleFailureRollsBackShipAndGoods",
    ),
    "eta-plus-one": (
        "server/api/handlers/province.go",
        'tradeTravelTicks := journey.TravelTicks',
        'tradeTravelTicks := journey.TravelTicks + 1 // MUTATION: ETA differs from saved journey',
        "TestCaravanTransfer_BentPathSavedAndETAAnchored",
    ),
    "trade-commit-before-schedule": (
        "server/api/handlers/messenger.go",
        'if err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,',
        '_ = tx.Commit(r.Context()) // MUTATION: consumes offer before job\n\t\tif err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,',
        "TestCaravanAccept_ScheduleFailureLeavesOfferPending",
    ),
}


def run(test, log):
    with log.open("wb") as output:
        result = subprocess.run(
            [str(ROOT / "tools/gotest.sh"), "./api/handlers", "-run", "^" + test + "$", "-count=1"],
            cwd=ROOT, stdout=output, stderr=subprocess.STDOUT,
        )
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("/tmp/megaron-caravan-consumer-mutations"))
    parser.add_argument("--case", choices=CASES, action="append")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    for name in args.case or CASES:
        relative, old, new, test = CASES[name]
        path = ROOT / relative
        original = path.read_bytes()
        source = original.decode()
        if old not in source:
            raise RuntimeError(f"mutation anchor missing: {name}")
        try:
            path.write_text(source.replace(old, new, 1))
            red_log = args.output / (name + "-red.log")
            red = run(test, red_log)
            content = red_log.read_text()
            if red == 0 or "--- FAIL: " + test not in content or "[build failed]" in content:
                raise RuntimeError(f"mutation did not produce a test assertion failure: {red_log}")
        finally:
            path.write_bytes(original)
        green_log = args.output / (name + "-restored.log")
        if run(test, green_log) != 0:
            raise RuntimeError(f"restored verification failed: {green_log}")
        print(f"{name}: assertion red, byte-restored green", flush=True)


if __name__ == "__main__":
    main()
