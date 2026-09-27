package handlers

// Acceptance 4 regression (megaron_plan_ordna_passage.md, slice 3b-2): Reply
// was rewritten to call messenger.StartReturnLeg — the same function the new
// stay-end timer uses — instead of doing its own resolve/update/schedule
// inline. This drives Reply through the real HTTP surface (not a hand-set
// end state) and proves the player-facing contract is unchanged: a spoken
// reply still turns the messenger around, returns returns_at/distance, and
// the reply text rides home with it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func replyTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type replyFixture struct {
	pool                   *pgxpool.Pool
	worldID                uuid.UUID
	senderID, receiverID   uuid.UUID
	senderToken, recvToken string
	originID, destID       uuid.UUID
	router                 *chi.Mux
}

// setupReplyFixture builds two settlements on a pure land route (q=0 and q=3,
// plains all the way) so Reply's return leg resolves a real land route
// through messenger.StartReturnLeg rather than falling back to the sea-lift.
func setupReplyFixture(t *testing.T) *replyFixture {
	t.Helper()
	pool := replyTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := &replyFixture{pool: pool}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-reply-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	var err error
	f.senderToken, _, err = authSvc.Register(ctx, "reply-sender-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register sender: %v", err)
	}
	senderClaims, err := authSvc.ValidateAccessToken(f.senderToken)
	if err != nil {
		t.Fatalf("validate sender token: %v", err)
	}
	f.senderID = senderClaims.PlayerID
	f.recvToken, _, err = authSvc.Register(ctx, "reply-receiver-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register receiver: %v", err)
	}
	recvClaims, err := authSvc.ValidateAccessToken(f.recvToken)
	if err != nil {
		t.Fatalf("validate receiver token: %v", err)
	}
	f.receiverID = recvClaims.PlayerID

	for q := 0; q <= 3; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}
	mkSettlement := func(q int, name string, owner uuid.UUID) uuid.UUID {
		var provID, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&provID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
			f.worldID, provID, name, owner,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.originID = mkSettlement(0, "Reply-Origin-"+uuid.NewString(), f.senderID)
	f.destID = mkSettlement(3, "Reply-Dest-"+uuid.NewString(), f.receiverID)

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	hub := notify.New()
	hub.SetPool(pool)
	mh := NewMessengerHandler(pool, scheduler, clk, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
	r.Post("/worlds/{worldID}/messengers/{messengerID}/reply", mh.Reply)

	f.router = r
	return f
}

func (f *replyFixture) post(t *testing.T, token, path string, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// TestReply_TurnsMessengerAroundWithReplyText is the acceptance-4 regression:
// Reply, driven through the real HTTP surface, still delivers a messenger,
// accepts a reply and turns it around with the correct response shape and DB
// state — unchanged behaviour after Reply was rewritten onto
// messenger.StartReturnLeg.
func TestReply_TurnsMessengerAroundWithReplyText(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()

	code, resp := f.post(t, f.senderToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers",
		map[string]any{"destination_id": f.destID.String(), "message": "Will you trade tin?"})
	if code != http.StatusOK && code != http.StatusCreated && code != http.StatusAccepted {
		t.Fatalf("Send status = %d, body = %v", code, resp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM messengers WHERE world_id = $1 AND destination_id = $2 ORDER BY sent_at DESC LIMIT 1`,
		f.worldID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}

	// Skip the real courier travel time — same shortcut
	// messenger_trade_naval_test.go's deliverMessenger uses.
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status = 'delivered' WHERE id = $1`, messengerID); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}

	code, resp = f.post(t, f.recvToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/reply",
		map[string]any{"reply": "Bring copper and we shall speak"})
	if code != http.StatusOK {
		t.Fatalf("Reply status = %d, body = %v", code, resp)
	}
	if _, ok := resp["returns_at"]; !ok {
		t.Errorf("response missing returns_at: %v", resp)
	}
	if _, ok := resp["distance"]; !ok {
		t.Errorf("response missing distance: %v", resp)
	}
	if _, ok := resp["passage_status"]; ok {
		t.Errorf("response carries passage_status for a pure land route: %v", resp)
	}

	var status, replyText string
	var returnDepartsAt *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, reply_text, return_departs_at FROM messengers WHERE id = $1`, messengerID,
	).Scan(&status, &replyText, &returnDepartsAt); err != nil {
		t.Fatalf("load messenger after reply: %v", err)
	}
	if status != "returning" {
		t.Errorf("status = %q, want returning", status)
	}
	if replyText != "Bring copper and we shall speak" {
		t.Errorf("reply_text = %q, want the reply carried home", replyText)
	}
	if returnDepartsAt == nil {
		t.Error("return_departs_at not set — the eye and the eventual arrival both need this")
	}
}

// TestReply_ConflictWhenAlreadyTurnedAround is the regression for Reply's
// existing status='delivered' guard, unchanged by the StartReturnLeg
// refactor: a messenger that has already started its way home (e.g. its stay
// already ended) cannot be replied to a second time.
func TestReply_ConflictWhenAlreadyTurnedAround(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()

	code, resp := f.post(t, f.senderToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers",
		map[string]any{"destination_id": f.destID.String(), "message": "Anyone there?"})
	if code != http.StatusOK && code != http.StatusCreated && code != http.StatusAccepted {
		t.Fatalf("Send status = %d, body = %v", code, resp)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM messengers WHERE world_id = $1 AND destination_id = $2 ORDER BY sent_at DESC LIMIT 1`,
		f.worldID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("load sent messenger: %v", err)
	}
	// Already turned around (as the stay-end timer would have done) BEFORE the reply lands.
	if _, err := f.pool.Exec(ctx,
		`UPDATE messengers SET status = 'returning', return_departs_at = now(), arrives_at = now() WHERE id = $1`,
		messengerID,
	); err != nil {
		t.Fatalf("simulate already-returning: %v", err)
	}

	code, resp = f.post(t, f.recvToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/reply",
		map[string]any{"reply": "too late"})
	if code != http.StatusForbidden {
		t.Fatalf("Reply status = %d, body = %v, want 403 (messenger not 'delivered' any more)", code, resp)
	}
}
