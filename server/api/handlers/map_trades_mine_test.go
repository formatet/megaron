package handlers

// DB integration tests for the "last i rörelse" slice (web/last-i-rorelse,
// 2026-08): MapTrades used to return no ownership marker at all, so the web
// Transfer tab could draw the caravan dot but never say WHICH dot was the
// player's own internal transfer. Fixed by reading t.owner_id (already the
// origin settlement's owner for every dispatch site — province.go Trade,
// messenger.go accept legs 1+2) and comparing it to the caller.
//
// Proven here: (1) a transport the caller owns (origin) is visible AND
// mine=true, with quantity/good_key coming off the heaviest manifest good;
// (2) a transport neither owned by nor visible to the caller is filtered out
// entirely — the FOW gate itself must be UNCHANGED by this slice; (3) a
// transport owned by someone else but visible only because its DESTINATION
// lands on the caller's own settlement (an incoming trade delivery) is
// admitted (proving the gate still checks origin OR dest, not touched) with
// mine=false — ownership and visibility must not be conflated; (4) an
// unauthenticated caller never gets mine:true on anything.
//
// Real Postgres, gated by DATABASE_URL — same harness as recruit_ship_test.go
// and province_trade_internal_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mapTradesMineFixture struct {
	pool    *pgxpool.Pool
	worldID uuid.UUID
	playerA uuid.UUID
	tokenA  string
	playerB uuid.UUID
	router  *chi.Mux
}

