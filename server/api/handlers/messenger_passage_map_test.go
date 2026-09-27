package handlers

// DB/handler tests for megaron_plan_ordna_passage.md slice 3b-1's MapMessengers
// rule: a foreign runner that is 'aboard' a carrier or 'returning_sealed'
// (sealed between a lost carrier and its port) must not be drawn for anyone
// but its owner — the flat origin→destination interpolation MapMessengers
// otherwise applies has no notion of "aboard" and drew it as a walker
// mid-sea. Same rig as marches_messengers_fow_test.go (citiesTestPool,
// registerViewer, insertProvince/insertSettlement, callMessengers).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/google/uuid"
)

// passageHiddenCase seeds one 'aboard' or 'returning_sealed' foreign runner
// whose origin AND destination both sit inside the viewer's vision (so, absent
// the 3b-1 rule, the ordinary interpolated-position gate would show it) and
// asserts it is never drawn for the foreign viewer.
func testForeignRunnerHiddenByPassageStatus(t *testing.T, passageStatus string) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "passage-viewer")

	var enemyID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"passage-enemy-"+uuid.New().String(),
	).Scan(&enemyID); err != nil {
		t.Fatalf("create enemy: %v", err)
	}

	capProv := insertProvince(t, ctx, pool, worldID, 0, 0)
	insertSettlement(t, ctx, pool, worldID, capProv, "Viewerton", viewerID, true)
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("map_tiles(%d,0): %v", q, err)
		}
	}
	// Both ends of the run, inside the viewer's radius-3 settlement vision —
	// so the ordinary endpoint/interpolation gate would show this runner were
	// it not sealed.
	originProv := insertProvince(t, ctx, pool, worldID, 2, 0)
	originSett := insertSettlement(t, ctx, pool, worldID, originProv, "Enemyburg", enemyID, true)
	destProv := insertProvince(t, ctx, pool, worldID, 4, 0)
	destSett := insertSettlement(t, ctx, pool, worldID, destProv, "Enemydest", enemyID, false)

	// The carrier the messenger row's carrier_unit_id FK requires — a real
	// enemy-owned ship, the same column boardOne sets in the real flow.
	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'galley', 'naval', 1, 10, 'marching', 3, 0) RETURNING id`,
		worldID, enemyID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create carrier ship: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, kind, message_text, status,
		      hex_q, hex_r, sent_at, arrives_at, passage_status, carrier_unit_id, carrier_name)
		 VALUES ($1, $2, $3, $4, 'message', 'hail', 'outbound',
		         4, 0, $5, $6, $7, $8, 'Enemy Galley')`,
		worldID, enemyID, originSett, destSett, now.Add(-1*time.Hour), now.Add(1*time.Hour),
		passageStatus, shipID,
	); err != nil {
		t.Fatalf("insert %s messenger: %v", passageStatus, err)
	}

	got := callMessengers(t, pool, authSvc, worldID, token, now)
	if len(got) != 0 {
		t.Fatalf("foreign %s runner must be hidden from a non-owner, got %d: %+v", passageStatus, len(got), got)
	}
}

func TestMapMessengers_ForeignAboardRunnerHidden(t *testing.T) {
	testForeignRunnerHiddenByPassageStatus(t, "aboard")
}

func TestMapMessengers_ForeignReturningSealedRunnerHidden(t *testing.T) {
	testForeignRunnerHiddenByPassageStatus(t, "returning_sealed")
}
