package handlers

// DB/handler tests for megaron_plan_budets_tre_ben.md (slice 3c) R3:
// MapMessengers must tell the OWNER of a passage-lifted runner which of the
// five physical legs (to_port, waiting, aboard, ashore, sealed) it is
// actually on right now, plus the aboard-with-NULL-columns fallback (a
// messenger boarded before mig 151 shipped). Same rig as
// messenger_passage_map_test.go (citiesTestPool, registerViewer,
// insertProvince/insertSettlement) — this file adds its own view struct and
// call helper so it can decode the new leg_* fields without touching the
// shared messengerMarkerView other test files already depend on.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type legMarkerView struct {
	ID                 string     `json:"id"`
	Own                bool       `json:"own"`
	PassageStatus      *string    `json:"passage_status"`
	Leg                *string    `json:"leg"`
	LegFromQ           *int       `json:"leg_from_q"`
	LegFromR           *int       `json:"leg_from_r"`
	LegToQ             *int       `json:"leg_to_q"`
	LegToR             *int       `json:"leg_to_r"`
	LegStart           *time.Time `json:"leg_start"`
	LegEnd             *time.Time `json:"leg_end"`
	CarrierUnitID      *string    `json:"carrier_unit_id"`
	CarrierTransportID *string    `json:"carrier_transport_id"`
	CarrierName        *string    `json:"carrier_name"`
	Stalled            *bool      `json:"stalled"`
}

func callMessengersLeg(t *testing.T, pool *pgxpool.Pool, authSvc *auth.Service, worldID uuid.UUID, token string, now time.Time) []legMarkerView {
	t.Helper()
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(now))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/messengers", wh.MapMessengers)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/messengers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /messengers = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var out []legMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /messengers: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

// legFixture is the common geography every leg case shares: the viewer's own
// origin (q=0) and port (q=3) settlements, and a distant destination (q=20,
// owned by someone else — the leg fields never depend on who owns the final
// target).
type legFixture struct {
	pool                     *pgxpool.Pool
	authSvc                  *auth.Service
	worldID                  uuid.UUID
	viewerID                 uuid.UUID
	token                    string
	originID, portID, destID uuid.UUID
}

func setupLegFixture(t *testing.T) *legFixture {
	t.Helper()
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "leg-viewer")

	var otherID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"leg-other-"+uuid.New().String(),
	).Scan(&otherID); err != nil {
		t.Fatalf("create other player: %v", err)
	}

	originID := insertSettlement(t, ctx, pool, worldID, insertProvince(t, ctx, pool, worldID, 0, 0), "Legton", viewerID, true)
	portID := insertSettlement(t, ctx, pool, worldID, insertProvince(t, ctx, pool, worldID, 3, 0), "Legport", viewerID, false)
	destID := insertSettlement(t, ctx, pool, worldID, insertProvince(t, ctx, pool, worldID, 20, 0), "Legdest", otherID, false)

	return &legFixture{
		pool: pool, authSvc: authSvc,
		worldID: worldID, viewerID: viewerID, token: token,
		originID: originID, portID: portID, destID: destID,
	}
}

func TestMapMessengers_Leg_ToPort(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'awaiting_passage',$7,0)`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-1*time.Hour), now.Add(1*time.Hour), f.portID,
	); err != nil {
		t.Fatalf("insert to_port messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "to_port" {
		t.Fatalf("leg = %v, want to_port", m.Leg)
	}
	if m.LegToQ == nil || m.LegToR == nil || *m.LegToQ != 3 || *m.LegToR != 0 {
		t.Errorf("leg_to = %v,%v, want the port (3,0)", m.LegToQ, m.LegToR)
	}
	if m.LegFromQ == nil || *m.LegFromQ != 0 {
		t.Errorf("leg_from_q = %v, want 0 (the origin)", m.LegFromQ)
	}
}

func TestMapMessengers_Leg_Waiting(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick,
		                          passage_stalled_notified_tick)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'awaiting_passage',$7,0,10) RETURNING id`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-2*time.Hour), now.Add(-1*time.Hour), f.portID,
	).Scan(&id); err != nil {
		t.Fatalf("insert waiting messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "waiting" {
		t.Fatalf("leg = %v, want waiting", m.Leg)
	}
	if m.LegFromQ == nil || m.LegToQ == nil || *m.LegFromQ != 3 || *m.LegToQ != 3 {
		t.Errorf("leg_from/leg_to = %v,%v, want both the port (3,0)", m.LegFromQ, m.LegToQ)
	}
	if m.Stalled == nil || !*m.Stalled {
		t.Errorf("stalled = %v, want true (passage_stalled_notified_tick is set)", m.Stalled)
	}
}