// setupMapTradesMineFixture builds two players. A owns a settlement at (0,0).
// B owns two settlements far away at (50,50) and (53,50) — outside A's live
// vision — so distance alone keeps a B-only transport hidden from A.
func setupMapTradesMineFixture(t *testing.T) *mapTradesMineFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")

	registerPlayer := func(prefix string) (uuid.UUID, string) {
		username := prefix + "-" + uuid.New().String()
		token, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
		if err != nil {
			t.Fatalf("register %s: %v", prefix, err)
		}
		claims, err := authSvc.ValidateAccessToken(token)
		if err != nil {
			t.Fatalf("validate %s token: %v", prefix, err)
		}
		return claims.PlayerID, token
	}
	playerA, tokenA := registerPlayer("wanax-a")
	playerB, _ := registerPlayer("wanax-b")

	mkSettlement := func(name string, q, r int, owner uuid.UUID) (settlementID uuid.UUID) {
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, q, r,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
			worldID, provinceID, name, owner,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return settlementID
	}
	settleA := mkSettlement("Athenai", 0, 0, playerA)
	settleBFar := mkSettlement("Farland", 50, 50, playerB)
	settleBFar2 := mkSettlement("Farland-II", 53, 50, playerB)

	mkTransport := func(ownerID, originID, destID uuid.UUID, oq, or_, dq, dr int, goods map[string]float64) {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO transports
			   (world_id, owner_id, kind, origin_id, dest_id, category,
			    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
			 VALUES ($1,$2,'transfer',$3,$4,'land',$5,$6,$7,$8,$9,$10,1,'in_transit',true)
			 RETURNING id`,
			worldID, ownerID, originID, destID, oq, or_, dq, dr,
			time.Now(), time.Now().Add(time.Hour),
		).Scan(&id); err != nil {
			t.Fatalf("create transport: %v", err)
		}
		for good, qty := range goods {
			if _, err := pool.Exec(ctx,
				`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,$2,$3)`,
				id, good, qty,
			); err != nil {
				t.Fatalf("seed manifest %s: %v", good, err)
			}
		}
	}

	// transport1: A's own internal transfer, A -> B's far settlement. Heaviest
	// good is grain (250) over silver (10) — response must report grain/250.
	mkTransport(playerA, settleA, settleBFar, 0, 0, 50, 50,
		map[string]float64{"silver": 10, "grain": 250})

	// transport2: entirely B's business, far <-> far. Must never reach A at all.
	mkTransport(playerB, settleBFar, settleBFar2, 50, 50, 53, 50,
		map[string]float64{"timber": 5})

	// transport3: B's caravan delivering TO A's own settlement (an incoming
	// trade leg) — origin far (invisible) but dest lands exactly on A's own
	// settlement hex, so the existing origin-OR-dest FOW gate admits it. Owner
	// is B (origin's owner), so mine must be false for A even though A can see it.
	mkTransport(playerB, settleBFar, settleA, 50, 50, 0, 0,
		map[string]float64{"copper": 7})

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)

	r := chi.NewRouter()
	r.Use(auth.OptionalMiddleware(authSvc))
	r.Get("/worlds/{worldID}/trades", wh.MapTrades)

	return &mapTradesMineFixture{
		pool: pool, worldID: worldID, playerA: playerA, tokenA: tokenA, playerB: playerB, router: r,
	}
}

func (f *mapTradesMineFixture) get(t *testing.T, token string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/trades", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /trades = %d %q", rec.Code, rec.Body.String())
	}
	var markers []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("unmarshal response: %v (body: %s)", err, rec.Body.String())
	}
	return markers
}

// findByGood locates the single marker carrying the given good_key, or fails —
// every marker in this fixture carries a distinct heaviest good so this is
// an unambiguous lookup.
func findByGood(t *testing.T, markers []map[string]any, goodKey string) map[string]any {
	t.Helper()
	for _, m := range markers {
		if m["good_key"] == goodKey {
			return m
		}
	}
	t.Fatalf("no marker with good_key=%q in %v", goodKey, markers)
	return nil
}

// TestMapTrades_MineFlagAndQuantity is AC1: the caller's own transport is
// marked mine:true and carries the correct quantity for its heaviest good.
func TestMapTrades_MineFlagAndQuantity(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	markers := f.get(t, f.tokenA)

	own := findByGood(t, markers, "grain")
	if mine, _ := own["mine"].(bool); !mine {
		t.Errorf("own transport (grain leg): mine = %v, want true", own["mine"])
	}
	if qty, _ := own["quantity"].(float64); qty != 250 {
		t.Errorf("own transport quantity = %v, want 250", own["quantity"])
	}
}

// TestMapTrades_FOWGateUnchainedFromOwnership is AC1 + AC2 together: the
// far B<->B transport must be invisible to A (FOW gate untouched), while the
// B-owned transport landing on A's own settlement must still be visible
// (origin-OR-dest gate untouched) yet reported mine:false — ownership must
// never be inferred from visibility.
func TestMapTrades_FOWGateUnchangedFromOwnership(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	markers := f.get(t, f.tokenA)

	for _, m := range markers {
		if m["good_key"] == "timber" {
			t.Fatalf("far B<->B transport (timber) must be filtered out by FOW, but was present: %v", m)
		}
	}

	incoming := findByGood(t, markers, "copper")
	if mine, _ := incoming["mine"].(bool); mine {
		t.Errorf("incoming B-owned transport: mine = %v, want false", incoming["mine"])
	}
}

// TestMapTrades_UnauthenticatedNeverSeesMine is AC1's negative case: an
// anonymous caller sees all rows unfiltered (existing behaviour) but mine
// must never be true on any of them.
func TestMapTrades_UnauthenticatedNeverSeesMine(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	markers := f.get(t, "")

	if len(markers) != 3 {
		t.Fatalf("unauthenticated marker count = %d, want 3 (no FOW gate applied)", len(markers))
	}
	for _, m := range markers {
		if mine, _ := m["mine"].(bool); mine {
			t.Errorf("unauthenticated caller got mine:true on %v", m)
		}
	}
}

// TestMapTrades_RoleNamesTheSideTheCallerIsOn closes the recipient hole
// (2026-08-24). The row was never missing: FOWGateUnchangedFromOwnership above
// already proves that a B-owned transport landing on A's settlement reaches A
// with mine=false. What was missing was any way for A to tell it apart from a
// stranger's caravan passing their walls — so `keryx cargo` and the web's
// Transfer tab, both filtering on `mine`, showed a buyer nothing at all while
// the goods they had paid escrow for crossed the map.
//
// `mine` keeps its exact old meaning ("I dispatched this") and is asserted
// unchanged here, because widening it would have silently changed what every
// existing reader draws.
func TestMapTrades_RoleNamesTheSideTheCallerIsOn(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	markers := f.get(t, f.tokenA)

	own := findByGood(t, markers, "grain")
	if role, _ := own["role"].(string); role != "sender" {
		t.Errorf("transport A dispatched: role = %q, want \"sender\"", own["role"])
	}
	if mine, _ := own["mine"].(bool); !mine {
		t.Errorf("role must not disturb mine: own transport mine = %v, want true", own["mine"])
	}

	incoming := findByGood(t, markers, "copper")
	if role, _ := incoming["role"].(string); role != "recipient" {
		t.Errorf("transport landing on A's own settlement: role = %q, want \"recipient\" — "+
			"without this a buyer cannot see the cargo they already paid for", incoming["role"])
	}
	if mine, _ := incoming["mine"].(bool); mine {
		t.Errorf("mine must stay false for a shipment A did not dispatch, got %v", incoming["mine"])
	}
}

// TestMapTrades_UnauthenticatedNeverSeesRole mirrors the mine rule: an
// anonymous caller is told nothing about who is party to what.
func TestMapTrades_UnauthenticatedNeverSeesRole(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	for _, m := range f.get(t, "") {
		if role, _ := m["role"].(string); role != "" {
			t.Errorf("unauthenticated caller got role=%q on %v", role, m)
		}
	}
}
