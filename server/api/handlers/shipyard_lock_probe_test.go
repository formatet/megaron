package handlers

// Opt-in diagnostic: real Recruit, completion and startup transition, with
// test-only advisory barriers. Never manufacture a reversed production lock.
// tools/gotest.sh ./api/handlers/ -run '^TestShipyardRecruitLockProbe$' -v -args -shipyard-lock-probe
import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

var shipyardLockProbe = flag.Bool("shipyard-lock-probe", false, "run isolated shipyard lock diagnostics on a fresh DB")

func TestShipyardRecruitLockProbe(t *testing.T) {
	if !*shipyardLockProbe {
		t.Skip("opt-in two-session diagnostic; run alone on a fresh DB with -shipyard-lock-probe")
	}
	for _, mode := range []string{"completion_unit_first", "completion_city_first", "startup_cutover_unit_first"} {
		t.Run(mode, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := f.pool.Exec(ctx, q, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID)
			rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
			if rec.Code != http.StatusAccepted {
				t.Fatal(rec.Code, rec.Body.String())
			}
			var ship uuid.UUID
			var rev int
			if err := f.pool.QueryRow(ctx, `SELECT id,shipyard_work_revision FROM units WHERE settlement_id=$1 AND status='forming'`, f.settlementID).Scan(&ship, &rev); err != nil {
				t.Fatal(err)
			}
			exec(`UPDATE worlds SET current_tick=20 WHERE id=$1`, f.worldID)
			cutover := strings.HasPrefix(mode, "startup")
			if cutover {
				exec(`UPDATE units SET shipyard_work_remaining=NULL,build_complete_at=NULL WHERE id=$1`, ship)
				raw, _ := json.Marshal(map[string]any{"unit_id": ship, "settlement_id": f.settlementID})
				exec(`INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,'TrainComplete',$2,now(),30)`, f.worldID, raw)
				exec(`UPDATE shipyard_work_transition SET completed=false WHERE id=true`)
			}
			gate, err := f.pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			key := int64(57000000) + int64(time.Now().UnixNano()%1000000)
			if _, err = gate.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
				gate.Release()
				t.Fatal(err)
			}
			released := false
			release := func() {
				if !released {
					_, _ = gate.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
					gate.Release()
					released = true
				}
			}
			defer release()
			var gatePID int
			if err = gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
				t.Fatal(err)
			}
			table, target := "units", ship
			if strings.Contains(mode, "city_first") {
				table, target = "settlements", f.settlementID
			}
			name := "varvet_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(%d); RETURN NEW; END $$`, name, key))
			exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW WHEN (OLD.id='%s'::uuid) EXECUTE FUNCTION %s()`, name, table, target, name))
			defer func() {
				_, _ = f.pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, name, table))
				_, _ = f.pool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, name))
			}()
			workerDone := make(chan error, 1)
			recruitDone := make(chan *httptest.ResponseRecorder, 1)
			worker := func() {
				if cutover {
					workerDone <- economy.CompleteShipyardWorkTransition(ctx, f.pool)
					return
				}
				raw, _ := json.Marshal(combat.ShipyardWorkCompletePayload{UnitID: ship, Revision: rev})
				h := combat.NewShipyardWorkCompleteHandler(f.pool, events.NewStore(f.pool), nil)
				workerDone <- h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: raw})
			}
			recruit := func() {
				req := httptest.NewRequest(http.MethodPost, recruitPath(f), bytes.NewBufferString(`{"unit_type":"galley"}`)).WithContext(ctx)
				req.Header.Set("Authorization", "Bearer "+f.accessToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				f.router.ServeHTTP(rec, req)
				recruitDone <- rec
			}
			waitBlocked := func(blocker int) int {
				t.Helper()
				until := time.NewTimer(4 * time.Second)
				defer until.Stop()
				poll := time.NewTicker(10 * time.Millisecond)
				defer poll.Stop()
				for {
					var pid int
					err := f.pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)) ORDER BY pid LIMIT 1`, blocker).Scan(&pid)
					if err == nil {
						t.Logf("observed PostgreSQL wait: pid %d blocked by %d", pid, blocker)
						return pid
					}
					select {
					case <-until.C:
						t.Fatalf("no observed blocker %d: %v", blocker, err)
						return 0
					case <-poll.C:
					}
				}
			}
			if table == "units" {
				go worker()
			} else {
				go recruit()
			}
			firstPID := waitBlocked(gatePID)
			if table == "units" {
				go recruit()
			} else {
				go worker()
			}
			secondPID := waitBlocked(firstPID)
			t.Logf("%s: wait chain %d -> %d -> diagnostic barrier %d; releasing barrier", mode, secondPID, firstPID, gatePID)
			release()
			select {
			case err := <-workerDone:
				if err != nil {
					t.Fatalf("production shipyard path failed: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case rec := <-recruitDone:
				if rec.Code != http.StatusAccepted {
					t.Fatalf("production Recruit failed: %d %s", rec.Code, rec.Body.String())
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var pop, n int
			if err := f.pool.QueryRow(ctx, `SELECT population,(SELECT count(*) FROM units WHERE settlement_id=$1) FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop, &n); err != nil {
				t.Fatal(err)
			}
			if pop != 4960 || n != 2 {
				t.Fatalf("lost or partial work: population=%d units=%d", pop, n)
			}
			t.Logf("%s: both real operations committed; population=%d units=%d; no deadlock/SQL abort", mode, pop, n)
		})
	}
}
