package handlers

// Slice contract: failed or concurrent recruitment must not leave payment without
// the unit and training job. Invariant: existing crew/cohort, costs and queue cap.
// Scope: Recruit transaction and transactional UnitFormed persistence. Non-scope:
// the larger action/evaluator extraction, new game rules and client rendering.
// Acceptance: real HTTP + DB rollback at unit/job/event failure, batch atomicity,
// exact affordable naval cost, and concurrent population/queue checks. Stop on
// new canon. Evidence: failing tests against the pre-fix code, then fresh DB suite.
import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type recruitState struct {
	Population                                 int
	Goods                                      map[string]float64
	Units, Jobs, Events, Ordinals, NextOrdinal int
}

func snapshotRecruitState(t *testing.T, f *recruitShipFixture) recruitState {
	t.Helper()
	ctx := context.Background()
	s := recruitState{Goods: make(map[string]float64)}
	if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&s.Population); err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(ctx, `SELECT good_key,amount FROM settlement_goods WHERE settlement_id=$1`, f.settlementID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var good string
		var n float64
		if err := rows.Scan(&good, &n); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		s.Goods[good] = n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM units WHERE settlement_id=$1),
 (SELECT count(*) FROM scheduled_events WHERE payload->>'settlement_id'=$1::text),
 (SELECT count(*) FROM events WHERE world_id=$2 AND event_type='UnitFormed'),
 (SELECT count(*) FROM unit_ordinals WHERE settlement_id=$1),
 (SELECT COALESCE(sum(next_ordinal),0) FROM unit_ordinals WHERE settlement_id=$1)`, f.settlementID, f.worldID).Scan(&s.Units, &s.Jobs, &s.Events, &s.Ordinals, &s.NextOrdinal); err != nil {
		t.Fatal(err)
	}
	return s
}
func recruitPath(f *recruitShipFixture) string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
}

func TestRecruit_GalleyAtActualCrewCost(t *testing.T) {
	f := setupRecruitShipFixture(t)
	sink := observeRecruitEvents(t, f)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='barracks'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=CASE good_key WHEN 'timber' THEN 30 WHEN 'silver' THEN 6 ELSE 0 END,rate=0 WHERE settlement_id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("payable galley got %d %s; want 201", rec.Code, rec.Body.String())
	}
	if sink.count != 1 {
		t.Fatalf("successful recruit published %d events, want1", sink.count)
	}
	s := snapshotRecruitState(t, f)
	if s.Goods["timber"] != 0 || s.Goods["silver"] != 0 || s.Population != 4980 || s.Units != 1 || s.Jobs != 1 || s.Events != 1 {
		t.Fatalf("successful galley state: %+v", s)
	}
}

// The trigger affects only this fixture and is removed even on failure. Inject
// errors in real Postgres, rather than replacing the handler with a mock.
func rejectRecruitWrite(t *testing.T, f *recruitShipFixture, table, condition string) {
	t.Helper()
	name := "recruit_fail_" + uuid.New().String()[:8]
	ctx := context.Background()
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected recruit failure'; END; $$;
 CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW WHEN (%s) EXECUTE FUNCTION %s();`, name, name, table, condition, name)
	if _, err := f.pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s; DROP FUNCTION IF EXISTS %s();`, name, table, name)); err != nil {
			t.Error(err)
		}
	})
}
func TestRecruit_FailureRollsBackWholeAction(t *testing.T) {
	cases := []struct {
		name, unit, table string
		count             int
		condition         func(*recruitShipFixture) string
	}{
		{"naval_unit", "galley", "units", 1, func(f *recruitShipFixture) string { return fmt.Sprintf("NEW.settlement_id='%s'::uuid", f.settlementID) }},
		{"land_unit", "spearman", "units", 1, func(f *recruitShipFixture) string { return fmt.Sprintf("NEW.settlement_id='%s'::uuid", f.settlementID) }},
		{"naval_job", "galley", "scheduled_events", 1, func(f *recruitShipFixture) string {
			return fmt.Sprintf("NEW.payload->>'settlement_id'='%s' AND NEW.event_type='TrainComplete'", f.settlementID)
		}},
		{"land_job", "spearman", "scheduled_events", 1, func(f *recruitShipFixture) string {
			return fmt.Sprintf("NEW.payload->>'settlement_id'='%s' AND NEW.event_type='TrainComplete'", f.settlementID)
		}},
		{"naval_event", "galley", "events", 1, func(f *recruitShipFixture) string {
			return fmt.Sprintf("NEW.world_id='%s'::uuid AND NEW.event_type='UnitFormed'", f.worldID)
		}},
		{"second_vessel", "galley", "units", 2, func(f *recruitShipFixture) string {
			return fmt.Sprintf("NEW.settlement_id='%s'::uuid AND NEW.ordinal=2", f.settlementID)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			sink := observeRecruitEvents(t, f)
			before := snapshotRecruitState(t, f)
			rejectRecruitWrite(t, f, tc.table, tc.condition(f))
			rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": tc.unit, "count": tc.count})
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("injected failure got %d %s; want 500", rec.Code, rec.Body.String())
			}
			if sink.count != 0 {
				t.Errorf("rolled back recruit published %d events", sink.count)
			}
			after := snapshotRecruitState(t, f)
			if !reflect.DeepEqual(before, after) {
				t.Errorf("failed recruit changed state: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRecruit_ConcurrentRequestsRecheckPopulation(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=150 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}

	counts := concurrentRecruitCodes(t, f, map[string]any{"unit_type": "spearman"})
	if counts[http.StatusCreated] != 1 || counts[http.StatusUnprocessableEntity] != 1 {
		t.Fatalf("codes=%v; want one201 and one422", counts)
	}

	s := snapshotRecruitState(t, f)
	if s.Population != 50 || s.Units != 1 || s.Goods["grain"] != 988 || s.Goods["silver"] != 980 {
		t.Fatalf("concurrent recruit state: %+v", s)
	}
}

func concurrentRecruitCodes(t *testing.T, f *recruitShipFixture, body map[string]any) map[int]int {
	t.Helper()
	ctx := context.Background()
	// Hold the population row until BOTH requests reach a DB lock. On the old
	// handler both have passed the stale precheck; on the fixed handler both
	// wait for the authoritative locked read. No timing-based race assumption.
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err := lock.Exec(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			rec, _ := f.post(t, recruitPath(f), body)
			codes <- rec.Code
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	var waiting int
	for time.Now().Before(deadline) {
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if waiting < 2 {
		t.Error("did not observe both requests waiting on DB locks")
	}
	counts := map[int]int{}
	for i := 0; i < 2; i++ {
		select {
		case code := <-codes:
			counts[code]++
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent recruit did not finish")
		}
	}
	return counts
}

func TestRecruit_ConcurrentRequestsRecheckShipQueue(t *testing.T) {
	f := setupRecruitShipFixture(t)
	counts := concurrentRecruitCodes(t, f, map[string]any{"unit_type": "galley", "count": 6})
	if counts[http.StatusCreated] != 1 || counts[http.StatusUnprocessableEntity] != 1 {
		t.Fatalf("codes=%v; want one201 and one422", counts)
	}
	s := snapshotRecruitState(t, f)
	if s.Units != 6 || s.Jobs != 6 || s.Events != 6 || s.Population != 4880 || s.Goods["timber"] != 820 || s.Goods["silver"] != 964 {
		t.Fatalf("queue capacity/payment state: %+v", s)
	}
}

type recruitEventObserver struct {
	t     *testing.T
	f     *recruitShipFixture
	count int
}

func (s *recruitEventObserver) Record(ctx context.Context, e events.SinkEvent) {
	s.count++
	// A separate pool connection must already see the event and unit; sinks
	// cannot leak a UnitFormed before its transaction commits.
	var visible bool
	if err := s.f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events e JOIN units u ON u.id=e.stream_id WHERE e.id=$1)`, e.ID).Scan(&visible); err != nil {
		s.t.Error(err)
	}
	if !visible {
		s.t.Error("sink saw an uncommitted event/unit")
	}
}
func observeRecruitEvents(t *testing.T, f *recruitShipFixture) *recruitEventObserver {
	t.Helper()
	sink := &recruitEventObserver{t: t, f: f}
	clk := clock.NewTestClock(time.Now())
	store := events.NewStore(f.pool, sink)
	ph := NewProvinceHandler(f.pool, events.NewScheduler(f.pool, clk), clk, economy.SitosConfig{}, store, nil)
	router := chi.NewRouter()
	router.Use(auth.Middleware(auth.NewService(f.pool, "test-secret")))
	router.Post("/worlds/{worldID}/provinces/{provinceID}/recruit", ph.Recruit)
	f.router = router
	return sink
}
