package transport

// R5 (megaron_plan_sjohandel_kraver_skepp.md): a naval transport with a bound
// ship rolls ONE of three outcomes when seized — captured, limped, sunk.
// Forced deterministically via an injected Dice, same seam pattern as
// economy.DeliveryHandler's trade-risk roll.

import (
	"context"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fixedDice always returns the same value — forces rollNavalSeizureOutcome
// down a specific branch (captured < 0.4 <= limped < 0.8 <= sunk).
type fixedDice struct{ v float64 }

func (d fixedDice) Float64() float64 { return d.v }

// navalSeizureFixture builds a victim caravan fixture (newFixture's land
// strip, reused for coordinates only — the seizure logic never checks the
// underlying terrain is actually sea, same as intercept_sea_leg_test.go's
// straight-line-fallback tests) plus a raider with a capital and a sentry
// watching the caravan's halfway hex, and a bound ship at the source.
type navalSeizureFixture struct {
	fixture
	raider        uuid.UUID
	raiderCapital uuid.UUID
	shipID        uuid.UUID
	transportID   uuid.UUID
	clk           clock.Clock
}

func newNavalSeizureFixture(t *testing.T, pool *pgxpool.Pool) navalSeizureFixture {
	t.Helper()
	ctx := context.Background()
	f := newFixture(t, pool)

	var raider, raiderProv, raiderCapital uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`,
		"raider-"+uuid.New().String()).Scan(&raider); err != nil {
		t.Fatalf("create raider: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 9, 9, 'plains') RETURNING id`,
		f.worldID).Scan(&raiderProv); err != nil {
		t.Fatalf("create raider province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Raidertown','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		f.worldID, raiderProv, raider).Scan(&raiderCapital); err != nil {
		t.Fatalf("create raider capital: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',80,0,'positioned','sentry',1,0,1,0)`,
		f.worldID, raider); err != nil {
		t.Fatalf("create sentry: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 10, 'freighting', $3) RETURNING id`,
		f.worldID, f.owner, f.sourceID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create ship: %v", err)
	}

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	departs := clk.Now().Add(-1 * time.Hour)
	arrives := clk.Now().Add(1 * time.Hour) // halfway now -> hex (1,0), same as the sentry

	var transportID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable, ship_unit_id)
		 VALUES ($1,$2,'transfer',$3,$4,'naval',0,0,3,0,$5,$6,1,'in_transit',true,$7)
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID, departs, arrives, shipID,
	).Scan(&transportID); err != nil {
		t.Fatalf("create naval transport: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,'silver',100)`, transportID,
	); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	return navalSeizureFixture{fixture: f, raider: raider, raiderCapital: raiderCapital, shipID: shipID, transportID: transportID, clk: clk}
}

func TestInterceptScan_NavalCaptured_TakesCargoAndEnqueuesShipCapture(t *testing.T) {
	pool := testPool(t)
	nf := newNavalSeizureFixture(t, pool)
	ctx := context.Background()

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, nf.clk), events.NewStore(pool), nil, nf.clk)
	h.Dice = fixedDice{0.1} // < 0.4 -> captured
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: nf.worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, nf.transportID).Scan(&status); err != nil {
		t.Fatalf("read transport status: %v", err)
	}
	if status != "intercepted" {
		t.Errorf("transport status = %q, want intercepted", status)
	}

	var loot float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		nf.raiderCapital,
	).Scan(&loot)
	if loot != 100 {
		t.Errorf("raider capital silver = %v, want 100 (captured: full cargo credited)", loot)
	}

	// The ship's ownership/march change is combat's job (G1) — this package
	// only proves the crossing event was enqueued, with the right payload.
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events WHERE world_id = $1 AND event_type = 'NavalSeizureOutcome' ORDER BY id DESC LIMIT 1`,
		nf.worldID,
	).Scan(&payload); err != nil {
		t.Fatalf("no NavalSeizureOutcome scheduled: %v", err)
	}
	got := string(payload)
	if !strings.Contains(got, nf.shipID.String()) || !strings.Contains(got, nf.raider.String()) || !strings.Contains(got, `"captured"`) {
		t.Errorf("NavalSeizureOutcome payload = %s, want ship/captor/outcome=captured", got)
	}

	// The ship's own row is untouched here — the (separate, combat-side)
	// handler does the owner change + march.
	var shipStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, nf.shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status right after seize = %q, want still freighting (owner change happens in the async handler)", shipStatus)
	}
}

func TestInterceptScan_NavalLimped_HalfCargoLostShipSailsHome(t *testing.T) {
	pool := testPool(t)
	nf := newNavalSeizureFixture(t, pool)
	ctx := context.Background()

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, nf.clk), events.NewStore(pool), nil, nf.clk)
	h.Dice = fixedDice{0.5} // 0.4 <= x < 0.8 -> limped
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: nf.worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	// Nothing credited to the raider — the lost half simply doesn't exist.
	var loot float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		nf.raiderCapital,
	).Scan(&loot)
	if loot != 0 {
		t.Errorf("raider capital silver = %v, want 0 (limped: nothing looted)", loot)
	}

	// A new damaged_return transport carries the surviving half, still bound
	// to the same ship.
	var kind string
	var qty float64
	var shipUnitID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT t.kind, tg.quantity, t.ship_unit_id
		 FROM transports t JOIN transport_goods tg ON tg.transport_id = t.id
		 WHERE t.world_id = $1 AND t.kind = 'damaged_return'
		 ORDER BY t.created_at DESC LIMIT 1`,
		nf.worldID,
	).Scan(&kind, &qty, &shipUnitID); err != nil {
		t.Fatalf("no damaged_return leg found: %v", err)
	}
	if qty != 50 {
		t.Errorf("damaged_return manifest silver = %v, want 50 (half of 100)", qty)
	}
	if shipUnitID == nil || *shipUnitID != nf.shipID {
		t.Errorf("damaged_return ship_unit_id = %v, want %s", shipUnitID, nf.shipID)
	}

	// The ship itself is untouched (still freighting) until that leg lands.
	var shipStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, nf.shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status right after limped seizure = %q, want still freighting", shipStatus)
	}
}

func TestInterceptScan_NavalSunk_CargoAndShipBothLost(t *testing.T) {
	pool := testPool(t)
	nf := newNavalSeizureFixture(t, pool)
	ctx := context.Background()

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, nf.clk), events.NewStore(pool), nil, nf.clk)
	h.Dice = fixedDice{0.9} // >= 0.8 -> sunk
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: nf.worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	var loot float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		nf.raiderCapital,
	).Scan(&loot)
	if loot != 0 {
		t.Errorf("raider capital silver = %v, want 0 (sunk: nothing looted)", loot)
	}

	var shipStatus string
	var size, crew int
	if err := pool.QueryRow(ctx, `SELECT status, size, crew FROM units WHERE id = $1`, nf.shipID).
		Scan(&shipStatus, &size, &crew); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "disbanded" {
		t.Errorf("ship status = %q, want disbanded", shipStatus)
	}
	if size != 0 || crew != 0 {
		t.Errorf("ship size/crew = %d/%d, want 0/0", size, crew)
	}
}
