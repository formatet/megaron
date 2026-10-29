package handlers

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/province"
	"testing"
	"time"
)

func TestCitiesOwnPortMetadataKeepsForeignHarbourPrivate(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE provinces SET coastal=true WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) SELECT id,'harbour',1 FROM settlements WHERE world_id=$1 ON CONFLICT DO NOTHING`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT $1,$2,p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 ON CONFLICT DO NOTHING`, f.worldID, f.playerA); err != nil {
		t.Fatal(err)
	}
	var hexes []province.MapPosition
	rows, err := f.pool.Query(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1`, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var h province.MapPosition
		if err := rows.Scan(&h.Q, &h.R); err != nil {
			t.Fatal(err)
		}
		hexes = append(hexes, h)
	}
	rows.Close()
	if err := province.RecordSeenSettlements(ctx, f.pool, f.worldID, f.playerA, hexes); err != nil {
		t.Fatal(err)
	}
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	own, foreign := false, false
	for _, row := range h.loadCities(ctx, f.worldID, f.playerA) {
		if row.Knowledge != "known" {
			continue
		}
		if row.Own {
			own = true
			if !row.Coastal || !row.HasHarbour {
				t.Fatalf("own city lost port metadata: %+v", row)
			}
		} else {
			foreign = true
			if !row.Coastal || row.HasHarbour {
				t.Fatalf("foreign city harbour must remain private: %+v", row)
			}
		}
	}
	if !own || !foreign {
		t.Fatalf("fixture must exercise known own and foreign cities: own%v foreign%v", own, foreign)
	}
}
