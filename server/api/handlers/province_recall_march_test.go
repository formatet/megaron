package handlers

// E2E proof for the aggregate-army recall fix (megaron_todo.md
// "Aggregatarmé-recall", 2026-07-30): ProvinceHandler.RecallMarch used to aim
// its recall messenger blindly at the army's target province and never
// checked whether the courier could actually catch the army first — a silent
// miss the player only discovered as "nothing happened" (recall.go's own old
// comment: "If the army arrives and fights first, the recall simply
// misses."). This mirrors the individual-unit fix in
// unit_recall_interception_test.go, but for the marching_armies aggregate
// table, whose courier always departs from the army's own march origin (no
// free "nearest settlement" choice — RecallMarch requires provinceID ==
// march.origin_id).
//
// Fixture: a 100-spearman army marching 8 plains hexes (0,0)→(8,0), 0.75h/hex
// = 6h total. A CategoryCourier runner moves at 2× that rate from the SAME
// origin, so the geometry is symmetric: a courier can always catch the army
// (at worst at the destination) while MORE than half the march remains, and
// can never catch it once LESS than half remains (recall.go's own doc
// comment on InterceptAlongPath). The two tests sit clearly on either side of
// that midpoint — not at the boundary tie.
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type marchRecallFixture struct {
	worldID     uuid.UUID
	playerID    uuid.UUID
	accessToken string
	provinceID  uuid.UUID // origin/home province — also the URL param and the settlement location
	marchID     uuid.UUID
	departsAt   time.Time
	arrivesAt   time.Time
}

// setupMarchRecallWorld builds the 8-hex plains march fixture with the given
// elapsed time since departure (out of a fixed 6h total), and returns the
// live pool alongside the fixture + router so tests can inspect DB state and
// drive delivery.
func setupMarchRecallWorld(t *testing.T, elapsed time.Duration) (marchRecallFixture, *chi.Mux, *pgxpool.Pool) {
	t.Helper()
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	// See internal/combat/unit_arrival_colonize_test.go for why leftover
	// active test worlds must be archived first (one_active_world partial
	// unique index) — RecallMarch's current_world_tick() call needs an
	// active world, or the whole tx aborts with 25P02.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var f marchRecallFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "marchrecall-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	f.playerID = claims.PlayerID
	f.accessToken = accessToken

	// Straight 8-hex plains line (0,0)..(8,0) — map_tiles + a province per
	// hex (marching_armies.origin_id/target_id are NOT NULL province FKs, and
	// the intercept hex resolved at dispatch can land anywhere on this line).
	provinceIDs := make(map[int]uuid.UUID, 9)
	for q := 0; q <= 8; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains') ON CONFLICT DO NOTHING`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
		var pid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&pid); err != nil {
			t.Fatalf("create province(%d,0): %v", q, err)
		}
		provinceIDs[q] = pid
	}
	f.provinceID = provinceIDs[0]

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		f.worldID, f.provinceID, f.playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	const total = 6 * time.Hour // 8 hexes × 0.75h/hex, plains
	f.departsAt = time.Now().Add(-elapsed)
	f.arrivesAt = f.departsAt.Add(total)
	if err := pool.QueryRow(ctx,
		`INSERT INTO marching_armies
		     (world_id, origin_id, target_id, infantry, chariot, ship, elite_infantry,
		      war_galley, merchantman, intent, departs_at, arrives_at, resolved)
		 VALUES ($1,$2,$3, 100,0,0,0,0,0, 'attack', $4,$5, false) RETURNING id`,
		f.worldID, f.provinceID, provinceIDs[8], f.departsAt, f.arrivesAt,
	).Scan(&f.marchID); err != nil {
		t.Fatalf("create marching army: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, events.NewScheduler(pool, clk), clk, economy.SitosConfig{}, events.NewStore(pool), nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Delete("/worlds/{worldID}/provinces/{provinceID}/marches/{marchID}", ph.RecallMarch)
	return f, r, pool
}

// TestRecallMarch_InterceptsAlongPathNotBlindTarget proves AC1+AC3: with 1.5h
// elapsed of the 6h march (4.5h remaining — comfortably more than the 3h
// midpoint), a courier from the same origin must catch the army well before
// its destination — at hex (5,0) (courier travel is rounded to whole ticks,
// CourierTravelOnGraph, so this is not the naive continuous-hours halfway
// point), never at the blind-aim (8,0) the old code used. The response and
// stored messenger/event rows must all agree on that point, and delivering
// the event must turn the army around from THERE, not from its original far
// target.
func TestRecallMarch_InterceptsAlongPathNotBlindTarget(t *testing.T) {
	f, router, pool := setupMarchRecallWorld(t, 90*time.Minute)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodDelete,
		"/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/marches/"+f.marchID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("RecallMarch = %d %q, want 200 (a courier from the army's own origin, 75%% of the march "+
			"remaining, can catch it well before the destination)", rec.Code, rec.Body.String())
	}
	var resp struct {
		MessengerID          uuid.UUID `json:"messenger_id"`
		MessengerTravelTicks int       `json:"messenger_travel_ticks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}

	var destQ, destR int
	if err := pool.QueryRow(ctx, `SELECT dest_q, dest_r FROM messengers WHERE id = $1`, resp.MessengerID).
		Scan(&destQ, &destR); err != nil {
		t.Fatalf("read messenger dest: %v", err)
	}
	if destQ == 8 && destR == 0 {
		t.Fatalf("messenger aimed at the army's full destination (8,0) — the old bug this fix replaces: " +
			"blind aim at the target instead of an honest interception point")
	}
	if destQ != 5 || destR != 0 {
		t.Errorf("messenger aimed at (%d,%d), want (5,0) — the earliest hex on the remaining path a courier "+
			"from the army's own origin can reach before the army does (courier travel time is rounded to whole "+
			"ticks — CourierTravelOnGraph — so this is not the naive continuous-hours halfway point)", destQ, destR)
	}
	if resp.MessengerTravelTicks != 2 {
		t.Errorf("messenger_travel_ticks = %d, want 2 (1.875h courier travel to (5,0) at 0.375h/hex, rounded to whole ticks)",
			resp.MessengerTravelTicks)
	}

	// Deliver: the return march must depart from the INTERCEPT point (5,0),
	// not the army's original far destination (8,0) — the return leg is only
	// honest if it starts where the army was actually caught.
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'RecallArrival' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled RecallArrival: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	rh := messenger.NewRecallArrivalHandler(pool, events.NewScheduler(pool, clk), nil, clk)
	if err := rh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver recall arrival: %v", err)
	}

	var returnOriginQ, returnOriginR int
	if err := pool.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM marching_armies ma JOIN provinces p ON p.id = ma.origin_id
		 WHERE ma.world_id = $1 AND ma.intent = 'return'`,
		f.worldID,
	).Scan(&returnOriginQ, &returnOriginR); err != nil {
		t.Fatalf("read return march origin: %v", err)
	}
	if returnOriginQ != 5 || returnOriginR != 0 {
		t.Errorf("return march departs from (%d,%d), want (5,0) — the interception point, not the army's "+
			"original far destination", returnOriginQ, returnOriginR)
	}
}

