package handlers

// Belägring S3 — acceptanstest, kedjan end-to-end (megaron_plan_belagring.md
// §S3, CLAUDE.md "Kedjegrinden"). Kapitulationen är i drift sedan 2026-08-26
// (S3, `490f617`, mig 131) men har ALDRIG observerats köra hela vägen: de
// befintliga testerna (internal/kharis/siege_starvation_clock_test.go,
// internal/combat/siege_capitulation_test.go) bevisar var sin halva av
// kedjan isolerat — klockan fast-forwardas till tröskel-1 i det ena, och
// besieged/siege_starvation_ticks sätts för hand till tröskelvärdet i det
// andra. Ingen av dem driver klockan organiskt från noll, ETT tick i taget,
// och LÅTER samma tröskelpassage faktiskt trigga den riktiga
// ockupationshandlern.
//
// Varför den kedjan inte kan skrivas i vare sig internal/kharis eller
// internal/combat: klockan ägs av kharis.TickHandler.applySiegeStarvationClock
// (oexporterad), ockupationsövergången av combat.SiegeCapitulationHandler
// (samma paket kan inte importera kharis — CLAUDE.md §Package dependency
// order, G1: combat får använda "capabilities, economy, gossip, loyalty,
// province, tick, transport, unit (+clock, events)" — kharis står INTE i
// den listan, och kharis får i sin tur inte importera combat uppåt). Det
// enda exporterade fästet i kharis.TickHandler är Handle (den riktiga
// ScheduledKharisTick-ingången tick-workern anropar i drift) —
// api/handlers ligger ovanför båda och "may use all", vilket är exakt vad
// den här testfilen utnyttjar. Samma mönster som combat/battle_wall_test.go
// (newSiegeFixture) och kharis/siege_starvation_clock_test.go
// (siegeClockFixture): en egen, minimal fixtur i den här filen — ingen
// delad hjälpare rörs, en annan agent arbetar samtidigt i
// internal/combat och internal/kharis.
//
// Tröskeln läses ur economy.SiegeCapitulationTicks, aldrig hårdkodad
// (megaron_arbetssatt.md: "ett test som mäter en konstant mot sig själv
// bevisar intet").

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func siegeAcceptanceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// siegeAcceptanceFixture is a minimal besieged-capital fixture — one world,
// one defending capital, one besieging SENTRY unit holding the settlement's
// own hex (the only posture economy.LoadBesiegers/ReachableCatchmentHexes
// recognise as a blockader since the 2026-08-08 revision,
// megaron_plan_belagring.md §Beslutade delbeslut 1). S1/S2's own
// catchment-denial machinery (ReachableCatchmentHexes) is a separate,
// already-tested layer — this fixture starts from besieged=true directly by
// hand, exactly like the existing siege_capitulation_test.go /
// siege_starvation_clock_test.go fixtures do, since S3 (this slice) begins
// AFTER besieged is already true.
type siegeAcceptanceFixture struct {
	worldID       uuid.UUID
	defender      uuid.UUID
	besieger      uuid.UUID
	defSettlement uuid.UUID
	q, r          int
}

func newSiegeAcceptanceFixture(t *testing.T, pool *pgxpool.Pool) siegeAcceptanceFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var f siegeAcceptanceFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-siegeaccept-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(),
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	f.defender = mkPlayer("defender")
	f.besieger = mkPlayer("besieger")

	f.q, f.r = 5, 5
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		f.worldID, f.q, f.r,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, besieged, food_unmet_amount)
		 VALUES ($1, $2, 'Starveburg', 'achaean', $3, 'capital', true, 'active', 8000, false, 0) RETURNING id`,
		f.worldID, provinceID, f.defender,
	).Scan(&f.defSettlement); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, stance, sentry_q, sentry_r)
		 VALUES ($1, $2, 'spearman', 'land', 500, 'positioned', $3, $4, 'sentry', $3, $4)`,
		f.worldID, f.besieger, f.q, f.r,
	); err != nil {
		t.Fatalf("place besieging sentry unit: %v", err)
	}

	return f
}

