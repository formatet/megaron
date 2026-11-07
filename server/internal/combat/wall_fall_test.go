package combat

// Murarna faller (steg 5c): the scheduled WallsFall lowers settlements.wall_level AND the 'wall'
// building row together, once per cast, and never touches a battle already begun.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func wallsFallFixture(t *testing.T, pool *pgxpool.Pool, wall int) (siegeFixture, *WallsFallHandler, *fakeBroadcaster) {
	t.Helper()
	f := newSiegeFixture(t, pool, wall)
	if wall >= 1 {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'wall', $2)`, f.defSettlement, wall); err != nil {
			t.Fatal(err)
		}
	}
	fb := &fakeBroadcaster{}
	return f, NewWallsFallHandler(pool, events.NewStore(pool), fb), fb
}

// wall_falls outlives the test world, like every claim table: cast numbers are salted per run.
var castSalt = time.Now().UnixNano() / 1000 * 100

func runWallsFall(t *testing.T, h *WallsFallHandler, f siegeFixture, cast int64) {
	t.Helper()
	cast += castSalt
	raw, _ := json.Marshal(WallsFallPayload{WorldID: f.worldID, SettlementID: f.defSettlement, CastEventID: cast})
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.worldID, EventType: events.ScheduledWallsFall, Payload: raw}); err != nil {
		t.Fatalf("walls fall: %v", err)
	}
}

// wallBoth reads the two stores of the wall: the settlement column and the building row (-1 = no row).
func wallBoth(t *testing.T, pool *pgxpool.Pool, f siegeFixture) (col, row int) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT wall_level FROM settlements WHERE id=$1`, f.defSettlement).Scan(&col); err != nil {
		t.Fatal(err)
	}
	row = -1
	_ = pool.QueryRow(ctx, `SELECT level FROM buildings WHERE settlement_id=$1 AND building_type='wall'`, f.defSettlement).Scan(&row)
	return
}

func TestWallsFall_OneLevelInBothStores(t *testing.T) {
	pool := testPool(t)
	f, h, _ := wallsFallFixture(t, pool, 2)
	runWallsFall(t, h, f, 1)
	if col, row := wallBoth(t, pool, f); col != 1 || row != 1 {
		t.Fatalf("wall column %d, building row %d, want 1 and 1", col, row)
	}
}

func TestWallsFall_ToZeroDeletesTheWallBuildingRow(t *testing.T) {
	pool := testPool(t)
	f, h, _ := wallsFallFixture(t, pool, 1)
	runWallsFall(t, h, f, 2)
	if col, row := wallBoth(t, pool, f); col != 0 || row != -1 {
		t.Fatalf("wall column %d, building row %d, want 0 and no row", col, row)
	}
}

func TestWallsFall_WallAlreadyZeroDoesNothing(t *testing.T) {
	pool := testPool(t)
	f, h, fb := wallsFallFixture(t, pool, 0)
	runWallsFall(t, h, f, 3)
	if col, row := wallBoth(t, pool, f); col != 0 || row != -1 {
		t.Fatalf("wall column %d, building row %d", col, row)
	}
	if len(fb.notified) != 0 {
		t.Fatalf("a wall that did not fall was announced: %v", fb.notified)
	}
}

func TestWallsFall_TheSameCastRunTwiceIsOneLevel(t *testing.T) {
	pool := testPool(t)
	f, h, fb := wallsFallFixture(t, pool, 3)
	runWallsFall(t, h, f, 4)
	runWallsFall(t, h, f, 4)
	if col, row := wallBoth(t, pool, f); col != 2 || row != 2 {
		t.Fatalf("wall column %d, building row %d, want 2 and 2 after a double run", col, row)
	}
	if len(fb.notified) != 1 {
		t.Fatalf("%d notices for one fall", len(fb.notified))
	}
}

func TestWallsFall_TwoCastsFallOneLevelEach(t *testing.T) {
	pool := testPool(t)
	f, h, _ := wallsFallFixture(t, pool, 3)
	runWallsFall(t, h, f, 5)
	runWallsFall(t, h, f, 6)
	if col, row := wallBoth(t, pool, f); col != 1 || row != 1 {
		t.Fatalf("wall column %d, building row %d, want 1 and 1", col, row)
	}
}

func TestWallsFall_OwnerIsToldAlwaysAndNeverByWhom(t *testing.T) {
	pool := testPool(t)
	f, h, fb := wallsFallFixture(t, pool, 2)
	runWallsFall(t, h, f, 7)
	if len(fb.notified) != 1 || fb.notified[0] != NoticeWallsFell {
		t.Fatalf("notices %v", fb.notified)
	}
	raw, _ := json.Marshal(fb.payloads[0])
	var ev []byte
	_ = pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type=$2`, f.worldID, EventWallsFell).Scan(&ev)
	for _, text := range []string{string(raw), string(ev)} {
		if strings.Contains(text, f.attacker.String()) || strings.Contains(strings.ToLower(text), "attacker") {
			t.Fatalf("the praying Wanax leaks into %s", text)
		}
	}
	var got struct{ WallBefore, WallAfter int }
	if err := json.Unmarshal([]byte(strings.NewReplacer("wall_before", "WallBefore", "wall_after", "WallAfter").Replace(string(ev))), &got); err != nil || got.WallBefore != 2 || got.WallAfter != 1 {
		t.Fatalf("event %s", ev)
	}
}

// A battle already under way keeps the wall it began with; one begun after the fall sees the new level.
func TestWallsFall_RunningBattleKeepsItsSnapshotNewBattlesSeeTheFall(t *testing.T) {
	pool := testPool(t)
	f, h, _ := wallsFallFixture(t, pool, 2)
	mkSiegeGarrison(t, pool, f, 300)
	att := mkSiegeAttacker(t, pool, f, 400, false)
	ah := newArrivalHandler(pool, economy.NewWallDice())
	runFieldArrival(t, pool, ah, f.worldID, att)
	running := loadBattleID(t, pool, f.worldID, 1, 0)

	runWallsFall(t, h, f, 8)

	snapshot := func(id uuid.UUID) (n int) {
		if err := pool.QueryRow(context.Background(), `SELECT wall_level FROM battles WHERE id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := snapshot(running); got != 2 {
		t.Fatalf("the running battle's wall changed to %d", got)
	}
	// A new battle in a world where the wall has already fallen.
	f2, h2, _ := wallsFallFixture(t, pool, 2)
	runWallsFall(t, h2, f2, 9)
	mkSiegeGarrison(t, pool, f2, 300)
	att2 := mkSiegeAttacker(t, pool, f2, 400, false)
	runFieldArrival(t, pool, newArrivalHandler(pool, economy.NewWallDice()), f2.worldID, att2)
	if got := snapshot(loadBattleID(t, pool, f2.worldID, 1, 0)); got != 1 {
		t.Fatalf("a battle begun after the fall sees wall %d, want 1", got)
	}
}
