package kharis

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
)

// The kharis tick is what makes a resting temple recover — and only while it
// is fully staffed (bönegrunden).
func TestKharisTickCountsTempleRestDownOnlyWhenStaffed(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO temple_rest(building_id,remaining_ticks,last_tick) SELECT id,5,0 FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, sid); err != nil {
		t.Fatal(err)
	}
	rest := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT remaining_ticks FROM temple_rest tr JOIN buildings b ON b.id=tr.building_id WHERE b.settlement_id=$1),0)`, sid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	h := newTestTickHandler(pool)
	staffTestTemple(t, pool, sid, 1)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 270001, WorldID: wid, DueTick: 1}); err != nil {
		t.Fatal(err)
	}
	if got := rest(); got != 5 {
		t.Fatalf("understaffed temple recovered: %d", got)
	}
	staffTestTemple(t, pool, sid, 2)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 270002, WorldID: wid, DueTick: 2}); err != nil {
		t.Fatal(err)
	}
	if got := rest(); got != 4 {
		t.Fatalf("fully staffed temple rest %d want 4", got)
	}
}
