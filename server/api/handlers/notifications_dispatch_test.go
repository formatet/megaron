package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The two server halves of "arkivet matar stapeln": the Dispatches strip is
// rebuilt from the archive on load, so the archive must be able to answer
// (a) which of my unread notifications WOULD have been dispatches — a muted
// kind must not come back through the front door a reload opens — and (b)
// "this one is dealt with", per notification, so a dismissed chip stays
// dismissed. Read is not delete: the row stays in the archive, always
// (megaron_plan_dispatches.md §1, "arkivet är heligt").

type notifFixture struct {
	pool     *pgxpool.Pool
	worldID  uuid.UUID
	playerID uuid.UUID
	token    string
	router   *chi.Mux
}

func setupNotifFixture(t *testing.T) *notifFixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var worldID uuid.UUID
	// status 'archived', not the 'active' default: a partial unique index
	// (one_active_world) allows exactly one active world, and nothing here needs
	// one — notifications are rows against a world id, not a running simulation.
	// Spelling it out also keeps this test from fighting whatever world a
	// previous test in the same DB left active.
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-notif-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	token, _, err := authSvc.Register(ctx, "notif-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	nh := NewNotificationsHandler(pool)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/notifications", nh.List)
	r.Post("/worlds/{worldID}/notifications/{notifID}/read", nh.MarkRead)

	return &notifFixture{pool: pool, worldID: worldID, playerID: claims.PlayerID, token: token, router: r}
}

// archive inserts one unread notification and returns its id.
func (f *notifFixture) archive(t *testing.T, kind string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
		 VALUES ($1, $2, $3, 3, '{}') RETURNING id`,
		f.worldID, f.playerID, kind,
	).Scan(&id); err != nil {
		t.Fatalf("archive %s: %v", kind, err)
	}
	return id
}

func (f *notifFixture) do(t *testing.T, method, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func (f *notifFixture) kindsIn(resp map[string]any) []string {
	var kinds []string
	items, _ := resp["notifications"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if k, ok := m["kind"].(string); ok {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// TestListDispatchableExcludesMutedKinds: the muted kind is in the archive
// (that never changes) but must not be handed to the strip rebuild. Mutation:
// delete the dispatchable branch in List and this fails — the mute would then
// hold for live pushes and silently break on every page reload, the worst of
// both worlds because nothing errors.
func TestListDispatchableExcludesMutedKinds(t *testing.T) {
	f := setupNotifFixture(t)
	f.archive(t, "SitosGranaryRelease")
	f.archive(t, "FoodShortfall")
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO dispatch_mutes (player_id, kind) VALUES ($1, 'SitosGranaryRelease')`,
		f.playerID,
	); err != nil {
		t.Fatalf("mute: %v", err)
	}

	base := "/worlds/" + f.worldID.String() + "/notifications"
	code, resp := f.do(t, http.MethodGet, base+"?unread=true&dispatchable=true")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got := f.kindsIn(resp); len(got) != 1 || got[0] != "FoodShortfall" {
		t.Errorf("dispatchable kinds = %v, want [FoodShortfall] only", got)
	}

	// The archive itself is untouched by the mute — both rows are still there.
	code, resp = f.do(t, http.MethodGet, base)
	if code != http.StatusOK {
		t.Fatalf("archive status = %d", code)
	}
	if got := f.kindsIn(resp); len(got) != 2 {
		t.Errorf("archive kinds = %v, want both — muting must never hide a row from the archive", got)
	}
}

// TestMarkReadIsPerNotificationAndNotDelete: dismissing one chip marks one row
// read and leaves every other unread — and leaves the row itself in place.
func TestMarkReadIsPerNotificationAndNotDelete(t *testing.T) {
	f := setupNotifFixture(t)
	first := f.archive(t, "SiegeStarted")
	f.archive(t, "FoodShortfall")

	base := "/worlds/" + f.worldID.String() + "/notifications"
	code, _ := f.do(t, http.MethodPost, base+"/"+first.String()+"/read")
	if code != http.StatusNoContent {
		t.Fatalf("mark read status = %d, want 204", code)
	}

	code, resp := f.do(t, http.MethodGet, base+"?unread=true")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got := f.kindsIn(resp); len(got) != 1 || got[0] != "FoodShortfall" {
		t.Errorf("unread after one mark-read = %v, want [FoodShortfall]", got)
	}

	var total int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE world_id = $1 AND player_id = $2`,
		f.worldID, f.playerID,
	).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Errorf("rows in archive = %d, want 2 — read is not delete", total)
	}
}

// TestMarkReadCannotTouchAnotherPlayersRow: the UPDATE is scoped to the caller.
// A wrong or foreign id answers 204 (a dismissal must not fail loudly in the
// UI) but must change nothing.
func TestMarkReadCannotTouchAnotherPlayersRow(t *testing.T) {
	f := setupNotifFixture(t)

	authSvc := auth.NewService(f.pool, "test-secret")
	otherToken, _, err := authSvc.Register(context.Background(), "notif-other-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	otherClaims, err := authSvc.ValidateAccessToken(otherToken)
	if err != nil {
		t.Fatalf("validate other: %v", err)
	}
	var otherID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
		 VALUES ($1, $2, 'BattleLost', 2, '{}') RETURNING id`,
		f.worldID, otherClaims.PlayerID,
	).Scan(&otherID); err != nil {
		t.Fatalf("archive for other player: %v", err)
	}

	base := "/worlds/" + f.worldID.String() + "/notifications"
	code, _ := f.do(t, http.MethodPost, base+"/"+otherID.String()+"/read")
	if code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (idempotent no-op)", code)
	}

	var readAt *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT read_at::text FROM notifications WHERE id = $1`, otherID,
	).Scan(&readAt); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if readAt != nil {
		t.Error("another Wanax's notification was marked read")
	}
}
