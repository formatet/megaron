package handlers

// Regression test for the player_reports.context column (mig 123,
// temenos_buggrapporter.md "Nån smart input till buggrapporteringen"): the
// report drawer used to hand back only kind/body/q/r/view, so tracing a
// report to the exact entity it was about meant reverse-engineering it from
// timing and position (see the 2026-08-08 FOW investigation, done the hard
// way). context is a free-form JSON blob the client fills with whatever
// State it has on hand for the currently-open panel — this test only proves
// the plumbing: what Create is sent round-trips through List unchanged, and
// omitting context entirely still works (nullable, no client changes forced
// on an old build).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestPlayerReports_ContextRoundTrips(t *testing.T) {
	pool := riteTestPool(t) // shared DB-integration helper, settlement_rite_offering_test.go
	ctx := context.Background()

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
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "reports-context-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}

	t.Setenv("POLEIA_ADMIN_KEY", "test-admin-key")

	rh := NewReportsHandler(pool)
	r := chi.NewRouter()
	// Mirrors main.go: List is mounted unauthenticated (X-Admin-Key gates it
	// itself, requireAdminKey), Create sits behind the normal player Bearer
	// auth. Wrapping both in auth.Middleware would 401 List before it ever
	// reached requireAdminKey.
	r.Get("/admin/worlds/{worldID}/reports", rh.List)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(authSvc))
		r.Post("/worlds/{worldID}/reports", rh.Create)
	})

	post := func(t *testing.T, body map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/reports", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("Create = %d: %s", rec.Code, rec.Body.String())
		}
	}

	// One report with a context blob (what the report drawer will actually
	// send — a march-ctx unit id, say), one without (an old client, or a view
	// with nothing to attach).
	post(t, map[string]any{
		"kind": "bug", "body": "fow did not lift",
		"context": map[string]any{"march_ctx_dest": map[string]any{"q": 5, "r": 60}, "unit_ids": []string{"11111111-1111-1111-1111-111111111111"}},
	})
	post(t, map[string]any{"kind": "confused", "body": "no context here"})

	req := httptest.NewRequest(http.MethodGet, "/admin/worlds/"+worldID.String()+"/reports", nil)
	req.Header.Set("X-Admin-Key", "test-admin-key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List = %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Reports []struct {
			Body    string          `json:"body"`
			Context json.RawMessage `json:"context"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse List response: %v", err)
	}
	if len(resp.Reports) != 2 {
		t.Fatalf("reports = %d, want 2", len(resp.Reports))
	}

	// List orders by created_at DESC, so the second post ("no context here")
	// comes first.
	noContext, withContext := resp.Reports[0], resp.Reports[1]
	if noContext.Body != "no context here" {
		t.Fatalf("reports[0].body = %q, want %q", noContext.Body, "no context here")
	}
	if len(noContext.Context) != 0 {
		t.Errorf("reports[0].context = %s, want empty (no context was sent)", noContext.Context)
	}

	if withContext.Body != "fow did not lift" {
		t.Fatalf("reports[1].body = %q, want %q", withContext.Body, "fow did not lift")
	}
	var gotContext struct {
		MarchCtxDest struct {
			Q, R int
		} `json:"march_ctx_dest"`
		UnitIDs []string `json:"unit_ids"`
	}
	if err := json.Unmarshal(withContext.Context, &gotContext); err != nil {
		t.Fatalf("unmarshal reports[1].context: %v (raw: %s)", err, withContext.Context)
	}
	if gotContext.MarchCtxDest.Q != 5 || gotContext.MarchCtxDest.R != 60 {
		t.Errorf("march_ctx_dest = (%d,%d), want (5,60)", gotContext.MarchCtxDest.Q, gotContext.MarchCtxDest.R)
	}
	if len(gotContext.UnitIDs) != 1 || gotContext.UnitIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("unit_ids = %v, want [11111111-1111-1111-1111-111111111111]", gotContext.UnitIDs)
	}
}
