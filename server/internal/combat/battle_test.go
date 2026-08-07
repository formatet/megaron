package combat

// KR3 substrate tests (megaron_plan_kr3_stridssystem.md §9).
//
// RED BEFORE this slice: economy.NewSeededDice, combat.NewBattleTickHandler,
// the battles/battle_participants/battle_rounds tables (migration 114) and
// initiateOrJoinBattle did not exist — this file could not even compile
// against master before this slice's code landed (combat used fortune.go's
// direct math/rand, with no seed to reproduce a round from). Green after: the
// tests below.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sequenceDice returns Intn results from a fixed slice, in order (wrapping).
// Used only to pin battles.seed deterministically in these tests — production
// still draws from economy.NewWallDice() (UnitArrivalHandler's default).
type sequenceDice struct {
	ints []int
	i    int
}

func (d *sequenceDice) Intn(n int) int {
	v := d.ints[d.i%len(d.ints)]
	d.i++
	return v
}
func (d *sequenceDice) Float64() float64 { return 0.5 }

// battleFixture is a minimal world+two-player+capitals setup shared by this
// file's tests — mirrors unit_arrival_field_test.go's fixture but factored so
// multiple tests can each get their own isolated world.
type battleFixture struct {
	worldID  uuid.UUID
	attacker uuid.UUID
	defender uuid.UUID
}

func newBattleFixture(t *testing.T, pool *pgxpool.Pool) battleFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f battleFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	f.attacker = mkPlayer("attacker")
	f.defender = mkPlayer("defender")

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Attacker Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		f.worldID, attCapProv, f.attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	var defCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 5, 5, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&defCapProv); err != nil {
		t.Fatalf("create defender capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Defender Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		f.worldID, defCapProv, f.defender,
	); err != nil {
		t.Fatalf("create defender capital: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains')`,
		f.worldID,
	); err != nil {
		t.Fatalf("create target province: %v", err)
	}
	return f
}

func mkFieldDefender(t *testing.T, pool *pgxpool.Pool, f battleFixture, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'positioned', 1, 0) RETURNING id`,
		f.worldID, f.defender, size,
	).Scan(&id); err != nil {
		t.Fatalf("create field defender: %v", err)
	}
	return id
}

func mkFieldAttacker(t *testing.T, pool *pgxpool.Pool, f battleFixture, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'marching', 0, 0, 1, 0, 'sack') RETURNING id`,
		f.worldID, f.attacker, size,
	).Scan(&id); err != nil {
		t.Fatalf("create field attacker: %v", err)
	}
	return id
}

func newArrivalHandler(pool *pgxpool.Pool, dice economy.Dice) *UnitArrivalHandler {
	clk := clock.NewTestClock(time.Now())
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        &fakeBroadcaster{},
		scheduler:  events.NewScheduler(pool, clk),
		clk:        clk,
	}
	h.Dice = dice
	return h
}

