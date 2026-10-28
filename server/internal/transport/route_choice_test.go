package transport

import (
	"context"
	"testing"
	"time"
)

func TestDispatchExplicitChoiceRejectsCategorySubstitution(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	for _, by := range []string{"sea", "sky"} {
		t.Run(by, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = CreateShadow(ctx, tx, DispatchParams{By: by, WorldID: f.worldID, OwnerID: f.owner, Kind: "trade_return", OriginID: f.sourceID, DestID: f.destID, Category: "land", OriginQ: 0, OriginR: 0, DestQ: 3, DestR: 0, DepartsAt: time.Now(), Manifest: Manifest{"grain": 1}})
			if err == nil {
				t.Fatalf("explicit %s silently used land", by)
			}
		})
	}
}
