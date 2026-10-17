package combat

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFoundingNotificationRunway_Colony(t *testing.T) {
	for _, tc := range []struct {
		name        string
		grain, fish float64
	}{{"dust", 5 - 1e-15, 0}, {"fish", 0, 5}, {"real deficit", 4, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()

			// foundColony's settlement_goods seeding calls current_world_tick(), which
			// reads the single globally-active world (status='active', enforced by the
			// one_active_world unique partial index) — so this fixture world must be
			// active. It is cleaned up (deleted) at the end of the test, restoring "no
			// active world". A prior test run that crashed before t.Cleanup ran can
			// leave an active `test-world-…` behind on the shared dev DB, which would
			// trip one_active_world on this run's INSERT. Archive our own detritus first
			// — the name prefix guarantees we only ever touch this test's leftovers,
			// never a real world. Archiving (not deleting) frees the partial unique
			// index (it only covers status='active') without chasing the web of
			// settlement-referencing tables that lack ON DELETE CASCADE.
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
			var ownerID uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
				"colonizer-"+uuid.New().String(),
			).Scan(&ownerID); err != nil {
				t.Fatalf("create test player: %v", err)
			}
			t.Cleanup(func() {
				// Archive rather than delete: it restores "no active world" (all the
				// unique index cares about) without needing to unwind the settlement-
				// referencing tables that lack ON DELETE CASCADE. Any leftover an
				// aborted run misses is re-archived by the sweep above on the next run.
				_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
			})

			// The owner needs a capital settlement — foundColony looks up the parent/
			// culture from it.
			var capitalProvinceID uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
				worldID,
			).Scan(&capitalProvinceID); err != nil {
				t.Fatalf("create capital province: %v", err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital City', 'achaean', $3, 'capital', true)`,
				worldID, capitalProvinceID, ownerID,
			); err != nil {
				t.Fatalf("create capital settlement: %v", err)
			}

			// Empty province for the new colony (reused, so foundColony skips the
			// map_tiles lookup path).
			var colonyProvinceID uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 10, 'plains') RETURNING id`,
				worldID,
			).Scan(&colonyProvinceID); err != nil {
				t.Fatalf("create colony province: %v", err)
			}

			const unitSize = 1000
			var unitID uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 'marching', 10, 10) RETURNING id`,
				worldID, ownerID, unitSize,
			).Scan(&unitID); err != nil {
				t.Fatalf("create colonizing unit: %v", err)
			}

			colonyName := "Newhaven"
			u := unitRow{
				id:         unitID,
				ownerID:    ownerID,
				utype:      "spearman",
				category:   "land",
				size:       unitSize,
				status:     "marching",
				q:          10,
				r:          10,
				colonyName: &colonyName,
			}

			recorder := &fakeBroadcaster{}
			installFoundingRateFixture(t, pool, worldID, tc.grain, tc.fish)
			h := &UnitArrivalHandler{
				pool:       pool,
				eventStore: events.NewStore(pool),
				hub:        recorder,
				scheduler:  nil,
				clk:        clock.NewTestClock(time.Now()),
			}

			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin tx: %v", err)
			}
			defer tx.Rollback(ctx)

			if err := h.foundColony(ctx, tx, u, colonyProvinceID, 10, 10, worldID); err != nil {
				t.Fatalf("foundColony: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("commit: %v", err)
			}

			var b map[string]any
			for i, kind := range recorder.notified {
				if kind == "ColonyFounded" {
					b = recorder.payloads[i].(map[string]any)
				}
			}
			if b == nil {
				t.Fatal("no ColonyFounded payload")
			}
			net, ok := b["food_net_per_tick"].(float64)
			expected := tc.grain + tc.fish - 5
			if !ok || math.Abs(net-expected) > 1e-14 {
				t.Fatalf("full food net absent/wrong: %+v", b)
			}
			ticks, present := b["grain_ticks"]
			alias, aliasPresent := b["grain_days"]
			if expected < -1e-9 {
				amount := b["grain_amount"].(float64)
				if !present || !aliasPresent || ticks.(float64) != amount/-net || alias != ticks {
					t.Fatalf("real deficit lost runway: %+v", b)
				}
			} else if present || aliasPresent {
				t.Fatalf("dust/fish wrote false runway: %+v", b)
			}

		})
	}
}

func installFoundingRateFixture(t *testing.T, pool *pgxpool.Pool, wid uuid.UUID, grain, fish float64) {
	t.Helper()
	ctx := context.Background()
	name := "founding_rate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ddl := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $fn$
 BEGIN
 IF EXISTS(SELECT 1 FROM settlements s WHERE s.id=NEW.settlement_id AND s.world_id='%s') THEN
 IF NEW.good_key='grain' THEN NEW.rate:=%s; ELSIF NEW.good_key='fish' THEN NEW.rate:=%s; END IF;
 END IF;RETURN NEW;END $fn$;
 CREATE TRIGGER %s BEFORE INSERT OR UPDATE ON settlement_goods FOR EACH ROW EXECUTE FUNCTION %s();`, name, wid, strings.TrimSpace(strconv.FormatFloat(grain, 'g', -1, 64)), strings.TrimSpace(strconv.FormatFloat(fish, 'g', -1, 64)), name, name)
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON settlement_goods; DROP FUNCTION %s();`, name, name)); err != nil {
			t.Error(err)
		}
	})
}
