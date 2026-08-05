package handlers

// The read surface must project the drain the upkeep tick actually takes.
// Two things break that if the projection groups by "who stands here" and
// counts the gross:
//
//   - mig 100 made support_settlement_id the sole payer, so a marching unit
//     still bills the town that raised it (understated drain — says a city is
//     sustainable when it isn't), while a unit garrisoning here on someone
//     else's payroll costs this city nothing (overstated).
//   - Del C circulates soldShare of a home garrison's silver back into the
//     town, so its net silver drain is (1−share)·gross (overstated).
//
// The fixture deliberately puts one unit in each of the three positions, so a
// projection that gets any one of them wrong lands on a different number.
// DB integration test (real Postgres, gated by DATABASE_URL via the shared
// armyDisplayTestPool helper).

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"

	"formatet/megaron/server/internal/auth"
)

func TestSettlementUpkeepDrain_PayerAndSoldCirculation(t *testing.T) {
	pool := armyDisplayTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
	username := "upkeepdrain-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	newSettlement := func(q, r int, name, control string, capital bool) uuid.UUID {
		t.Helper()
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal)
			 VALUES ($1, $2, $3, 'plains', true) RETURNING id`,
			worldID, q, r,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		var settlementID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6) RETURNING id`,
			worldID, provinceID, name, playerID, control, capital,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return settlementID
	}

	// Mykene pays for everything below; Tiryns is a colony of the same Wanax
	// that pays for its own garrison.
	mykene := newSettlement(0, 0, "Mykene", "capital", true)
	tiryns := newSettlement(1, 0, "Tiryns", "colony", false)

	// settlementID = where the unit stands, supportSettlementID = who pays.
	seed := []struct {
		utype    string
		category string
		size     int
		status   string
		standsIn uuid.UUID
		paidBy   uuid.UUID
	}{
		// Home garrison of Mykene: 100 spearmen, grain 50 / silver 2.
		// The sold circulates → net silver 0.6 at share 0.7.
		{"spearman", "land", 100, "garrison", mykene, mykene},
		// Mykene's field army, marching: still on Mykene's payroll (mig 100),
		// but spends nothing at home → full 6 silver drains.
		{"war_chariot", "land", 100, "marching", uuid.Nil, mykene},
		// Tiryns' own garrison, standing in Tiryns. Costs Mykene nothing —
		// grouping by "units in this world owned by me" would wrongly bill it.
		{"spearman", "land", 100, "garrison", tiryns, tiryns},
		// A forming unit draws no upkeep yet, in any position.
		{"spearman", "land", 60, "forming", mykene, mykene},
	}
	for _, u := range seed {
		var standsIn any
		if u.standsIn != uuid.Nil {
			standsIn = u.standsIn
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, status,
			                    settlement_id, support_settlement_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			worldID, playerID, u.utype, u.category, u.size, u.status, standsIn, u.paidBy,
		); err != nil {
			t.Fatalf("seed unit %s/%s: %v", u.utype, u.status, err)
		}
	}

	const eps = 1e-9
	const share = 0.7

	gross, circulated, err := settlementUpkeepDrain(ctx, pool, mykene, share)
	if err != nil {
		t.Fatalf("settlementUpkeepDrain(mykene): %v", err)
	}
	// Mykene pays for the spearmen (50 garrison grain / 2 silver) and the marching
	// chariot (80 base ×2 field factor = 160 grain / 6 silver, SLICE A 2026-08-05).
	// Tiryns' garrison and the forming unit are not its bill.
	if math.Abs(gross.Grain-210) > eps {
		t.Errorf("Mykene gross grain = %v, want 210 (50 garrison + 160 marching chariot)", gross.Grain)
	}
	if math.Abs(gross.Silver-8) > eps {
		t.Errorf("Mykene gross silver = %v, want 8 (2 garrison + 6 marching chariot)", gross.Silver)
	}
	// Only the unit standing in the town that pays it circulates: 0.7 × 2.
	// The marching chariot's 6 does not — that is the whole point of Del C.
	if math.Abs(circulated-1.4) > eps {
		t.Errorf("Mykene circulated silver = %v, want 1.4 (0.7 × 2 from the home garrison only)", circulated)
	}
	if net := gross.Silver - circulated; math.Abs(net-6.6) > eps {
		t.Errorf("Mykene net silver drain = %v, want 6.6", net)
	}

	// The colony pays only for its own garrison, and all of it comes back.
	grossT, circulatedT, err := settlementUpkeepDrain(ctx, pool, tiryns, share)
	if err != nil {
		t.Fatalf("settlementUpkeepDrain(tiryns): %v", err)
	}
	if math.Abs(grossT.Silver-2) > eps {
		t.Errorf("Tiryns gross silver = %v, want 2 (its own garrison only)", grossT.Silver)
	}
	if math.Abs(circulatedT-1.4) > eps {
		t.Errorf("Tiryns circulated silver = %v, want 1.4", circulatedT)
	}

	// share = 0 must be bit-identical to the pre-Del-C world: nothing circulates.
	_, circulatedZero, err := settlementUpkeepDrain(ctx, pool, mykene, 0)
	if err != nil {
		t.Fatalf("settlementUpkeepDrain(share=0): %v", err)
	}
	if circulatedZero != 0 {
		t.Errorf("circulated at share=0 = %v, want 0 (identity with pre-Del-C)", circulatedZero)
	}
}
