package economy

// Blockad med enhet (megaron_plan_blockad_med_enhet.md, Timothy 2026-09-05):
// "om en fientlig wanax ställer en enhet i fortify eller sentry på en hex så
// slutar gubbarna att producera från den hexen." Röd-först coverage for the
// plan's own contract:
//   - a fientlig unit in fortify OR sentry on a worked ring hex zeroes it
//   - the settlement's own hex is exempt (that is a siege, a different
//     mechanic that already exists)
//   - the gubbe stays PLACED and resumes the instant the unit leaves
//   - an unblocked settlement's production is measured bit-for-bit unchanged
//   - "fientlig" excludes the settlement's own units, and a unit that is
//     merely marching through or has no stance at all does not block
//   - SyncHexBlockade fires exactly one dispatch per hex per transition

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// placeEnemyUnitWithStance is placeEnemyUnit's (siege_test.go) sibling for a
// caller-chosen stance — the blockade rule fires for BOTH fortify and
// sentry, unlike belägring's siege denial (economy/siege.go), which only
// ever reads sentry.
func placeEnemyUnitWithStance(t *testing.T, worldID, enemyOwner uuid.UUID, q, r int, stance string) uuid.UUID {
	t.Helper()
	pool := testPool(t)
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, stance)
		 VALUES ($1, $2, 'spearman', 'land', 40, 'positioned', $3, $4, $5) RETURNING id`,
		worldID, enemyOwner, q, r, stance,
	).Scan(&id); err != nil {
		t.Fatalf("place enemy unit (%s) at (%d,%d): %v", stance, q, r, err)
	}
	return id
}

func createEnemyOwner(t *testing.T) uuid.UUID {
	t.Helper()
	pool := testPool(t)
	enemyOwner := uuid.New()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO players (id, username, password_hash) VALUES ($1, $2, 'x')`,
		enemyOwner, "blockade-enemy-"+enemyOwner.String(),
	); err != nil {
		t.Fatalf("create enemy player: %v", err)
	}
	return enemyOwner
}

// TestRecomputeProduction_EnemyFortifyOrSentryOnWorkedHex_ZeroesItsYield is
// the plan's core contract, for BOTH stances the rule names.
func TestRecomputeProduction_EnemyFortifyOrSentryOnWorkedHex_ZeroesItsYield(t *testing.T) {
	for _, stance := range []string{"fortify", "sentry"} {
		t.Run(stance, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			const tick = 100

			settlementID, worldID, _ := seedSiegeFixture(t, tick, 100)
			seedTile(t, worldID, 1, 0, "hills")
			placeGubbe(t, settlementID, 1, 1, 0, GoodGrain)

			if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
				t.Fatalf("RecomputeProduction (baseline): %v", err)
			}
			baseline := readGrainRate(t, settlementID)
			if baseline <= NearjordGrainPerTick {
				t.Fatalf("baseline grain rate = %v, want > nearjord alone (%v) — the placed gubbe must contribute", baseline, NearjordGrainPerTick)
			}

			enemyOwner := createEnemyOwner(t)
			enemyUnitID := placeEnemyUnitWithStance(t, worldID, enemyOwner, 1, 0, stance)

			if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
				t.Fatalf("RecomputeProduction (blocked): %v", err)
			}
			blocked := readGrainRate(t, settlementID)
			if blocked != NearjordGrainPerTick {
				t.Errorf("grain rate while a %s enemy stands on the worked hex = %v, want exactly the flat nearjord trickle (%v) — the gubbe's whole contribution must be denied", stance, blocked, NearjordGrainPerTick)
			}
			// Belägring (S1+S2) is a SEPARATE mechanic — one unit standing
			// directly on one hex is not a siege.
			if besieged := readBesieged(t, settlementID); besieged {
				t.Errorf("besieged = true from a single fortify/sentry unit on one hex, want false — that is siege's job, not this rule's")
			}
			// Rule 3: the gubbe stays PLACED, not reassigned or deleted.
			var placementCount int
			if err := pool.QueryRow(ctx,
				`SELECT count(*) FROM settlement_placement WHERE settlement_id = $1 AND hex_q = 1 AND hex_r = 0`,
				settlementID,
			).Scan(&placementCount); err != nil {
				t.Fatalf("count placement: %v", err)
			}
			if placementCount != 1 {
				t.Errorf("placement row count on the blocked hex = %d, want 1 — the gubbe must stay placed, not be removed or moved", placementCount)
			}

			// ── Rule 3: the enemy withdraws, production resumes EXACTLY. ──
			removeUnit(t, enemyUnitID)
			if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
				t.Fatalf("RecomputeProduction (lifted): %v", err)
			}
			resumed := readGrainRate(t, settlementID)
			if resumed != baseline {
				t.Errorf("grain rate after the %s unit withdrew = %v, want exactly the pre-blockade baseline %v", stance, resumed, baseline)
			}
		})
	}
}

