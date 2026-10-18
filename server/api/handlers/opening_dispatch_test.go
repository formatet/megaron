package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestOpeningDispatchJoinConcurrentRetryRiseAndPrivacy(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	svc := auth.NewService(pool, "opening-test")
	pid, token := registerViewer(t, ctx, svc, "opening-"+uuid.NewString())
	_, otherToken := registerViewer(t, ctx, svc, "opening-other-"+uuid.NewString())
	h := NewJoinHandler(pool, events.NewStore(pool), clock.NewTestClock(time.Now()), nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Post("/worlds/{worldID}/join", h.Join)
	r.Post("/worlds/{worldID}/rise", h.Rise)
	r.Get("/worlds/{worldID}/notifications", NewNotificationsHandler(pool).List)
	request := func(method, path, tk string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/worlds/"+wid.String()+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tk)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	var responses [2]*httptest.ResponseRecorder
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = request("POST", "/join", token) }(i)
	}
	wg.Wait()
	created, existing := 0, 0
	for _, rec := range responses {
		switch rec.Code {
		case 201:
			created++
		case 200:
			existing++
		default:
			t.Fatalf("join%d %s", rec.Code, rec.Body.String())
		}
	}
	if created != 1 || existing != 1 {
		t.Fatalf("concurrent join created%d existing%d", created, existing)
	}
	read := func(tk string) []openingDispatchBody {
		rec := request("GET", "/notifications", tk)
		if rec.Code != 200 {
			t.Fatalf("archive%d %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Notifications []struct {
				Kind string              `json:"kind"`
				Body openingDispatchBody `json:"body"`
			} `json:"notifications"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		out := []openingDispatchBody{}
		for _, n := range payload.Notifications {
			if n.Kind == openingDispatchKind {
				out = append(out, n.Body)
			}
		}
		return out
	}
	opening := read(token)
	if len(opening) != 1 || !strings.Contains(strings.Join(opening[0].Paragraphs, " "), fmt.Sprintf("starts at %g Kharis", kharis.StartingKharis)) {
		t.Fatalf("archived join dispatch must describe the current starting standing: %+v", opening)
	}
	if len(opening) != 1 || opening[0].Returning || len(opening[0].Paragraphs) != 9 {
		t.Fatalf("join notice %+v", opening)
	}
	if len(read(otherToken)) != 0 {
		t.Fatal("another Wanax received opening dispatch")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,20,20,'plains')`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET state='active' WHERE id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	if rec := request("POST", "/join", otherToken); rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	active := read(otherToken)
	if len(active) != 1 || strings.Contains(strings.Join(active[0].Paragraphs, " "), "Days begin after") {
		t.Fatalf("active-world join still waits: %+v", active)
	}
	if rec := request("POST", "/join", token); rec.Code != 200 {
		t.Fatalf("retry%d", rec.Code)
	}
	if len(read(token)) != 1 {
		t.Fatal("retry duplicated opening")
	}
	// Simulated loss state is explicit; the actual Rise HTTP transaction is used.
	if _, err := pool.Exec(ctx, `UPDATE founder_phase SET active=false WHERE world_id=$1 AND owner_id=$2`, wid, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE player_world_records SET status='dispossessed',kharis_amount=15,kharis_rate=0 WHERE world_id=$1 AND player_id=$2`, wid, pid); err != nil {
		t.Fatal(err)
	}
	if rec := request("POST", "/rise", token); rec.Code != 201 {
		t.Fatalf("rise%d %s", rec.Code, rec.Body.String())
	}
	if rec := request("POST", "/rise", token); rec.Code != 200 {
		t.Fatalf("rise retry%d", rec.Code)
	}
	all := read(token)
	if len(all) != 2 {
		t.Fatalf("two lives have%d opening notices", len(all))
	}
	returning := 0
	for _, n := range all {
		if n.Returning {
			returning++
			if n.Title != "Begin again" || n.HostID == opening[0].HostID || !strings.Contains(strings.Join(n.Paragraphs, " "), "60 silver") {
				t.Fatalf("rise notice %+v", n)
			}
		}
	}
	if returning != 1 {
		t.Fatalf("returning notices%d", returning)
	}
}

func TestOpeningDispatchFactsFromActiveRules(t *testing.T) {
	h := &JoinHandler{startWanaxes: 3}
	old := tick.TickSeconds
	t.Cleanup(func() { tick.TickSeconds = old })
	for _, tc := range []struct {
		seconds int
		label   string
	}{{1, "1 second"}, {60, "1 minute"}, {1800, "30 minutes"}, {3600, "1 hour"}} {
		tick.TickSeconds = tc.seconds
		body := h.openingDispatch(uuid.New(), newcomerHost, true)
		text := strings.Join(body.Paragraphs, "\n")
		for _, want := range []string{tc.label, "3 Wanaxes", fmt.Sprintf("%g grain", float64(nomadicHostDowryGrain)), fmt.Sprintf("%g silver", nomadicHostStartingSilver), fmt.Sprintf("starts at %g Kharis", kharis.StartingKharis), fmt.Sprintf("temple takes %d days", province.BuildingSpecs[province.BuildingTemple].DurationTicks), fmt.Sprintf("below %d people", economy.CollapseBelowPopulation), fmt.Sprintf("olive woodland in %g", province.TerrainMoveTicks("forest_olive_grove")), "reserves do not drain"} {
			if !strings.Contains(text, want) {
				t.Fatalf("opening missing%q: %s", want, text)
			}
		}
		if strings.Contains(text, "144") || strings.Contains(text, "grace") || strings.Contains(text, "no punishment") || strings.Contains(text, "tick") || strings.Contains(text, "pay for") || len(strings.Fields(text)) > 250 {
			t.Fatalf("wrong/too long opening %s", text)
		}
	}
	t.Logf("actual starting kharis %g, temple duration%d", kharis.StartingKharis, province.BuildingSpecs[province.BuildingTemple].DurationTicks)
}

func TestOpeningDispatchArchiveFailureRollsBackHost(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	svc := auth.NewService(pool, "opening-rollback")
	pid, token := registerViewer(t, ctx, svc, "opening-rollback-"+uuid.NewString())
	h := NewJoinHandler(pool, events.NewStore(pool), clock.NewTestClock(time.Now()), nil)
	r := chi.NewRouter()
	r.With(auth.Middleware(svc)).Post("/worlds/{worldID}/join", h.Join)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_opening_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='OpeningDispatch' THEN RAISE EXCEPTION 'reject opening'; END IF; RETURN NEW; END $$;
	 CREATE TRIGGER reject_opening_notice BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION reject_opening_notice()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_opening_notice ON notifications; DROP FUNCTION IF EXISTS reject_opening_notice()`)
	})
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+wid.String()+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(); rec.Code != 500 {
		t.Fatalf("failed archive join%d", rec.Code)
	}
	var hosts, units, records int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM founder_phase WHERE world_id=$1 AND owner_id=$2),(SELECT count(*) FROM units WHERE world_id=$1 AND owner_id=$2),(SELECT count(*) FROM player_world_records WHERE world_id=$1 AND player_id=$2)`, wid, pid).Scan(&hosts, &units, &records); err != nil {
		t.Fatal(err)
	}
	if hosts+units+records != 0 {
		t.Fatalf("failed notice left host%d units%d records%d", hosts, units, records)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_opening_notice ON notifications; DROP FUNCTION reject_opening_notice()`); err != nil {
		t.Fatal(err)
	}
	if rec := post(); rec.Code != 201 {
		t.Fatalf("restored archive retry%d %s", rec.Code, rec.Body.String())
	}
}

func TestOpeningDispatch_ActiveWorldDoesNotWaitForJoiners(t *testing.T) {
	h := &JoinHandler{startWanaxes: 4}
	body := h.openingDispatch(uuid.New(), newcomerHost, false)
	text := strings.Join(body.Paragraphs, " ")
	if strings.Contains(text, "Days begin after") || !strings.Contains(text, "Place citizens") || !strings.Contains(text, "stone") {
		t.Fatal(text)
	}
}