func newSiegeAcceptanceTickHandler(pool *pgxpool.Pool) *kharis.TickHandler {
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	store := events.NewStore(pool)
	return kharis.NewTickHandler(pool, sched, store, nil)
}

func newSiegeAcceptanceCapitulationHandler(pool *pgxpool.Pool) *combat.SiegeCapitulationHandler {
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	store := events.NewStore(pool)
	return combat.NewSiegeCapitulationHandler(pool, store, sched, nil)
}

// driveOneDay runs the REAL production entry point for a game day —
// kharis.TickHandler.Handle (events.ScheduledKharisTick), exactly what the
// tick worker calls in drift — which internally calls applyDecay, which
// calls applySiegeStarvationClock LAST (tick.go line ~1097). Each call gets
// a fresh eventID, matching production (EnqueueTickRecurring rows a new
// event per due tick, never a replay of the same one) and the convention
// internal/kharis/grain_growth_test.go's advanceOneDay already uses for the
// same reason (G2 exactly-once claims are keyed on event_id+settlement_id).
//
// food_unmet_amount is set by the CALLER before each call — FoodTick (the
// handler that derives it in production) is a separate, already-tested
// layer (Utfodringsordningen) out of scope for this slice; forcing it by
// hand is the same shortcut siege_starvation_clock_test.go's own fixture
// takes.
func driveOneDay(t *testing.T, h *kharis.TickHandler, pool *pgxpool.Pool, worldID uuid.UUID, eventID int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1`, worldID); err != nil {
		t.Fatalf("advance tick: %v", err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{ID: eventID, WorldID: worldID, EventType: events.ScheduledKharisTick, DueTick: 0}); err != nil {
		t.Fatalf("kharis daily tick handle: %v", err)
	}
}

func readSiegeAcceptanceStarvationTicks(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) int {
	t.Helper()
	var ticks int
	if err := pool.QueryRow(context.Background(),
		`SELECT siege_starvation_ticks FROM settlements WHERE id = $1`, settlementID,
	).Scan(&ticks); err != nil {
		t.Fatalf("read siege_starvation_ticks: %v", err)
	}
	return ticks
}

type siegeAcceptanceSettlementSnapshot struct {
	state      string
	ownerID    uuid.UUID
	occupantID *uuid.UUID
	besieged   bool
	ticks      int
}

func readSiegeAcceptanceSettlement(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) siegeAcceptanceSettlementSnapshot {
	t.Helper()
	var snap siegeAcceptanceSettlementSnapshot
	if err := pool.QueryRow(context.Background(),
		`SELECT state, owner_id, occupant_id, besieged, siege_starvation_ticks FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&snap.state, &snap.ownerID, &snap.occupantID, &snap.besieged, &snap.ticks); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	return snap
}

func countSiegeAcceptanceScheduledCapitulations(t *testing.T, pool *pgxpool.Pool, worldID, settlementID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'SiegeCapitulation'
		   AND payload->>'settlement_id' = $2`,
		worldID, settlementID.String(),
	).Scan(&n); err != nil {
		t.Fatalf("count scheduled SiegeCapitulation events: %v", err)
	}
	return n
}

func siegeAcceptanceScheduledCapitulationPayload(t *testing.T, pool *pgxpool.Pool, worldID, settlementID uuid.UUID) json.RawMessage {
	t.Helper()
	var payload json.RawMessage
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'SiegeCapitulation'
		   AND payload->>'settlement_id' = $2
		 ORDER BY id LIMIT 1`,
		worldID, settlementID.String(),
	).Scan(&payload); err != nil {
		t.Fatalf("read scheduled SiegeCapitulation payload: %v", err)
	}
	return payload
}