// runFieldArrival drives resolve() for attackerUnitID exactly like a real
// ScheduledUnitArrival firing would, in its own transaction.
func runFieldArrival(t *testing.T, pool *pgxpool.Pool, h *UnitArrivalHandler, worldID, attackerUnitID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.resolve(ctx, tx, attackerUnitID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("resolve: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func loadBattleID(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM battles WHERE world_id = $1 AND q = $2 AND r = $3`, worldID, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("load battle at (%d,%d): %v", q, r, err)
	}
	return id
}

type battleRoundRow struct {
	tickIndex  int
	roundIndex int
	attacker   json.RawMessage
	defender   json.RawMessage
}

func loadBattleRounds(t *testing.T, pool *pgxpool.Pool, battleID uuid.UUID) []battleRoundRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT tick_index, round_index, attacker, defender FROM battle_rounds
		 WHERE battle_id = $1 ORDER BY tick_index, round_index`,
		battleID,
	)
	if err != nil {
		t.Fatalf("load battle_rounds: %v", err)
	}
	defer rows.Close()
	var out []battleRoundRow
	for rows.Next() {
		var r battleRoundRow
		if err := rows.Scan(&r.tickIndex, &r.roundIndex, &r.attacker, &r.defender); err != nil {
			t.Fatalf("scan battle_round: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// ── §9: initiation creates the persistent rows and first tick ──────────────

func TestInitiateBattle_CreatesBattleParticipantsAndFirstTick(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	defenderUnitID := mkFieldDefender(t, pool, f, 10)
	attackerUnitID := mkFieldAttacker(t, pool, f, 50)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{111, 222}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)

	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	var status string
	var startedTick, currentTick int
	var seed int64
	if err := pool.QueryRow(context.Background(),
		`SELECT status, started_tick, current_tick, seed FROM battles WHERE id = $1`, battleID,
	).Scan(&status, &startedTick, &currentTick, &seed); err != nil {
		t.Fatalf("read battle: %v", err)
	}
	if status != "active" {
		t.Errorf("battle status = %q, want active", status)
	}
	if seed == 0 {
		t.Errorf("battle seed = 0, want a drawn (non-zero) seed")
	}

	var pending int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events WHERE world_id = $1 AND event_type = 'BattleTick' AND processed_at IS NULL`,
		f.worldID,
	).Scan(&pending); err != nil {
		t.Fatalf("count pending BattleTick: %v", err)
	}
	if pending != 1 {
		t.Errorf("pending BattleTick events = %d, want 1", pending)
	}

	var eventCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'BattleStarted'`, battleID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count BattleStarted events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("BattleStarted event count = %d, want 1", eventCount)
	}

	// Not resolved yet — initiation only, no dice rolled.
	if len(loadBattleRounds(t, pool, battleID)) != 0 {
		t.Errorf("battle_rounds should be empty right after initiation, before any ScheduledBattleTick has run")
	}
	_ = defenderUnitID
}

// ── §9: reproducibility — same seed ⇒ bit-for-bit identical battle_rounds ──

func TestBattleReproducibility_SameSeedProducesIdenticalRounds(t *testing.T) {
	pool := testPool(t)

	runOnce := func() []battleRoundRow {
		f := newBattleFixture(t, pool)
		mkFieldDefender(t, pool, f, 40)
		attackerUnitID := mkFieldAttacker(t, pool, f, 45)

		// Fixed sequenceDice ⇒ fixed battles.seed ⇒ every round's dice stream
		// (derived from seed+tick+round, §3) is bit-for-bit identical whenever
		// the seed matches, regardless of process/run.
		h := newArrivalHandler(pool, &sequenceDice{ints: []int{999983, 999979}})
		runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
		battleID := loadBattleID(t, pool, f.worldID, 1, 0)

		battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil)
		runBattleToEnd(t, pool, battleH, f.worldID, battleID, 50)

		return loadBattleRounds(t, pool, battleID)
	}

	first := runOnce()
	second := runOnce()

	if len(first) == 0 {
		t.Fatalf("first run produced zero battle_rounds — fixture never resolved")
	}
	if len(first) != len(second) {
		t.Fatalf("round count differs: first=%d second=%d — same seed must reproduce the exact same battle length", len(first), len(second))
	}
	for i := range first {
		if first[i].tickIndex != second[i].tickIndex || first[i].roundIndex != second[i].roundIndex {
			t.Fatalf("round %d: (tick,round) differs: first=(%d,%d) second=(%d,%d)",
				i, first[i].tickIndex, first[i].roundIndex, second[i].tickIndex, second[i].roundIndex)
		}
		if string(first[i].attacker) != string(second[i].attacker) {
			t.Errorf("round %d: attacker result differs:\n  first:  %s\n  second: %s", i, first[i].attacker, second[i].attacker)
		}
		if string(first[i].defender) != string(second[i].defender) {
			t.Errorf("round %d: defender result differs:\n  first:  %s\n  second: %s", i, first[i].defender, second[i].defender)
		}
	}
}

// ── §9: a battle can legitimately span ≥2 battle-ticks ──────────────────────

func TestBattleTick_SpansMultipleTicksForAnEvenFight(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	// Evenly matched (not overwhelming either way) — some seeds settle in one
	// tick, but this fixed seed is pinned specifically because it does not
	// (verified empirically for this exact fixture/seed pair): the assertion
	// below is what actually matters (battle spans >1 tick AND still
	// terminates), not the seed's numerology.
	mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 200)

	rounds := loadBattleRounds(t, pool, battleID)
	maxTick := 0
	for _, r := range rounds {
		if r.tickIndex > maxTick {
			maxTick = r.tickIndex
		}
	}
	if maxTick < 2 {
		t.Errorf("battle's last tick_index = %d, want ≥ 2 (an evenly matched fight should span multiple battle-ticks) — adjust the fixture sizes/seed if this specific pair now settles in one tick", maxTick)
	}

	var status, reason string
	if err := pool.QueryRow(context.Background(),
		`SELECT status, termination_reason FROM battles WHERE id = $1`, battleID,
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read battle: %v", err)
	}
	if status != "ended" || reason != "annihilation" {
		t.Errorf("battle status/reason = %q/%q, want ended/annihilation", status, reason)
	}

	var eventCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'BattleEnded'`, battleID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count BattleEnded events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("BattleEnded event count = %d, want 1", eventCount)
	}
}

