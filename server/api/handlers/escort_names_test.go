package handlers

// DB integration test (real Postgres, gated by DATABASE_URL) for the escort's
// names before founding. Timothy 2026-09-25, playing: a new Wanax saw two
// identical "Spearmen" and could not tell them apart. Namnstandarden
// (unit/naming.go) had the "of <stad>" form but nothing for a unit with no
// support settlement, and seedNomadicHost gave the escort no ordinal — so both
// rendered as the bare type. Timothy: "at start they are called 'First
// Spearmen of WANAX_NAME'".

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// TestJoin_EscortCohortsNamedAfterWanax: after join, GET /units shows the two
// escort cohorts as "1st Spearmen of <Wanax>" and "2nd Spearmen of <Wanax>",
// and a notification (unit.LoadDisplayName) says the same string. After
// founding, the city's name takes over and 1st stays 1st.
func TestJoin_EscortCohortsNamedAfterWanax(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	playerID, token := registerViewer(t, ctx, authSvc, "escort-names")
	clk := clock.NewTestClock(time.Now())

	r := joinCultureRouter(pool, authSvc, clk)
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r.Get("/worlds/{worldID}/units", uh.ListUnits)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/worlds/"+worldID.String()+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(http.MethodPost, "/join", "{}"); rec.Code != http.StatusCreated {
		t.Fatalf("POST /join = %d %q, want 201", rec.Code, rec.Body.String())
	}

	var wanax string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(wanax_name, username) FROM players WHERE id = $1`, playerID,
	).Scan(&wanax); err != nil {
		t.Fatalf("read wanax name: %v", err)
	}

	type listed struct {
		ID          uuid.UUID `json:"id"`
		Type        string    `json:"type"`
		DisplayName string    `json:"display_name"`
	}
	spearmen := func() map[uuid.UUID]string {
		t.Helper()
		rec := do(http.MethodGet, "/units", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /units = %d %q", rec.Code, rec.Body.String())
		}
		var resp struct {
			Units []listed `json:"units"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /units: %v", err)
		}
		out := map[uuid.UUID]string{}
		for _, u := range resp.Units {
			if u.Type == string(unit.TypeSpearman) {
				out[u.ID] = u.DisplayName
			}
		}
		return out
	}

	before := spearmen()
	if len(before) != 2 {
		t.Fatalf("escort after join: %d spearman cohorts, want 2: %v", len(before), before)
	}
	want := map[string]bool{
		"1st Spearmen of " + wanax: false,
		"2nd Spearmen of " + wanax: false,
	}
	firstID := uuid.Nil
	for id, name := range before {
		seen, ok := want[name]
		if !ok || seen {
			t.Errorf("escort cohort display_name = %q, want one each of %v", name, want)
			continue
		}
		want[name] = true
		if strings.HasPrefix(name, "1st ") {
			firstID = id
		}
		// The notification surface must say exactly what the Army card says.
		if got := unit.LoadDisplayName(ctx, pool, id); got != name {
			t.Errorf("LoadDisplayName = %q, /units display_name = %q — the two surfaces disagree", got, name)
		}
	}
	if firstID == uuid.Nil {
		t.Fatalf("no \"1st\" cohort among %v", before)
	}

	// After founding the city names the cohort, and the 1st keeps its number.
	city := "Escortopolis-" + uuid.New().String()[:8]
	if rec := do(http.MethodPost, "/founding/settle", `{"name":"`+city+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("POST /founding/settle = %d %q, want 201", rec.Code, rec.Body.String())
	}
	after := spearmen()
	if got, want := after[firstID], "1st Spearmen of "+city; got != want {
		t.Errorf("after founding, the former 1st = %q, want %q (all: %v)", got, want, after)
	}
}
