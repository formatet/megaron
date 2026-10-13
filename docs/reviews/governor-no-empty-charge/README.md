# Passive governor: stop spending silver without an action

## Summary and contract

On master `a7b8c858`, `PassiveGovernorTick` spent 10 silver whenever an AI-governed
settlement had loyalty ≤2 and settled silver ≥20. It created no gift, loyalty change,
or audit event, and a retry could spend again. Remove that unfinished action; existing
upkeep and loyalty handlers retain ownership of their respective effects.

Player truth: a colony's reserve cannot silently disappear to a governor action that
has no outcome. Invariant: this passive hook preserves the complete silver lazy tuple,
loyalty projection and event counts, including on replay.

Scope: `internal/ai/governor.go`, its DB regression, and this evidence package.
No migration, compensation for historical losses, new AI behaviour, gift semantics,
client verb or UI change. Introducing an automatic gift would require a separate
design contract and the existing atomic loyalty/event path, not a direct projection write.

Acceptance: low loyalty with sufficient/lazy silver stays unchanged; threshold 20,
poor/loyal/human/no-stock cases stay unchanged; three calls preserve every snapshot;
missing settlement still returns an error; the old implementation fails the named test.

This proves conservation for a reachable scheduled hook (`kharis.Handle` calls it for
AI-governed non-sunk settlements). It does not prove human completion of the main chain.
Classification: FUNCTION; no BILD/TEXT change or new four-surface verb.

## Baseline, mutation and checks

Real PostgreSQL 16, fresh migrated database per run, migration 179. The baseline
implementation physically restored in the source file fails three cases: ordinary
low-loyalty silver 100→90, threshold 20→10, and lazy stock whose `calc_tick` changes
40→50 while consuming 10 settled silver. Loyalty and event counts remain unchanged,
confirming that these debits buy no effect. See `mutation.log`.

The fix is restored byte-for-byte before running checks. The DB regression captures
the full `settlement_goods` row, loyalty and loyalty_points, loyalty_events and world
event counts before and after three calls for seven cases. The world/province/colony
are explicit test fixtures; this is a domain integration test, not a natural web playtest.

Reproduce from this worktree:

```sh
python3 docs/reviews/governor-no-empty-charge/mutate.py
GOCACHE=/tmp/megaron-governor-gocache tools/gotest.sh
env -i HOME=/home/tk PATH=/usr/local/bin:/usr/bin:/bin GOCACHE=/tmp/megaron-governor-vet-cache go vet ./... # from server/
```

Full suite output: `full-go.log`. Targeted restored output: `restored-green.log`.
Final results: full fresh Go suite exit 0 (world package 92.529 s), clean-environment
`go vet ./...` exit 0, and reproducible physical mutation RED → identical bytes →
fresh GREEN. All seven DB cases ran; no regression case was skipped.
No server build/live health provenance is claimed: no deployment is performed in this slice.

Initial rig attempts hit a read-only default Go cache and an overlapping test-container
name; neither is a game regression. Use a writable task cache and sequential DB runs.
The first compile also overlapped source restoration; all authoritative checks run on
stable source afterwards.

## Limits and handoff

Historical unexplained debits cannot be refunded safely from an outcome-free log.
AI governor development is separate scope. No balance tuning or new loyalty credit
is introduced. Claude owns review, merge and deploy under the current AGENTS.md roles.
