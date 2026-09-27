package handlers

// DB integration tests for a same-root bifynd from megaron_plan_ordna_passage.md
// 3b-1: LoadLiveEyes' outbound query (province/eyes.go mRows) read m.hex_q/hex_r
// as the runner's ORIGIN. That is only true for kind='order' (unit.go
// sendOrderCourier) and kind='recall' (province.go's march recall) — both
// dispatchers deliberately write the origin there. A plain diplomatic message
// (Send/SendFromHost, api/handlers/messenger.go) writes the DESTINATION into
// hex_q/hex_r instead (dQ,dR at INSERT time), so the outbound query's "origin"
// was really the destination a second time (COALESCE(dest_q, dp.map_q) also
// resolves to the destination for this kind, since dest_q is NULL). Net effect:
// pos == target == destination for the WHOLE outbound leg of a plain message —
// the sender's own eye sat on the destination from the instant it was sent,
// land route or not, passage or not.
//
// Both tests below go through the REAL POST flow (Send, SendFromHost), per
// megaron_plan_ordna_passage.md's "Arbetsordning": every acceptance here must
// be driven through the real endpoint, not a hand-set end state.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// originEyeFixture is a minimal Send/SendFromHost rig — its own pool
// connection and router (Send + SendFromHost), no naval/passage machinery,
// since both cases here are pure land routes.
type originEyeFixture struct {
	pool    *pgxpool.Pool
	worldID uuid.UUID
	router  *chi.Mux
	authSvc *auth.Service
	clk     *clock.TestClock
}

func setupOriginEyeFixture(t *testing.T) *originEyeFixture {
	t.Helper()
	pool := navalPlayerTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-origin-eye-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	hub := notify.New()
	hub.SetPool(pool)
	mh := NewMessengerHandler(pool, scheduler, clk, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
	r.Post("/worlds/{worldID}/founding/messengers", mh.SendFromHost)

	return &originEyeFixture{pool: pool, worldID: worldID, router: r, authSvc: authSvc, clk: clk}
}

func (f *originEyeFixture) mapTile(t *testing.T, q, r int, terrain string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
		f.worldID, q, r, terrain,
	); err != nil {
		t.Fatalf("insert map tile (%d,%d)=%s: %v", q, r, terrain, err)
	}
}

func (f *originEyeFixture) settlement(t *testing.T, name string, q int, ownerID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	f.mapTile(t, q, 0, "plains")
	var provinceID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
		f.worldID, q,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province %s: %v", name, err)
	}
	var settlementID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
		f.worldID, provinceID, name, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %s: %v", name, err)
	}
	return settlementID
}

func (f *originEyeFixture) registerPlayer(t *testing.T, prefix string) (uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	token, _, err := f.authSvc.Register(ctx, prefix+"-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register %s: %v", prefix, err)
	}
	claims, err := f.authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate %s token: %v", prefix, err)
	}
	return claims.PlayerID, token
}

