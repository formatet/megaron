package handlers

// Slice 2 of megaron_plan_spawn_landmassa.md: join's spawn-tile ORDER BY must
// balance per landmass_id (persisted by Slice 1/migration 124), not per
// hemisphere. The hemisphere rule split the map at a single vertical line
// (half_q) — a landmass that itself straddles that line can absorb every
// joiner while a genuinely separate, untouched landmass sits empty, because
// tier-1 only ever asked "which SIDE has fewer occupants", not "which
// LANDMASS".
//
// Fixture (landmass_id is a plain column here, not recomputed via
// world.LandComponents — Slice 1 already proves that computation separately;
// this test only exercises the join query's use of the stored value):
//   - map_width = 12 -> half_q = (12-1)/2 = 5 (west: q<=5, east: q>5).
//   - Landmass 1 ("B"): one lone WEST tile (0,0), plus two EAST tiles
//     (7,100) and (8,100) sitting right next to a tin deposit (9,100) —
//     within catchment radius of both, so both rank top on the ore-bias
//     tiebreak. All far apart in raw hex distance from each other, so the
//     4-hex clearance filter never lets occupying one exclude the others.
//   - Landmass 2 ("A"): two EAST tiles (8,200) and (9,200), far from
//     everything above (no ore bias, no clearance interaction).
//
// Red-before (unmodified hemisphere ORDER BY): join #1 has no other choice
// than landmass B's only west tile (0,0) (tier-1 hard-partitions to west
// when west_count<=east_count and B is the only tile w/ q<=5). Join #2 then
// sees west_count=1 > east_count=0, so tier-1 hard-partitions to east — where
// B's own ore-biased tiles (7,100)/(8,100) outrank A's unbiased ones on
// tier-2, so join #2 ALSO lands on landmass B. Bug: two joiners, one
// landmass, and the untouched one never gets a look.
//
// Green-after (landmass-load ORDER BY): join #1 still has to pick from
// SOMEWHERE (both landmasses tie at load=0, so tier-2's ore bias decides —
// landmass B wins, same as before). But join #2 now sees landmass_load
// B=1 > A=0, and tier-1 (ascending load) strictly outranks tier-2 across
// landmasses, so join #2 is forced onto landmass A regardless of ore bias.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedTwoLandmassWorld builds the fixture described above and returns the
// world id. landmass_id 1 = "B" (has the ore-biased east tiles + the lone
// west tile), landmass_id 2 = "A" (untouched, unbiased).
func seedTwoLandmassWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_width, map_height) VALUES ($1, 12, 250) RETURNING id`,
		"test-landmass-balance-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	type tile struct {
		q, r       int
		terrain    string
		landmassID *int
		tinDeposit bool
	}
	one, two := 1, 2
	tiles := []tile{
		{0, 0, "plains", &one, false},       // landmass B, west (halfQ=5, q<=5)
		{7, 100, "plains", &one, false},     // landmass B, east, near the tin deposit
		{8, 100, "plains", &one, false},     // landmass B, east, near the tin deposit
		{9, 100, "mountain_red", nil, true}, // deposit-only tile, excluded from candidacy by terrain
		{8, 200, "plains", &two, false},     // landmass A, east, untouched, unbiased
		{9, 200, "plains", &two, false},     // landmass A, east, untouched, unbiased
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, tin_deposit, landmass_id) VALUES ($1, $2, $3, $4, $5, $6)`,
			worldID, tl.q, tl.r, tl.terrain, tl.tinDeposit, tl.landmassID,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	return worldID
}

func TestJoin_BalancesPerLandmassNotHemisphere(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedTwoLandmassWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))

	joinOnce := func(prefix string) (q, r2 int) {
		_, token := registerViewer(t, ctx, authSvc, prefix)
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /join (%s) = %d %q, want 201", prefix, rec.Code, rec.Body.String())
		}
		var resp struct {
			Tile struct {
				Q int `json:"Q"`
				R int `json:"R"`
			} `json:"tile"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /join response (%s): %v (body: %s)", prefix, err, rec.Body.String())
		}
		return resp.Tile.Q, resp.Tile.R
	}

	q1, r1 := joinOnce("landmass-balance-1")
	q2, r2 := joinOnce("landmass-balance-2")

	landmassOf := func(q, r int) *int {
		var id *int
		if err := pool.QueryRow(ctx,
			`SELECT landmass_id FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, q, r,
		).Scan(&id); err != nil {
			t.Fatalf("read landmass_id for (%d,%d): %v", q, r, err)
		}
		return id
	}
	lm1, lm2 := landmassOf(q1, r1), landmassOf(q2, r2)
	if lm1 == nil || lm2 == nil {
		t.Fatalf("join landed on a tile with NULL landmass_id: join1=(%d,%d)->%v join2=(%d,%d)->%v", q1, r1, lm1, q2, r2, lm2)
	}
	if *lm1 != 1 {
		t.Fatalf("join 1 landed on landmass %d at (%d,%d), want landmass 1 (the only candidate the hemisphere/ore-bias fixture leaves for the first joiner)", *lm1, q1, r1)
	}
	if *lm1 == *lm2 {
		t.Fatalf("join 1 -> landmass %d (%d,%d), join 2 -> landmass %d (%d,%d): both landed on the SAME landmass — spawn balance is still per-hemisphere, not per-landmass",
			*lm1, q1, r1, *lm2, q2, r2)
	}
}
