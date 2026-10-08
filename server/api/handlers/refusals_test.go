package handlers

// Avslagsloggen (megaron_plan_avslagslogg.md): a refused write is recorded with
// the chi route pattern, status and code; reads, successes and 401s are not.

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func refusalRows(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID) []struct {
	route, method, code string
	status              int
} {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT route, method, status, code FROM refusals WHERE world_id = $1 ORDER BY id`, worldID)
	if err != nil {
		t.Fatalf("read refusals: %v", err)
	}
	defer rows.Close()
	var out []struct {
		route, method, code string
		status              int
	}
	for rows.Next() {
		var r struct {
			route, method, code string
			status              int
		}
		if err := rows.Scan(&r.route, &r.method, &r.status, &r.code); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestRecordRefusals(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	authSvc := auth.NewService(pool, "test-secret")
	_, tok := registerViewer(t, ctx, authSvc, "refusal-a")

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(authSvc))
			r.Use(RecordRefusals(pool))
			r.Use(RequireStartedWorld(pool, 99)) // world stays forming → verbs refused 409
			r.Post("/worlds/{worldID}/units/{unitID}/stance", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			r.Post("/worlds/{worldID}/reports", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusCreated, map[string]string{"ok": "1"})
			})
			r.Get("/worlds/{worldID}/units", func(w http.ResponseWriter, _ *http.Request) {
				writeError(w, http.StatusNotFound, "a read refusal is not a verb refusal")
			})
			r.Delete("/worlds/{worldID}/build-queue/{queueID}", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "long prose", "error_code": "insufficient_goods"})
			})
		})
	})
	base := "/api/v1/worlds/" + worldID.String()

	// Refused by the world guard: recorded with the pattern, not the concrete URL.
	if rec := gateDo(t, r, http.MethodPost, base+"/units/"+uuid.New().String()+"/stance", tok, `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("stance = %d, want 409", rec.Code)
	}
	// Success, read and unauthenticated: never recorded.
	gateDo(t, r, http.MethodPost, base+"/reports", tok, `{}`)
	gateDo(t, r, http.MethodGet, base+"/units", tok, "")
	gateDo(t, r, http.MethodPost, base+"/units/"+uuid.New().String()+"/stance", "", `{}`)

	got := refusalRows(t, pool, worldID)
	if len(got) != 1 {
		t.Fatalf("refusals = %+v, want exactly the one stance refusal", got)
	}
	if got[0].route != "/api/v1/worlds/{worldID}/units/{unitID}/stance" || got[0].method != "POST" || got[0].status != 409 {
		t.Fatalf("refusal = %+v", got[0])
	}
	if got[0].code == "" || got[0].code == "(no error text)" {
		t.Fatalf("code = %q, want the guard's error text", got[0].code)
	}
}

func TestRefusalCodePrefersErrorCode(t *testing.T) {
	if c := refusalCode([]byte(`{"error":"long prose","error_code":"insufficient_goods"}`)); c != "insufficient_goods" {
		t.Fatalf("code = %q", c)
	}
	long := make([]byte, 0, 400)
	for i := 0; i < 300; i++ {
		long = append(long, 'x')
	}
	if c := refusalCode([]byte(`{"error":"` + string(long) + `"}`)); len([]rune(c)) != 200 {
		t.Fatalf("code length = %d, want 200", len([]rune(c)))
	}
}
