package combat

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// The audit witnesses every actual loss even after the last elite is gone.
// Routine land-garrison losses belong to the city transition notices; reading
// all previous notifications must not cause a fresh per-unit notice each tick.
func TestGarrisonAttritionAuditAndNoticeOwnership(t *testing.T) {
	cases := []struct {
		name, unitType, category, status string
		size, crew, ticks, wantNotices   int
		witness, disbanded               bool
	}{
		{"elite_last_unit", "elite_infantry", "land", "garrison", 1, 0, 1, 1, true, true},
		{"elite_garrison_read_all", "elite_infantry", "land", "garrison", 100, 0, 3, 0, true, false},
		{"spearman_garrison_read_all", "spearman", "land", "garrison", 100, 0, 3, 0, true, false},
		{"field_read_all", "elite_infantry", "land", "positioned", 100, 0, 3, 3, false, false},
		{"naval_garrison_read_all", "galley", "naval", "garrison", 1, 20, 3, 3, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			f := newSupportFixture(t, pool, "attrition-notice")
			seedGoods(t, pool, f.capitalID, f.tick, 0, 100000)
			var uid uuid.UUID
			var location any
			if tc.status == "garrison" {
				location = f.capitalID
			}
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,type,category,size,crew,status,q,r) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,0,0) RETURNING id`, f.worldID, f.owner, location, f.capitalID, tc.unitType, tc.category, tc.size, tc.crew, tc.status).Scan(&uid); err != nil {
				t.Fatal(err)
			}
			rec := &unpaidWarningRecorder{pool: pool}
			clk := clock.NewTestClock(time.Unix(0, 0))
			h := NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), rec, nil)
			h.soldShare = 0
			for n := 0; n < tc.ticks; n++ {
				if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, f.tick+n); err != nil {
					t.Fatal(err)
				}
				if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(14009000 + n), WorldID: f.worldID, DueTick: f.tick + n}); err != nil {
					t.Fatal(err)
				}
				// Model an attentive reader between every tick; unread dedupe is insufficient.
				if _, err := pool.Exec(ctx, `UPDATE notifications SET read_at=now() WHERE world_id=$1`, f.worldID); err != nil {
					t.Fatal(err)
				}
			}
			var auditCount, witnessCount int
			if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE payload->>'garrison_settlement_id'=$2) FROM events WHERE stream_id=$1 AND event_type='UnitAttrition' AND payload->>'reason'='food_shortage'`, uid, f.capitalID.String()).Scan(&auditCount, &witnessCount); err != nil {
				t.Fatal(err)
			}
			if auditCount != tc.ticks {
				t.Errorf("daily actual-loss audits=%d want%d", auditCount, tc.ticks)
			}
			wantWitness := 0
			if tc.witness {
				wantWitness = tc.ticks
			}
			if witnessCount != wantWitness {
				t.Errorf("pre-loss garrison witnesses=%d want%d", witnessCount, wantWitness)
			}
			var size, crew int
			var status string
			if err := pool.QueryRow(ctx, `SELECT size,crew,status FROM units WHERE id=$1`, uid).Scan(&size, &crew, &status); err != nil {
				t.Fatal(err)
			}
			if tc.disbanded {
				if status != "disbanded" || size != 0 {
					t.Errorf("last unit=%s size%d", status, size)
				}
			} else if tc.category == "land" && size >= tc.size || tc.category == "naval" && crew >= tc.crew {
				t.Errorf("fixture did not cause actual loss: size%d crew%d", size, crew)
			}
			gotNotices := 0
			for _, call := range rec.calls {
				if call.kind == "UnitAttrition" {
					gotNotices++
					if call.payload["disbanded"] != tc.disbanded {
						t.Errorf("destruction flag=%v", call.payload["disbanded"])
					}
					if tc.disbanded && call.level != 2 {
						t.Errorf("destroyed unit notice level=%d", call.level)
					}
				}
			}
			if gotNotices != tc.wantNotices {
				t.Errorf("UnitAttrition notices after read-all=%d want%d", gotNotices, tc.wantNotices)
			}
		})
	}
}