// ── §4: participation formula — pure, no DB ─────────────────────────────────

func TestParticipation_LoyaltyBaseAndClamp(t *testing.T) {
	cases := []struct {
		name        string
		loyalty     int
		kharis      float64
		fortify     bool
		wantAtLeast float64
		wantAtMost  float64
	}{
		{"loyalty 1, no kharis, no fortify", 1, 0, false, 0.65, 0.65},
		{"loyalty 2 baseline", 2, 0, false, 0.80, 0.80},
		{"loyalty 4, no kharis", 4, 0, false, 1.00, 1.00}, // already at MAX
		{"unknown loyalty falls back to baseline (2)", 99, 0, false, 0.80, 0.80},
		{"loyalty 4 + kharis + fortify clamps at MAX", 4, 1000, true, participationMax, participationMax},
		{"loyalty 1 + huge kharis still ≥ MIN, ≤ loyalty1+caps", 1, 1000, false, 0.65, 0.75},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := participation(c.loyalty, c.kharis, c.fortify)
			if got < participationMin || got > participationMax {
				t.Fatalf("participation() = %f, out of [%f,%f] bounds", got, participationMin, participationMax)
			}
			if got < c.wantAtLeast-1e-9 || got > c.wantAtMost+1e-9 {
				t.Errorf("participation(loyalty=%d, kharis=%f, fortify=%v) = %f, want in [%f,%f]",
					c.loyalty, c.kharis, c.fortify, got, c.wantAtLeast, c.wantAtMost)
			}
		})
	}
}

func TestParticipation_KharisNeverPenalises(t *testing.T) {
	// Kharis read in this file is always GREATEST(0, ...) — there is no
	// negative kharis in this system, so participation must be monotonically
	// non-decreasing in kharis, never a penalty.
	base := participation(2, 0, false)
	withKharis := participation(2, 50, false)
	if withKharis < base {
		t.Errorf("participation with kharis=50 (%f) < participation with kharis=0 (%f) — kharis must never penalise participation", withKharis, base)
	}
}

// ── §3: seeded dice reproducibility (pure, no DB) ───────────────────────────

func TestSeededDice_SameSeedSameSequence(t *testing.T) {
	seed := int64(1234567890)
	a := economy.NewSeededDice(seed)
	b := economy.NewSeededDice(seed)
	for i := 0; i < 100; i++ {
		if a.Intn(12) != b.Intn(12) {
			t.Fatalf("Intn(12) diverged at call %d for the same seed", i)
		}
	}
}

func TestBattleRoundSeed_DeterministicPerTriple(t *testing.T) {
	s1 := battleRoundSeed(42, 3, 1)
	s2 := battleRoundSeed(42, 3, 1)
	if s1 != s2 {
		t.Fatalf("battleRoundSeed(42,3,1) not deterministic: %d != %d", s1, s2)
	}
	if battleRoundSeed(42, 3, 2) == s1 {
		t.Errorf("battleRoundSeed must differ across round_index (got same value for round 1 and 2)")
	}
	if battleRoundSeed(42, 4, 1) == s1 {
		t.Errorf("battleRoundSeed must differ across tick_index (got same value for tick 3 and 4)")
	}
}
