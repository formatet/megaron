package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// province cannot import unit (G1), so the two spellings of the chariot's
// price class are pinned to each other here.
func TestChariotMoveCategoryAgreesWithProvince(t *testing.T) {
	if unit.MoveCategory(unit.TypeWarChariot) != province.CategoryWarChariot {
		t.Fatalf("unit.MoveCategory(war_chariot) = %q, province.CategoryWarChariot = %q",
			unit.MoveCategory(unit.TypeWarChariot), province.CategoryWarChariot)
	}
	for _, c := range []struct {
		ty   unit.Type
		want string
	}{
		{unit.TypeSpearman, "land"}, {unit.TypeEliteInfantry, "land"},
		{unit.TypeGalley, "naval"}, {unit.TypeWarGalley, "naval"}, {unit.TypeMerchantman, "naval"},
		{unit.TypeNomadicHost, "land"},
	} {
		if got := unit.MoveCategory(c.ty); got != c.want {
			t.Errorf("MoveCategory(%s) = %q, want %q", c.ty, got, c.want)
		}
	}
	if got := unit.MoveCategoryOfRow("naval", "war_chariot"); got != "naval" {
		t.Errorf("a naval row is never re-priced, got %q", got)
	}
	if got := unit.MoveCategoryOfRow("land", "war_chariot"); got != "war_chariot" {
		t.Errorf("land+war_chariot row = %q", got)
	}
}

// A real StartMarch for a war chariot over the plains detour of
// setupLandMarchWorld: 3 steps of 0.67 = 670 milli each (a spearman: 1000).
func TestChariotMarchSavesChariotCosts(t *testing.T) {
	pool, worldID, ownerID := setupLandMarchWorld(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	run := func(utype string) (StoredRoute, int) {
		var unitID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
			 VALUES ($1, $2, $3, 'land', 100, 'positioned', 0, 0) RETURNING id`,
			worldID, ownerID, utype,
		).Scan(&unitID); err != nil {
			t.Fatalf("create %s: %v", utype, err)
		}
		res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: unitID, TargetQ: 2, TargetR: 0,
		}, nil)
		if err != nil {
			t.Fatalf("StartMarch %s: %v", utype, err)
		}
		raw, _, _ := loadMarchRoute(t, pool, unitID)
		var r StoredRoute
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		return r, res.DurationTicks
	}
	foot, footTicks := run("spearman")
	cha, chaTicks := run("war_chariot")
	for i, c := range foot.Costs {
		if c != 1000 {
			t.Errorf("spearman Costs[%d] = %d, want 1000", i, c)
		}
	}
	for i, c := range cha.Costs {
		if c != 670 {
			t.Errorf("chariot Costs[%d] = %d, want 670", i, c)
		}
	}
	if footTicks != 3 || chaTicks != 2 { // 3 × 0.67 = 2.01
		t.Errorf("duration spearman=%d chariot=%d ticks, want 3 and 2", footTicks, chaTicks)
	}
}
