package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// "A new Wanax has entered the Thalassa": the first founding is told to every
// other Wanax — name and the founder's own words, never the place.

type enteredFixture struct {
	pool    *pgxpool.Pool
	world   uuid.UUID
	founder uuid.UUID
	others  []uuid.UUID
	send    func(path, body string) *httptest.ResponseRecorder
}

func newEnteredFixture(t *testing.T) *enteredFixture {
	t.Helper()
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	svc := auth.NewService(pool, "entered-test")
	hub := notify.New()
	hub.SetPool(pool)
	h := NewJoinHandler(pool, events.NewStore(pool), clock.NewTestClock(time.Now()), hub)
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Post("/worlds/{worldID}/join", h.Join)
	r.Post("/worlds/{worldID}/founding/settle", h.Settle)
	f := &enteredFixture{pool: pool, world: wid}
	tokens := map[uuid.UUID]string{}
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		pid, tok := registerViewer(t, ctx, svc, "entered")
		tokens[pid] = tok
		ids = append(ids, pid)
	}
	f.founder, f.others = ids[0], ids[1:]
	cur := f.founder
	f.send = func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+wid.String()+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokens[cur])
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	// The fixture world has room for one Host: only the founder joins; the
	// others are Wanaxes of the world by their record alone.
	if rec := f.send("/join", `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("join: %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range f.others {
		if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id, world_id, status, kharis_amount, kharis_rate, kharis_calc_tick)
			VALUES($1,$2,'active',50,0,0)`, id, wid); err != nil {
			t.Fatal(err)
		}
	}
	cur = f.founder
	return f
}

func (f *enteredFixture) notices(t *testing.T, player uuid.UUID) []map[string]any {
	t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT body_json FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='WanaxEntered'`, f.world, player)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestWanaxEntered_FirstFoundingToldToAllOthersWithoutPlace(t *testing.T) {
	f := newEnteredFixture(t)
	if rec := f.send("/founding/settle", `{"name":"Secretopolis","message":"  Hear me, all of you.  "}`); rec.Code != http.StatusCreated {
		t.Fatalf("settle: %d %s", rec.Code, rec.Body.String())
	}
	if n := f.notices(t, f.founder); len(n) != 0 {
		t.Fatalf("founder got %d copies, want none", len(n))
	}
	for _, o := range f.others {
		n := f.notices(t, o)
		if len(n) != 1 {
			t.Fatalf("recipient got %d notices, want 1", len(n))
		}
		keys := []string{}
		for k := range n[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if strings.Join(keys, ",") != "message,wanax_id,wanax_name" {
			t.Fatalf("payload keys %v: the place must never be in it", keys)
		}
		if n[0]["message"] != "Hear me, all of you." {
			t.Fatalf("message %q", n[0]["message"])
		}
		raw, _ := json.Marshal(n[0])
		if strings.Contains(string(raw), "Secretopolis") || strings.Contains(string(raw), `"q"`) || strings.Contains(string(raw), `"r"`) {
			t.Fatalf("payload leaks the place: %s", raw)
		}
	}
	// The event leaks nothing either.
	var payload string
	if err := f.pool.QueryRow(context.Background(), `SELECT payload::text FROM events WHERE world_id=$1 AND event_type='WanaxEntered'`, f.world).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "Secretopolis") || strings.Contains(payload, `"q"`) {
		t.Fatalf("event leaks the place: %s", payload)
	}
}

func TestWanaxEntered_NotToldForAReturningWanax(t *testing.T) {
	f := newEnteredFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE founder_phase SET is_refugee=true WHERE world_id=$1 AND owner_id=$2`, f.world, f.founder); err != nil {
		t.Fatal(err)
	}
	if rec := f.send("/founding/settle", `{"message":"again"}`); rec.Code != http.StatusCreated {
		t.Fatalf("settle: %d %s", rec.Code, rec.Body.String())
	}
	for _, o := range f.others {
		if n := f.notices(t, o); len(n) != 0 {
			t.Fatalf("a returning Wanax was announced: %v", n)
		}
	}
}

func TestWanaxEntered_AnnouncingTwiceMakesNoDuplicates(t *testing.T) {
	f := newEnteredFixture(t)
	if rec := f.send("/founding/settle", `{"message":"once"}`); rec.Code != http.StatusCreated {
		t.Fatalf("settle: %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	again, err := announceWanaxEntered(ctx, tx, events.NewStore(f.pool), f.world, f.founder, "once")
	if err != nil {
		t.Fatal(err)
	}
	if len(again.notices) != 0 {
		t.Fatalf("second announcement wrote %d notices", len(again.notices))
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, o := range f.others {
		if n := f.notices(t, o); len(n) != 1 {
			t.Fatalf("recipient has %d notices after a repeat, want 1", len(n))
		}
	}
}

func TestWanaxEntered_MessageLimitAndCoarseWords(t *testing.T) {
	f := newEnteredFixture(t)
	if rec := f.send("/founding/settle", `{"message":"`+strings.Repeat("a", 281)+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("281 chars: %d, want 400", rec.Code)
	}
	var cities int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM settlements WHERE world_id=$1 AND owner_id=$2`, f.world, f.founder).Scan(&cities); err != nil || cities != 0 {
		t.Fatalf("a refused message must not found the city: %d %v", cities, err)
	}
	if rec := f.send("/founding/settle", `{"message":"`+strings.Repeat("a", 280)+` fuck"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("over the limit with a coarse word: %d, want 400", rec.Code)
	}
	if rec := f.send("/founding/settle", `{"message":"Fuck you all"}`); rec.Code != http.StatusCreated {
		t.Fatalf("settle: %d %s", rec.Code, rec.Body.String())
	}
	n := f.notices(t, f.others[0])
	if len(n) != 1 || strings.Contains(strings.ToLower(n[0]["message"].(string)), "fuck") {
		t.Fatalf("coarse word reached the recipient: %v", n)
	}
	var angered int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='GodsAngered'`, f.world, f.founder).Scan(&angered); err != nil || angered != 1 {
		t.Fatalf("GodsAngered for the founder: %d %v, want 1", angered, err)
	}
}

func TestWanaxEntered_EmptyMessageStillAnnounces(t *testing.T) {
	f := newEnteredFixture(t)
	if rec := f.send("/founding/settle", `{}`); rec.Code != http.StatusCreated {
		t.Fatalf("settle: %d %s", rec.Code, rec.Body.String())
	}
	n := f.notices(t, f.others[0])
	if len(n) != 1 || n[0]["message"] != "" {
		t.Fatalf("empty message: %v", n)
	}
}