// TestRecallMarch_UndeliverableFailsVisibly proves AC2 (fall a): with 5h
// elapsed of the 6h march (1h remaining — well under the 3h midpoint), no
// courier from the army's own origin can catch it anywhere on the remaining
// path, including the destination. Dispatch must reject with 422 now,
// visibly, instead of queuing a courier already certain to arrive too late
// and vanish into a silent log line — and must not create any messenger row
// or scheduled event (no partial commit).
func TestRecallMarch_UndeliverableFailsVisibly(t *testing.T) {
	f, router, pool := setupMarchRecallWorld(t, 5*time.Hour)
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodDelete,
		"/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/marches/"+f.marchID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("RecallMarch = %d %q, want 422 (with only 1h left of a 6h march, no courier from the army's "+
			"own origin can catch it even at the destination) — a doomed dispatch must fail now, visibly, "+
			"not silently later", rec.Code, rec.Body.String())
	}

	var messengerCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM messengers WHERE world_id = $1 AND kind = 'recall'`, f.worldID,
	).Scan(&messengerCount); err != nil {
		t.Fatalf("count recall messengers: %v", err)
	}
	if messengerCount != 0 {
		t.Errorf("recall messengers created = %d, want 0 — a rejected dispatch must not partially commit", messengerCount)
	}

	var resolved bool
	if err := pool.QueryRow(ctx, `SELECT resolved FROM marching_armies WHERE id = $1`, f.marchID).Scan(&resolved); err != nil {
		t.Fatalf("read march: %v", err)
	}
	if resolved {
		t.Errorf("march.resolved = true, want false — a rejected recall must not touch the outbound march")
	}
}