// TestRecomputeProduction_EnemyOnSettlementsOwnHex_DoesNotBlockCatchment
// covers plan rule 2: a unit standing on the settlement's OWN hex is a
// siege (an existing, different mechanic) — it must not trip this rule at
// all, since the settlement's own hex is not part of the catchment ring
// this rule filters.
func TestRecomputeProduction_EnemyOnSettlementsOwnHex_DoesNotBlockCatchment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, _ := seedSiegeFixture(t, tick, 100)
	seedTile(t, worldID, 1, 0, "hills")
	placeGubbe(t, settlementID, 1, 1, 0, GoodGrain)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (baseline): %v", err)
	}
	baseline := readGrainRate(t, settlementID)

	enemyOwner := createEnemyOwner(t)
	// Fortify unit standing ON THE SETTLEMENT'S OWN HEX (0,0) — not a ring hex.
	placeEnemyUnitWithStance(t, worldID, enemyOwner, 0, 0, "fortify")

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}
	after := readGrainRate(t, settlementID)
	if after != baseline {
		t.Errorf("grain rate with an enemy fortified on the settlement's OWN hex = %v, want unchanged from baseline %v — this rule exempts the own hex (that's a siege, not a blockade)", after, baseline)
	}
}

// TestRecomputeProduction_UnblockedSettlement_ProductionExactlyUnchanged is
// the measurement the slice brief names explicitly: a settlement with no
// FIENTLIG fortify/sentry unit on any of its worked hexes must produce a
// bit-for-bit identical rate whether or not this rule's code path runs — it
// runs every RecomputeProduction call now, so this proves it is a true
// no-op for the overwhelmingly common case. Three near-miss shapes that
// must NOT block, each checked against the exact same baseline:
//   - the settlement's OWN unit in fortify (not fientlig — it's yours)
//   - an enemy unit that is merely marching through (status != 'positioned')
//   - an enemy unit positioned with NO stance at all (the 2026-08-08
//     "man kan inte bara ställa sig där" rule siege already enforces)
func TestRecomputeProduction_UnblockedSettlement_ProductionExactlyUnchanged(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, 100)
	seedTile(t, worldID, 1, 0, "hills")
	placeGubbe(t, settlementID, 1, 1, 0, GoodGrain)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (baseline): %v", err)
	}
	baseline := readGrainRate(t, settlementID)
	if baseline <= NearjordGrainPerTick {
		t.Fatalf("baseline grain rate = %v, want > nearjord alone", baseline)
	}

	check := func(label string, mutate func() func()) {
		t.Helper()
		cleanup := mutate()
		defer cleanup()
		if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
			t.Fatalf("%s: RecomputeProduction: %v", label, err)
		}
		got := readGrainRate(t, settlementID)
		if got != baseline {
			t.Errorf("%s: grain rate = %v, want EXACTLY the unblocked baseline %v (diff %v)", label, got, baseline, got-baseline)
		}
	}

	check("own unit in fortify on the worked hex", func() func() {
		id := placeEnemyUnitWithStance(t, worldID, ownerID, 1, 0, "fortify")
		return func() { removeUnit(t, id) }
	})
	check("enemy unit marching through the worked hex", func() func() {
		enemyOwner := createEnemyOwner(t)
		id := func() uuid.UUID {
			var id uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
				 VALUES ($1, $2, 'spearman', 'land', 40, 'marching', $3, $4) RETURNING id`,
				worldID, enemyOwner, 1, 0,
			).Scan(&id); err != nil {
				t.Fatalf("place marching enemy: %v", err)
			}
			return id
		}()
		return func() { removeUnit(t, id) }
	})
	check("enemy unit positioned with no stance", func() func() {
		enemyOwner := createEnemyOwner(t)
		id := placeEnemyUnitNoStance(t, worldID, enemyOwner, 1, 0)
		return func() { removeUnit(t, id) }
	})
}

// ── SyncHexBlockade: the dispatch-transition half of the slice ────────────

type fakeBlockadeNotifier struct {
	calls []fakeBlockadeCall
}

type fakeBlockadeCall struct {
	worldID  uuid.UUID
	playerID uuid.UUID
	kind     string
	level    int
	payload  HexBlockadePayload
}

func (f *fakeBlockadeNotifier) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.calls = append(f.calls, fakeBlockadeCall{worldID, playerID, kind, level, payload.(HexBlockadePayload)})
	return nil
}

// TestSyncHexBlockade_FiresDispatchOnStartAndLift is the plan's rule 4: the
// owner gets a dispatch when the blockade hits AND when it lifts, naming
// the settlement, the hex (so "⌖ Take me there" resolves from q/r alone —
// megaron_plan_dispatches.md §3), and how many gubbar are affected.
func TestSyncHexBlockade_FiresDispatchOnStartAndLift(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, 100)
	seedTile(t, worldID, 1, 0, "hills")
	placeGubbe(t, settlementID, 1, 1, 0, GoodGrain)
	placeGubbe(t, settlementID, 2, 1, 0, "livestock") // second gubbe, SAME hex

	store := events.NewStore(pool)
	notifier := &fakeBlockadeNotifier{}

	// No enemy yet: nothing should transition.
	if err := SyncHexBlockade(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncHexBlockade (no enemy): %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired with no enemy present: %v", notifier.calls)
	}

	enemyOwner := createEnemyOwner(t)
	enemyUnitID := placeEnemyUnitWithStance(t, worldID, enemyOwner, 1, 0, "fortify")

	if err := SyncHexBlockade(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncHexBlockade (blocked): %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("dispatch calls after blockade starts = %d, want 1 (one dispatch per HEX, not per gubbe)", len(notifier.calls))
	}
	started := notifier.calls[0]
	if started.kind != EventHexBlockaded {
		t.Errorf("kind = %q, want %q", started.kind, EventHexBlockaded)
	}
	if started.playerID != ownerID {
		t.Errorf("notified player = %v, want the settlement owner %v", started.playerID, ownerID)
	}
	if started.payload.Q != 1 || started.payload.R != 0 {
		t.Errorf("payload q,r = (%d,%d), want (1,0) — required for the take-me-there button", started.payload.Q, started.payload.R)
	}
	if started.payload.Workers != 2 {
		t.Errorf("payload workers = %d, want 2 (both gubbar on the blocked hex)", started.payload.Workers)
	}
	if started.payload.Name == "" {
		t.Errorf("payload name is empty — every dispatch must name its subject (megaron_plan_dispatches.md §4)")
	}

	// Idempotency: calling again with NOTHING changed must fire nothing more.
	notifier.calls = nil
	if err := SyncHexBlockade(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncHexBlockade (steady state): %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired on an unchanged steady state: %v", notifier.calls)
	}

	// ── Lift: the enemy withdraws. ─────────────────────────────────────────
	removeUnit(t, enemyUnitID)
	if err := SyncHexBlockade(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncHexBlockade (lifted): %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("dispatch calls after the blockade lifts = %d, want 1", len(notifier.calls))
	}
	lifted := notifier.calls[0]
	if lifted.kind != EventHexUnblockaded {
		t.Errorf("kind = %q, want %q", lifted.kind, EventHexUnblockaded)
	}
	if lifted.payload.Workers != 2 {
		t.Errorf("payload workers = %d, want 2", lifted.payload.Workers)
	}

	// blockaded flag actually persisted false again on both rows.
	var stillBlockaded int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlement_placement WHERE settlement_id = $1 AND blockaded`,
		settlementID,
	).Scan(&stillBlockaded); err != nil {
		t.Fatalf("count blockaded rows: %v", err)
	}
	if stillBlockaded != 0 {
		t.Errorf("blockaded rows remaining after lift = %d, want 0", stillBlockaded)
	}

	// A real event was appended for both transitions (events store outcomes,
	// per CLAUDE.md's event-sourcing rule).
	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type IN ($2, $3)`,
		settlementID, EventHexBlockaded, EventHexUnblockaded,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 2 {
		t.Errorf("events appended = %d, want 2 (one per transition)", eventCount)
	}
}