// TestSiegeAcceptance_StarvationClockDrivesCapitulationEndToEnd is the core
// proof (megaron_plan_belagring.md §S3, uppdragets punkt 1-3): a besieged,
// starving capital's siege_starvation_ticks climbs by exactly one per REAL
// daily tick (kharis.TickHandler.Handle → applyDecay → applySiegeStarvationClock),
// nothing happens before economy.SiegeCapitulationTicks is reached, and
// crossing it — at the EXACT threshold, not one tick earlier or later —
// enqueues the real ScheduledSiegeCapitulation event which, when processed
// by the REAL combat.SiegeCapitulationHandler, flips the settlement to
// occupied under its besieger (owner_id unchanged — occupy, not annex) and
// clears besieged/siege_starvation_ticks. This exact chain has been live in
// production since 2026-08-26 (mig 131) without ever having been observed
// end-to-end.
func TestSiegeAcceptance_StarvationClockDrivesCapitulationEndToEnd(t *testing.T) {
	pool := siegeAcceptanceTestPool(t)
	ctx := context.Background()
	f := newSiegeAcceptanceFixture(t, pool)

	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET besieged = true, food_unmet_amount = 5 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("mark besieged+starving: %v", err)
	}

	h := newSiegeAcceptanceTickHandler(pool)
	threshold := economy.SiegeCapitulationTicks
	var eventID int64

	// Punkt 1+2: ett steg per tick, INGET förrän exakt tröskeln.
	for day := 1; day < threshold; day++ {
		eventID++
		driveOneDay(t, h, pool, f.worldID, eventID)

		got := readSiegeAcceptanceStarvationTicks(t, pool, f.defSettlement)
		if got != day {
			t.Fatalf("day %d: siege_starvation_ticks = %d, want %d (one step per tick)", day, got, day)
		}
		snap := readSiegeAcceptanceSettlement(t, pool, f.defSettlement)
		if snap.state != "active" || snap.occupantID != nil {
			t.Fatalf("day %d: settlement already changed (state=%q occupant=%v) — capitulation must not fire before threshold %d",
				day, snap.state, snap.occupantID, threshold)
		}
		if n := countSiegeAcceptanceScheduledCapitulations(t, pool, f.worldID, f.defSettlement); n != 0 {
			t.Fatalf("day %d: %d SiegeCapitulation event(s) already scheduled, want 0 before threshold %d", day, n, threshold)
		}
	}

	// Dag == tröskeln: passagen sker HÄR.
	eventID++
	driveOneDay(t, h, pool, f.worldID, eventID)

	if got := readSiegeAcceptanceStarvationTicks(t, pool, f.defSettlement); got != 0 {
		t.Errorf("at threshold day: siege_starvation_ticks = %d, want reset to 0 the instant it crosses", got)
	}
	if n := countSiegeAcceptanceScheduledCapitulations(t, pool, f.worldID, f.defSettlement); n != 1 {
		t.Fatalf("at threshold day: %d SiegeCapitulation event(s) scheduled, want exactly 1", n)
	}

	// Punkt 3: den RIKTIGA ockupationshandlern processar den riktiga
	// enqueuade händelsen — inte en handskriven ersättning.
	payload := siegeAcceptanceScheduledCapitulationPayload(t, pool, f.worldID, f.defSettlement)
	ch := newSiegeAcceptanceCapitulationHandler(pool)
	if err := ch.Handle(ctx, events.ScheduledEvent{
		WorldID: f.worldID, EventType: events.ScheduledSiegeCapitulation, Payload: payload,
	}); err != nil {
		t.Fatalf("handle siege capitulation: %v", err)
	}

	snap := readSiegeAcceptanceSettlement(t, pool, f.defSettlement)
	if snap.state != "occupied" {
		t.Errorf("state = %q, want \"occupied\"", snap.state)
	}
	if snap.ownerID != f.defender {
		t.Errorf("owner_id = %s, want UNCHANGED %s — capitulation is occupy, not annex", snap.ownerID, f.defender)
	}
	if snap.occupantID == nil || *snap.occupantID != f.besieger {
		t.Errorf("occupant_id = %v, want besieger %s", snap.occupantID, f.besieger)
	}
	if snap.besieged {
		t.Errorf("besieged still true, want cleared once occupied")
	}
	if snap.ticks != 0 {
		t.Errorf("siege_starvation_ticks = %d, want 0 after occupation", snap.ticks)
	}
}

