package combat

// The four proofs for ForeignMarchSighted (megaron_plan_inkommande_marsch.md §4) —
// the notification that starts the clock in the asynchronicity gate.
//
// Before this handler, none of the 21 notification kinds internal/ emits said "an
// army is heading your way". The defender's first signal arrived AFTER the battle
// was resolved, so the gate's inequality (order travel + defender travel <
// attacker's remaining travel) had no start time and could not be tested at all.
//
// The load-bearing test is T3. The invariant is not "the defender is told" but
// "the defender is told exactly when the march becomes VISIBLE" — the gate sits on
// the marching unit's interpolated CURRENT position, never on its departure hex and
// never on its target hex. A test that only asserts a notification arrives would
// pass just as happily against a handler that notified on dispatch, which would leak
// every march in the world to its target. Only stepping the clock separates them.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sightingRecorder is a Broadcaster double that both records calls in memory AND
// performs the real INSERT into notifications, so notifySighting's own dedupe query
// reads real rows exactly as it does against *notify.Hub in production. combat may
// not import internal/notify (G1), so this reimplements only that one INSERT.
// Same pattern as unpaidWarningRecorder (upkeep_unpaid_warning_test.go).
type sightingRecorder struct {
	pool  *pgxpool.Pool
	calls []sightingCall
}

type sightingCall struct {
	playerID uuid.UUID
	kind     string
	level    int
	payload  map[string]any
}

func (r *sightingRecorder) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (r *sightingRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var m map[string]any
	_ = json.Unmarshal(bodyJSON, &m)
	r.calls = append(r.calls, sightingCall{playerID: playerID, kind: kind, level: level, payload: m})
	_, err = r.pool.Exec(ctx,
		`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		worldID, playerID, kind, level, bodyJSON,
	)
	return err
}

// The corridor the march walks. Deliberately ONE hex wide (r = 0 only): with a
// wider block several equal-cost A* routes exist between the endpoints and the
// interpolated position would depend on tie-breaking. A single-file corridor makes
// the path (0,0)…(30,0) unique, so hex index == elapsed hours and every assertion
// below is about the gate, never about pathfinding luck.
const (
	sightCorridorLen = 30 // hexes from origin to target, i.e. 31 tiles
	sightMarchHours  = 30 // one hex per hour, so hour N == hex N
	sightBystanderQ  = 15 // corridor hex the bystander's city overlooks
)

type marchSightFixture struct {
	worldID                       uuid.UUID
	attacker, defender, bystander uuid.UUID
	defenderCity                  uuid.UUID
	defenderCityName              string
	unitID                        uuid.UUID
	departsAt, arrivesAt          time.Time
	arriveTick                    int
	tick                          int
}

// newMarchSightFixture builds three Wanax on one corridor:
//
//	attacker  — owns the marching unit, starts at (0,0), no settlement
//	defender  — city Pylos at (30,0), the march's TARGET (level 2 case)
//	bystander — city Mycenae at (15,3), 3 hexes off the corridor’s midpoint, so it
//	            sees the march pass but is not its target (level 3 case)
func newMarchSightFixture(t *testing.T, pool *pgxpool.Pool, now time.Time) marchSightFixture {
	t.Helper()
	ctx := context.Background()
	const tick = 3000

	f := marchSightFixture{
		tick:             tick,
		defenderCityName: "Pylos",
		departsAt:        now,
		arrivesAt:        now.Add(sightMarchHours * time.Hour),
		arriveTick:       tick + sightMarchHours,
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"marchsight-"+uuid.New().String(), tick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
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
	f.attacker = mkPlayer("nikandros")
	f.defender = mkPlayer("thyestes")
	f.bystander = mkPlayer("alkinoos")

	// The corridor itself, plus the two flanking rows the bystander's city sits
	// beyond. Only the corridor is walkable — the flanks are sea, so A* cannot
	// leave the line.
	for q := -1; q <= sightCorridorLen+1; q++ {
		for r := -1; r <= 1; r++ {
			terrain := "deep_sea"
			if r == 0 {
				terrain = "plains"
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
				f.worldID, q, r, terrain,
			); err != nil {
				t.Fatalf("seed map tile (%d,%d): %v", q, r, err)
			}
		}
	}

	mkCity := func(owner uuid.UUID, q, r int, name string) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			f.worldID, q, r,
		).Scan(&prov); err != nil {
			t.Fatalf("create province for %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 1000) RETURNING id`,
			f.worldID, prov, name, owner,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.defenderCity = mkCity(f.defender, sightCorridorLen, 0, f.defenderCityName)
	// 3 hexes off the corridor: exactly a settlement eye's plains radius, so the
	// bystander sees the march only while it stands on hex sightBystanderQ.
	mkCity(f.bystander, sightBystanderQ, 3, "Mycenae")

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, stance,
		                    q, r, target_q, target_r, departs_at, arrives_at, depart_tick, arrive_tick)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 'aggressive',
		         0, 0, $3, 0, $4, $5, $6, $7)
		 RETURNING id`,
		f.worldID, f.attacker, sightCorridorLen, f.departsAt, f.arrivesAt, tick, f.arriveTick,
	).Scan(&f.unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}
	return f
}

// notifCount counts persisted ForeignMarchSighted rows for one player — the DB is
// the authority here, not the recorder's in-memory slice, because the dedupe query
// reads the DB.
func notifCount(t *testing.T, pool *pgxpool.Pool, worldID, playerID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications
		  WHERE world_id = $1 AND player_id = $2 AND kind = $3`,
		worldID, playerID, ForeignMarchSightedKind,
	).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return n
}

