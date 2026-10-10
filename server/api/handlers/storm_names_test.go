package handlers

// Stormnamn (megaron_plan_stormnamn.md): the first Wanax to meet a storm names it, once; the
// name rides with the storm; admin can rename or clear it; FOW is untouched.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type nameRig struct {
	t       *testing.T
	pool    *pgxpool.Pool
	worldID uuid.UUID
	viewer  uuid.UUID
	token   string
	other   string
	router  *chi.Mux
	get     func() stormsResp
}

func newNameRig(t *testing.T) (*nameRig, uuid.UUID) {
	pool, worldID, viewerID, token, get := stormsRig(t)
	authSvc := auth.NewService(pool, "test-secret")
	_, otherToken := registerViewer(t, context.Background(), authSvc, "storm-other")
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Post("/worlds/{worldID}/storms/{stormID}/name", wh.NameStorm)
	r.Put("/admin/worlds/{worldID}/storms/{stormID}/name", wh.AdminNameStorm)
	r.Delete("/admin/worlds/{worldID}/storms/{stormID}/name", wh.AdminClearStormName)
	t.Setenv("POLEIA_ADMIN_KEY", "k")
	storm := seedStorm(t, pool, worldID, 1, [3][2]int{{2, 0}, {3, 0}, {2, 1}})
	if _, err := pool.Exec(context.Background(), `UPDATE sea_storms SET claimed_by = $2, claimed_tick = 5 WHERE id = $1`, storm, viewerID); err != nil {
		t.Fatal(err)
	}
	return &nameRig{t: t, pool: pool, worldID: worldID, viewer: viewerID, token: token, other: otherToken, router: r, get: get}, storm
}

func (n *nameRig) do(method string, storm uuid.UUID, body, token, admin string) int {
	prefix := "/worlds/"
	if admin != "" {
		prefix = "/admin/worlds/"
	}
	req := httptest.NewRequest(method, prefix+n.worldID.String()+"/storms/"+storm.String()+"/name", bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if admin != "" {
		req.Header.Set("X-Admin-Key", admin)
	}
	rec := httptest.NewRecorder()
	n.router.ServeHTTP(rec, req)
	return rec.Code
}

func TestNameStorm_OnlyTheFirstToMeetMayNameItOnce(t *testing.T) {
	n, storm := newNameRig(t)
	if got := n.get().Storms[0]; !got.CanName || got.Name != "" {
		t.Fatalf("before naming: %+v, want can_name and no name", got)
	}
	if c := n.do("POST", storm, `{"name":"Loud Tide"}`, n.other, ""); c != http.StatusForbidden {
		t.Fatalf("another Wanax naming = %d, want 403", c)
	}
	if c := n.do("POST", storm, `{"name":"  Poseidon's   Wrath "}`, n.token, ""); c != http.StatusOK {
		t.Fatalf("the first to meet naming = %d, want 200", c)
	}
	got := n.get().Storms[0]
	if got.Name != "Poseidon's Wrath" || got.CanName {
		t.Fatalf("after naming: %+v, want the trimmed name and no further right", got)
	}
	if c := n.do("POST", storm, `{"name":"Second Try"}`, n.token, ""); c != http.StatusConflict {
		t.Fatalf("naming twice = %d, want 409", c)
	}
}

func TestNameStorm_RejectsBadNamesAndDuplicates(t *testing.T) {
	n, storm := newNameRig(t)
	for _, bad := range []string{`{"name":"x"}`, `{"name":"` + strings.Repeat("a", 31) + `"}`, `{"name":"<b>hej</b>"}`, `{"name":"  "}`, `nope`} {
		if c := n.do("POST", storm, bad, n.token, ""); c != http.StatusBadRequest {
			t.Fatalf("name %q = %d, want 400", bad, c)
		}
	}
	other := seedStorm(t, n.pool, n.worldID, 0, [3][2]int{{30, 0}, {31, 0}, {30, 1}})
	if _, err := n.pool.Exec(context.Background(), `UPDATE sea_storms SET name = 'Skyla', named_tick = 3 WHERE id = $1`, other); err != nil {
		t.Fatal(err)
	}
	if c := n.do("POST", storm, `{"name":"skyla"}`, n.token, ""); c != http.StatusConflict {
		t.Fatalf("a taken name (any case) = %d, want 409", c)
	}
}

func TestNameStorm_AdminCanRenameClearAndTheRightStaysSpent(t *testing.T) {
	n, storm := newNameRig(t)
	if c := n.do("POST", storm, `{"name":"Foul Word"}`, n.token, ""); c != http.StatusOK {
		t.Fatal(c)
	}
	if c := n.do("PUT", storm, `{"name":"Calm Mother"}`, "", "wrong"); c != http.StatusForbidden {
		t.Fatalf("admin without the key = %d, want 403", c)
	}
	if c := n.do("PUT", storm, `{"name":"Calm Mother"}`, "", "k"); c != http.StatusOK {
		t.Fatalf("admin rename = %d, want 200", c)
	}
	if got := n.get().Storms[0].Name; got != "Calm Mother" {
		t.Fatalf("name = %q, want Calm Mother", got)
	}
	if c := n.do("DELETE", storm, ``, "", "k"); c != http.StatusNoContent {
		t.Fatalf("admin clear = %d, want 204", c)
	}
	got := n.get().Storms[0]
	if got.Name != "" || got.CanName {
		t.Fatalf("after clear: %+v, want unnamed and the namer's right spent", got)
	}
	if c := n.do("POST", storm, `{"name":"Again"}`, n.token, ""); c != http.StatusConflict {
		t.Fatalf("namer retrying after a clear = %d, want 409", c)
	}
}

// A name never reveals a storm: out of sight and unremembered, it is not in the list at all.
func TestNameStorm_NameDoesNotLeakAStormOutOfSight(t *testing.T) {
	n, _ := newNameRig(t)
	far := seedStorm(t, n.pool, n.worldID, 0, [3][2]int{{40, 0}, {41, 0}, {40, 1}})
	if _, err := n.pool.Exec(context.Background(), `UPDATE sea_storms SET name = 'Hidden Maw', named_tick = 1 WHERE id = $1`, far); err != nil {
		t.Fatal(err)
	}
	for _, s := range n.get().Storms {
		if s.ID == far || s.Name == "Hidden Maw" {
			t.Fatalf("far named storm leaked: %+v", s)
		}
	}
}