// TestSiegeAcceptance_StarvationClockIsConsecutiveNotCumulative is
// uppdragets punkt 4 — kedjans mest lömska plats för en tyst bugg: klockan
// måste vara en KONSEKUTIV räknare, inte en paus-och-fortsätt-räknare. Maten
// kommer tillbaka en enda dag mitt i en pågående belägring → räknaren
// nollställs, och belägringen måste därefter gå igenom HELA tröskeln på
// nytt (inte bara de återstående dagarna) innan kapitulation kan inträffa —
// ett strängare bevis än att bara se att den nollställs en gång.
func TestSiegeAcceptance_StarvationClockIsConsecutiveNotCumulative(t *testing.T) {
	pool := siegeAcceptanceTestPool(t)
	f := newSiegeAcceptanceFixture(t, pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET besieged = true, food_unmet_amount = 5 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("mark besieged+starving: %v", err)
	}

	h := newSiegeAcceptanceTickHandler(pool)
	threshold := economy.SiegeCapitulationTicks
	var eventID int64

	const partial = 10 // långt under tröskeln, men en betydande andel
	for day := 1; day <= partial; day++ {
		eventID++
		driveOneDay(t, h, pool, f.worldID, eventID)
	}
	if got := readSiegeAcceptanceStarvationTicks(t, pool, f.defSettlement); got != partial {
		t.Fatalf("after %d starving days: siege_starvation_ticks = %d, want %d", partial, got, partial)
	}

	// Maten kommer tillbaka EN dag — svälten bryts.
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET food_unmet_amount = 0 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("cover food: %v", err)
	}
	eventID++
	driveOneDay(t, h, pool, f.worldID, eventID)
	if got := readSiegeAcceptanceStarvationTicks(t, pool, f.defSettlement); got != 0 {
		t.Fatalf("after food covered one day: siege_starvation_ticks = %d, want reset to 0 (consecutive, not paused)", got)
	}

	// Svälten återupptas — måste nu gå HELA tröskeln (threshold-1 dagar
	// utan att trigga), inte bara de återstående (threshold-partial).
	for day := 1; day < threshold; day++ {
		if _, err := pool.Exec(ctx,
			`UPDATE settlements SET food_unmet_amount = 5 WHERE id = $1`, f.defSettlement,
		); err != nil {
			t.Fatalf("resume starving day %d: %v", day, err)
		}
		eventID++
		driveOneDay(t, h, pool, f.worldID, eventID)

		got := readSiegeAcceptanceStarvationTicks(t, pool, f.defSettlement)
		if got != day {
			t.Fatalf("resumed day %d: siege_starvation_ticks = %d, want %d — a paused (non-consecutive) clock would already have capitulated by now if it kept the old %d-day head start",
				day, got, day, partial)
		}
	}
	if n := countSiegeAcceptanceScheduledCapitulations(t, pool, f.worldID, f.defSettlement); n != 0 {
		t.Fatalf("after resuming for threshold-1 fresh days: %d SiegeCapitulation event(s) scheduled, want 0 — the earlier %d-day head start must not have carried over", n, partial)
	}

	// Den fräscha tröskel-dagen korsar nu, för första gången.
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET food_unmet_amount = 5 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("final starving day: %v", err)
	}
	eventID++
	driveOneDay(t, h, pool, f.worldID, eventID)
	if n := countSiegeAcceptanceScheduledCapitulations(t, pool, f.worldID, f.defSettlement); n != 1 {
		t.Errorf("after the fresh threshold day: %d SiegeCapitulation event(s) scheduled, want exactly 1", n)
	}
}