func (f *originEyeFixture) post(t *testing.T, token, path string, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// TestLoadLiveEyes_SentMessageSeesOwnOriginNotDestination is the RED-first case
// (city-sent half): a plain message sent over a pure land route must give the
// sender's own eye somewhere near ITS OWN city, not the destination it hasn't
// reached — the moment it is sent. On master the eye sits at the destination
// (4,0) instead of the origin (0,0).
func TestLoadLiveEyes_SentMessageSeesOwnOriginNotDestination(t *testing.T) {
	f := setupOriginEyeFixture(t)
	ctx := context.Background()

	senderID, senderToken := f.registerPlayer(t, "origin-sender")
	recipientID, _ := f.registerPlayer(t, "origin-recipient")

	originID := f.settlement(t, "Originburg", 0, senderID)
	destID := f.settlement(t, "Farcity", 4, recipientID)
	for q := 1; q <= 3; q++ {
		f.mapTile(t, q, 0, "plains")
	}

	code, resp := f.post(t, senderToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+originID.String()+"/messengers",
		map[string]any{"destination_id": destID.String(), "message": "hello over land"})
	if code != 201 {
		t.Fatalf("Send status = %d, want 201 (%v)", code, resp)
	}
	if _, ok := resp["passage_status"]; ok {
		t.Fatalf("response carries passage_status for a land route, want none: %v", resp)
	}

	eyes := province.LoadLiveEyes(ctx, f.pool, f.worldID, senderID, f.clk.Now())
	var gotEye *province.Eye
	for i := range eyes {
		if eyes[i].Kind == province.EyeLandUnit {
			gotEye = &eyes[i]
		}
	}
	if gotEye == nil {
		t.Fatalf("no land-unit eye for the just-sent runner; eyes = %+v", eyes)
	}
	if gotEye.Pos.Q == 4 && gotEye.Pos.R == 0 {
		t.Fatalf("runner eye sits at the destination (4,0) the instant it was sent — "+
			"the bifynd this fix closes: the outbound query must resolve the REAL origin (0,0), not hex_q/hex_r's destination")
	}
	if gotEye.Pos.Q != 0 || gotEye.Pos.R != 0 {
		t.Fatalf("runner eye = %+v, want the sender's own city (0,0) right after send", gotEye.Pos)
	}
}

// TestLoadLiveEyes_HostSentMessageSeesHostPositionNotDestination is the
// RED-first case's host half: SendFromHost freezes the wandering host's
// CURRENT position onto origin_q/origin_r (mig 087) — the eye must read that,
// never hex_q/hex_r (the destination for this kind too).
func TestLoadLiveEyes_HostSentMessageSeesHostPositionNotDestination(t *testing.T) {
	f := setupOriginEyeFixture(t)
	ctx := context.Background()

	senderID, senderToken := f.registerPlayer(t, "host-sender")
	recipientID, _ := f.registerPlayer(t, "host-recipient")

	// The wandering host, standing still (not marching) at (0,0) — no
	// settlement of the sender's own exists yet (founder phase).
	var hostID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'nomadic_host', 'land', 1, 'positioned', 0, 0) RETURNING id`,
		f.worldID, senderID,
	).Scan(&hostID); err != nil {
		t.Fatalf("create host unit: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO founder_phase (world_id, owner_id, host_unit_id, population,
		                            grain_amount, grain_rate, silver_amount, silver_rate)
		 VALUES ($1, $2, $3, 1000, 100, 0, 100, 0)`,
		f.worldID, senderID, hostID,
	); err != nil {
		t.Fatalf("create founder_phase: %v", err)
	}

	destID := f.settlement(t, "Farcity", 4, recipientID)
	// The host's own tile (0,0) — missing it made FindPath see no origin at
	// all, wrongly routing this plain land send through the sea-lift branch
	// (megaron_plan_ordna_passage.md 3b-4 R2/R3: a courier with no real land
	// route and no owned port gets ErrNoPort instead of the old fallback). A
	// city-founded sender gets this tile from f.settlement() itself; a
	// founder-phase host, with no settlement at all, does not.
	f.mapTile(t, 0, 0, "plains")
	for q := 1; q <= 3; q++ {
		f.mapTile(t, q, 0, "plains")
	}
	// FOW gate (Send/SendFromHost require the destination within the sender's
	// visible range): a scouted-tile row at the destination hex is exactly
	// the memory mechanism that grants this in the real game (a host that has
	// wandered close enough to see the city).
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, 4, 0)`,
		f.worldID, senderID,
	); err != nil {
		t.Fatalf("seed scouted tile: %v", err)
	}

	code, resp := f.post(t, senderToken,
		"/worlds/"+f.worldID.String()+"/founding/messengers",
		map[string]any{"destination_id": destID.String(), "message": "hello from the host"})
	if code != 201 {
		t.Fatalf("SendFromHost status = %d, want 201 (%v)", code, resp)
	}

	eyes := province.LoadLiveEyes(ctx, f.pool, f.worldID, senderID, f.clk.Now())
	var gotEye *province.Eye
	for i := range eyes {
		if eyes[i].Kind == province.EyeLandUnit {
			gotEye = &eyes[i]
		}
	}
	if gotEye == nil {
		t.Fatalf("no land-unit eye for the just-sent host runner; eyes = %+v", eyes)
	}
	if gotEye.Pos.Q == 4 && gotEye.Pos.R == 0 {
		t.Fatalf("host runner eye sits at the destination (4,0) the instant it was sent — "+
			"want the host's own frozen departure point (0,0), read from origin_q/origin_r")
	}
	if gotEye.Pos.Q != 0 || gotEye.Pos.R != 0 {
		t.Fatalf("host runner eye = %+v, want the host's departure point (0,0)", gotEye.Pos)
	}
}