func TestMapMessengers_Leg_Aboard(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, name)
		 VALUES ($1,$2,'merchantman','naval',1,10,'marching',3,0,'Leg Carrier') RETURNING id`,
		f.worldID, f.viewerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create carrier ship: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick,
		                          carrier_unit_id, carrier_name, disembark_q, disembark_r, boarded_at, disembark_at)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'aboard',$7,0,$8,'Leg Carrier',10,0,$9,$10)`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-2*time.Hour), now.Add(2*time.Hour), f.portID,
		shipID, now.Add(-30*time.Minute), now.Add(30*time.Minute),
	); err != nil {
		t.Fatalf("insert aboard messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "aboard" {
		t.Fatalf("leg = %v, want aboard", m.Leg)
	}
	if m.LegFromQ == nil || *m.LegFromQ != 3 {
		t.Errorf("leg_from_q = %v, want 3 (the port)", m.LegFromQ)
	}
	if m.LegToQ == nil || *m.LegToQ != 10 {
		t.Errorf("leg_to_q = %v, want 10 (disembark_q)", m.LegToQ)
	}
	if m.CarrierUnitID == nil || *m.CarrierUnitID != shipID.String() {
		t.Errorf("carrier_unit_id = %v, want %v", m.CarrierUnitID, shipID)
	}
	if m.CarrierName == nil || *m.CarrierName != "Leg Carrier" {
		t.Errorf("carrier_name = %v, want %q", m.CarrierName, "Leg Carrier")
	}
}

func TestMapMessengers_Leg_Ashore(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, name)
		 VALUES ($1,$2,'merchantman','naval',1,10,'marching',10,0,'Leg Carrier') RETURNING id`,
		f.worldID, f.viewerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create carrier ship: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick,
		                          carrier_unit_id, carrier_name, disembark_q, disembark_r, boarded_at, disembark_at)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'aboard',$7,0,$8,'Leg Carrier',10,0,$9,$10)`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-2*time.Hour), now.Add(30*time.Minute), f.portID,
		shipID, now.Add(-90*time.Minute), now.Add(-30*time.Minute),
	); err != nil {
		t.Fatalf("insert ashore messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "ashore" {
		t.Fatalf("leg = %v, want ashore", m.Leg)
	}
	if m.LegFromQ == nil || *m.LegFromQ != 10 {
		t.Errorf("leg_from_q = %v, want 10 (disembark_q)", m.LegFromQ)
	}
	if m.LegToQ == nil || *m.LegToQ != 20 {
		t.Errorf("leg_to_q = %v, want 20 (the true final destination)", m.LegToQ)
	}
}

func TestMapMessengers_Leg_Sealed(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick,
		                          passage_lost_until_tick)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'returning_sealed',$7,0,999)`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-2*time.Hour), now.Add(2*time.Hour), f.portID,
	); err != nil {
		t.Fatalf("insert sealed messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "sealed" {
		t.Fatalf("leg = %v, want sealed", m.Leg)
	}
	if m.LegFromQ == nil || m.LegToQ == nil || *m.LegFromQ != 3 || *m.LegToQ != 3 {
		t.Errorf("leg_from/leg_to = %v,%v, want both the port (3,0)", m.LegFromQ, m.LegToQ)
	}
}

// TestMapMessengers_Leg_AboardNullColumns is R3's own stop condition: a
// messenger boarded before mig 151 shipped has passage_status='aboard' but
// NULL disembark_q/r/boarded_at/disembark_at — never guess a disembark point,
// draw stationary at the port instead.
func TestMapMessengers_Leg_AboardNullColumns(t *testing.T) {
	f := setupLegFixture(t)
	pool := f.pool
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, name)
		 VALUES ($1,$2,'merchantman','naval',1,10,'marching',3,0,'Legacy Carrier') RETURNING id`,
		f.worldID, f.viewerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create carrier ship: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind,
		                          hex_q, hex_r, sent_at, arrives_at, passage_status, passage_port_id, passage_since_tick,
		                          carrier_unit_id, carrier_name)
		 VALUES ($1,$2,$3,$4,'hi','outbound','message',0,0,$5,$6,'aboard',$7,0,$8,'Legacy Carrier')`,
		f.worldID, f.viewerID, f.originID, f.destID, now.Add(-2*time.Hour), now.Add(2*time.Hour), f.portID, shipID,
	); err != nil {
		t.Fatalf("insert legacy aboard messenger: %v", err)
	}

	got := callMessengersLeg(t, pool, f.authSvc, f.worldID, f.token, now)
	if len(got) != 1 {
		t.Fatalf("got %d markers, want 1", len(got))
	}
	m := got[0]
	if m.Leg == nil || *m.Leg != "aboard" {
		t.Fatalf("leg = %v, want aboard (stationary fallback)", m.Leg)
	}
	if m.LegFromQ == nil || m.LegToQ == nil || *m.LegFromQ != 3 || *m.LegToQ != 3 {
		t.Errorf("leg_from/leg_to = %v,%v, want both the port (3,0) — never a guessed disembark point", m.LegFromQ, m.LegToQ)
	}
}