func newSightingHandler(pool *pgxpool.Pool, clk clock.Clock) (*MarchSightingHandler, *sightingRecorder) {
	rec := &sightingRecorder{pool: pool}
	return NewMarchSightingHandler(pool, events.NewScheduler(pool, clk), rec, clk), rec
}

// T1 — the base case plus the threat level. A march onto the defender's own city,
// seen by that city's eye, produces exactly one urgent notification carrying who,
// how big, where it is going and — the number the player actually plans in — which
// tick it lands.
func TestMarchSighting_NotifiesTargetedDefenderOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	f := newMarchSightFixture(t, pool, t0)

	// Hour 28 of 30: the march stands at (28,0), two hexes from Pylos — inside a
	// settlement eye's radius 3 on plains, with two hours of travel still to run.
	clk := clock.NewTestClock(t0.Add(28 * time.Hour))
	h, rec := newSightingHandler(pool, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := notifCount(t, pool, f.worldID, f.defender); got != 1 {
		t.Fatalf("defender notifications = %d, want 1", got)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer calls = %d, want 1", len(rec.calls))
	}
	c := rec.calls[0]
	if c.level != 2 {
		t.Errorf("level = %d, want 2 (urgent — the march targets the receiver's own city)", c.level)
	}
	if got := c.payload["threatens_name"]; got != f.defenderCityName {
		t.Errorf("threatens_name = %v, want %q", got, f.defenderCityName)
	}
	if got, want := c.payload["arrive_tick"], float64(f.arriveTick); got != want {
		t.Errorf("arrive_tick = %v, want %v", got, want)
	}
	if got, want := c.payload["size"], float64(100); got != want {
		t.Errorf("size = %v, want %v", got, want)
	}
	if got := c.payload["stance"]; got != "aggressive" {
		t.Errorf("stance = %v, want aggressive (live tier reveals in full — Timothy 2026-08-03)", got)
	}
	// q/r is the OBSERVATION hex, not the origin and not the target.
	if got, want := c.payload["q"], float64(28); got != want {
		t.Errorf("q = %v, want %v (interpolated position at sighting)", got, want)
	}
}

