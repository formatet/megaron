package handlers

// The report log (Timothy 2026-10-04): every report is also appended to
// REPORTS_DIR/reports.jsonl, outside the database, and the DB copy no longer
// hangs on worlds — a reseed (TRUNCATE worlds CASCADE) used to empty
// player_reports wholesale (2026-09-09).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestPlayerReports_AppendedToLogOutsideDB(t *testing.T) {
	pool := riteTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	worldName := "test-world-" + uuid.New().String()
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`, worldName,
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "reports-log-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "reports") // does not exist yet: Create must make it
	rh := NewReportsHandler(pool)
	rh.SetLogDir(dir)
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(authSvc))
		r.Post("/worlds/{worldID}/reports", rh.Create)
	})

	post := func(body map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/reports", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("Create = %d: %s", rec.Code, rec.Body.String())
		}
	}
	post(map[string]any{"kind": "bug", "body": "walls vanished", "q": 3, "r": 4, "view": "city",
		"context": map[string]any{"unit_id": "u1"}})
	post(map[string]any{"kind": "confused", "body": "what is kharis"})

	f, err := os.Open(filepath.Join(dir, "reports.jsonl"))
	if err != nil {
		t.Fatalf("open report log: %v", err)
	}
	defer f.Close()
	var lines []reportLogLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l reportLogLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("log line is not JSON: %v (%s)", err, sc.Text())
		}
		lines = append(lines, l)
	}
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want 2 (one per report, appended)", len(lines))
	}
	first := lines[0]
	if first.Body != "walls vanished" || first.Kind != "bug" || first.View != "city" {
		t.Errorf("line 0 = %+v, want the first report", first)
	}
	if first.Q == nil || *first.Q != 3 || first.R == nil || *first.R != 4 {
		t.Errorf("line 0 hex = %v,%v, want 3,4", first.Q, first.R)
	}
	// Names resolved at write time — the world row will be gone after a reseed.
	if first.WorldName != worldName || first.WorldID != worldID {
		t.Errorf("line 0 world = %q/%s, want %q/%s", first.WorldName, first.WorldID, worldName, worldID)
	}
	if first.Username != username || first.ID == uuid.Nil || first.CreatedAt.IsZero() {
		t.Errorf("line 0 identity = user %q id %s at %v", first.Username, first.ID, first.CreatedAt)
	}
	if string(first.Context) != `{"unit_id":"u1"}` {
		t.Errorf("line 0 context = %s", first.Context)
	}
	if lines[1].Body != "what is kharis" || len(lines[1].Context) != 0 {
		t.Errorf("line 1 = %+v, want the second report without context", lines[1])
	}
}

// TRUNCATE ... CASCADE follows foreign keys whatever their ON DELETE action,
// so the only way player_reports survives `TRUNCATE worlds CASCADE` is to
// have no key to worlds at all (mig 157).
func TestPlayerReports_NoForeignKeyToWorlds(t *testing.T) {
	pool := riteTestPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_constraint
		 WHERE contype = 'f' AND conrelid = 'player_reports'::regclass AND confrelid = 'worlds'::regclass`,
	).Scan(&n); err != nil {
		t.Fatalf("query constraints: %v", err)
	}
	if n != 0 {
		t.Fatalf("player_reports has %d foreign key(s) to worlds — a reseed would TRUNCATE the reports", n)
	}
}