// T2 — dedupe, including that read status must not matter. The sweep runs every
// tick for as long as the army stays in view; if the dedupe key were unread-only
// (as notifyUpkeepUnpaid's deliberately is), reading the warning would immediately
// re-arm it and the player would be punished for looking.
func TestMarchSighting_DedupesRegardlessOfReadStatus(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	f := newMarchSightFixture(t, pool, t0)

	clk := clock.NewTestClock(t0.Add(28 * time.Hour))
	h, _ := newSightingHandler(pool, clk)

	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
			t.Fatalf("Handle #%d: %v", i+1, err)
		}
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 1 {
		t.Fatalf("after two sweeps: %d notifications, want 1", got)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE world_id = $1 AND player_id = $2`,
		f.worldID, f.defender,
	); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	// One more hour of travel — a new sweep, a new position, the same march.
	clk.Set(t0.Add(29 * time.Hour))
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick + 1}); err != nil {
		t.Fatalf("Handle after read: %v", err)
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 1 {
		t.Fatalf("after reading and a third sweep: %d notifications, want 1", got)
	}
}

// T3 — RED BEFORE. The gate sits on the march's current position, so the same
// fixture must produce nothing at hour 0 and exactly one notification at hour 28.
// Without stepping the clock this test proves nothing: a handler that notified the
// target on dispatch would pass every other assertion in this file.
func TestMarchSighting_SilentUntilTheMarchIsActuallyVisible(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	f := newMarchSightFixture(t, pool, t0)

	// Hour 0: the unit stands on its origin hex, 30 hexes from Pylos. The defender
	// is already the declared target — and must still be told nothing.
	clk := clock.NewTestClock(t0)
	h, _ := newSightingHandler(pool, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("Handle at hour 0: %v", err)
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 0 {
		t.Fatalf("hour 0: %d notifications, want 0 — the march is 30 hexes away and cannot be seen", got)
	}

	// Hour 26: still two hexes outside a settlement's radius 3.
	clk.Set(t0.Add(26 * time.Hour))
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick + 26}); err != nil {
		t.Fatalf("Handle at hour 26: %v", err)
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 0 {
		t.Fatalf("hour 26: %d notifications, want 0 — still outside the city's live tier", got)
	}

	// Hour 28: inside. Now, and only now, the clock starts.
	clk.Set(t0.Add(28 * time.Hour))
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick + 28}); err != nil {
		t.Fatalf("Handle at hour 28: %v", err)
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 1 {
		t.Fatalf("hour 28: %d notifications, want 1 — the march is two hexes from the city", got)
	}
}

// T4 — receiver selection and the level gradient in one sweep. At the corridor's
// midpoint the bystander's city sees the march go by while the defender's city
// (16 hexes ahead) cannot yet, and the attacker is never told about their own army.
func TestMarchSighting_BystanderGetsInfoLevelAndOwnerGetsNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	f := newMarchSightFixture(t, pool, t0)

	clk := clock.NewTestClock(t0.Add(sightBystanderQ * time.Hour))
	h, rec := newSightingHandler(pool, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick + sightBystanderQ}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if got := notifCount(t, pool, f.worldID, f.attacker); got != 0 {
		t.Fatalf("attacker notifications = %d, want 0 — never warn a Wanax about their own march", got)
	}
	if got := notifCount(t, pool, f.worldID, f.defender); got != 0 {
		t.Fatalf("defender notifications = %d, want 0 — the march is still 16 hexes from Pylos", got)
	}
	if got := notifCount(t, pool, f.worldID, f.bystander); got != 1 {
		t.Fatalf("bystander notifications = %d, want 1", got)
	}

	var c sightingCall
	for _, call := range rec.calls {
		if call.playerID == f.bystander {
			c = call
		}
	}
	if c.level != 3 {
		t.Errorf("bystander level = %d, want 3 (info — a march passing by is not an alarm)", c.level)
	}
	if _, ok := c.payload["threatens_name"]; ok {
		t.Errorf("bystander payload carries threatens_name = %v, want absent — the march targets someone else's city",
			c.payload["threatens_name"])
	}
	if got := c.payload["owner_id"]; got != f.attacker.String() {
		t.Errorf("owner_id = %v, want %v", got, f.attacker)
	}
}

// T5 — RED BEFORE the fix (megaron_plan_namnlager.md). The payload's "owner"
// field must carry the Wanax's PUBLIC name (wanax_name), never the login
// (username). Before the fix, loadMarches' query read only
// COALESCE(pl.username, empty-string-fallback) — the lone holdout among every other player-name
// read in the server (foreign_units.go, reports.go, god.go, world.go,
// kingdom.go, battle.go, chronicle.go all already read
// COALESCE(wanax_name, username)). A defender told "Mikharios is heading your
// way" could not `message --to Mikharios` back — the CLI's own "'X' is a Wanax
// name, try --to Y" hint matches only wanax_name, so username never triggers
// it and the player fell through to "no settlement named X in view".
func TestMarchSighting_PayloadCarriesWanaxNameNotUsername(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	f := newMarchSightFixture(t, pool, t0)

	// The attacker owns the marching unit, so it is the attacker's name that
	// ends up as payload["owner"]. Give the attacker a wanax_name that differs
	// from its (randomly generated) username — exactly the case migration 111
	// backfilled for every existing player and province.UniqueWanaxName assigns
	// for every new one. Suffixed with a fresh uuid: wanax_name is UNIQUE and
	// poleia_test accumulates players across runs, so a fixed literal collides.
	attackerWanaxName := "Mikharios-" + uuid.New().String()
	if _, err := pool.Exec(ctx,
		`UPDATE players SET wanax_name = $1 WHERE id = $2`,
		attackerWanaxName, f.attacker,
	); err != nil {
		t.Fatalf("set attacker wanax_name: %v", err)
	}

	// Same clock position as T1: hour 28, inside Pylos's live tier.
	clk := clock.NewTestClock(t0.Add(28 * time.Hour))
	h, rec := newSightingHandler(pool, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer calls = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].payload["owner"]; got != attackerWanaxName {
		t.Errorf("payload[owner] = %v, want %q (wanax_name) — the defender must be able to "+
			"`message --to` this exact string back", got, attackerWanaxName)
	}
}
