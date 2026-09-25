package handlers

// Stance to a MARCHING unit (megaron_styrande_beslut §11, Timothy 2026-09-25):
// "yes, by Runner, but then the Runner must catch up with it". Dispatch reuses
// redirect's catch-up (messenger.InterceptCourierTarget); delivery runs the
// new "stance_pursuit" order verb (combat.SetStanceInPursuit). Full E2E
// through the HTTP handler on the same fixture the recall/redirect courier
// tests use: capital at (0,0), a spearman 45 min into a 3 h march (0,0)→(4,0).
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type stanceDispatch struct {
	Status           string    `json:"status"`
	Verb             string    `json:"verb"`
	MessengerID      uuid.UUID `json:"messenger_id"`
	CourierArrivesAt time.Time `json:"courier_arrives_at"`
	CatchUp          string    `json:"catch_up"`
	InterceptQ       int       `json:"intercept_q"`
	InterceptR       int       `json:"intercept_r"`
}

func postJSON(t *testing.T, router *chi.Mux, token, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func setupStanceMarchingWorld(t *testing.T) (recallCourierFixture, *chi.Mux) {
	t.Helper()
	f, router := setupRecallCourierWorld(t)
	pool := unitLoadTestPool(t)
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
	return f, router
}

func dispatchStance(t *testing.T, f recallCourierFixture, router *chi.Mux, stance string) stanceDispatch {
	t.Helper()
	rec := postJSON(t, router, f.accessToken,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/stance", map[string]any{"stance": stance})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("stance(%s) on marching unit = %d %q, want 202 order_dispatched", stance, rec.Code, rec.Body.String())
	}
	var d stanceDispatch
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode dispatch: %v", err)
	}
	return d
}

func loadDelivery(t *testing.T, messengerID uuid.UUID) events.ScheduledEvent {
	t.Helper()
	pool := unitLoadTestPool(t)
	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM scheduled_events
		 WHERE event_type = $1 AND (payload->>'messenger_id')::uuid = $2
		   AND processed_at IS NULL AND failed_at IS NULL`,
		string(events.ScheduledOrderDelivery), messengerID,
	).Scan(&raw); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	return events.ScheduledEvent{Payload: raw}
}

func asReject(err error, rej **combat.OrderReject) bool { return errors.As(err, rej) }

// (1) Dispatch: 202 order_dispatched, verb stance_pursuit, a Runner aimed at
// the redirect intercept point — the courier ETA is no later than the moment
// the unit itself reaches that hex, and earlier than the march's end.
func TestStanceMarching_DispatchCatchesUpLikeRedirect(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupStanceMarchingWorld(t)

	d := dispatchStance(t, f, router, "sentry")
	if d.Status != "order_dispatched" || d.Verb != "stance_pursuit" || d.CatchUp != "on_the_march" {
		t.Fatalf("dispatch = %s/%s/%s, want order_dispatched/stance_pursuit/on_the_march", d.Status, d.Verb, d.CatchUp)
	}

	// Same catch-up model as redirect: the aim must equal what
	// InterceptCourierTarget computes for this courier origin (capital (0,0)).
	var dq, dr int
	var ea time.Time
	_ = pool.QueryRow(ctx, `SELECT dest_q, dest_r, arrives_at FROM messengers WHERE id=$1`, d.MessengerID).Scan(&dq, &dr, &ea)
	if dq != d.InterceptQ || dr != d.InterceptR {
		t.Errorf("messenger dest (%d,%d) != response intercept (%d,%d)", dq, dr, d.InterceptQ, d.InterceptR)
	}
	if d.InterceptR != 0 || d.InterceptQ < 1 || d.InterceptQ > 4 {
		t.Fatalf("intercept (%d,%d) is not on the unit's path (0,0)→(4,0)", d.InterceptQ, d.InterceptR)
	}
	// Unit reaches path[i] at departsAt + i/4 of the march (InterceptAlongPath's model).
	unitAt := f.departsAt.Add(time.Duration(float64(d.InterceptQ) / 4 * float64(f.arrivesAt.Sub(f.departsAt))))
	if d.CourierArrivesAt.After(unitAt.Add(time.Second)) {
		t.Errorf("courier arrives %v, after the unit passes the intercept hex at %v — not a catch-up", d.CourierArrivesAt, unitAt)
	}
	if !d.CourierArrivesAt.Before(f.arrivesAt) {
		t.Errorf("courier arrives %v, not before the march ends %v", d.CourierArrivesAt, f.arrivesAt)
	}

	// Command is never instant: nothing on the unit changed yet.
	var stance *string
	_ = pool.QueryRow(ctx, `SELECT stance FROM units WHERE id=$1`, f.unitID).Scan(&stance)
	if stance != nil {
		t.Errorf("stance = %q before delivery, want unset (command is never instant)", *stance)
	}
}

// (2) Delivery while still marching: the stance is set on the moving unit
// (no hold centre yet); when the march ends the sentry's centre is the hex it
// stopped on. Also (5): running the same delivery twice changes nothing more.
func TestStanceMarching_DeliveredWhileMarching(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupStanceMarchingWorld(t)

	d := dispatchStance(t, f, router, "sentry")
	evt := loadDelivery(t, d.MessengerID)
	clk := clock.NewTestClock(time.Now())
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, evt); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	var status string
	var stance *string
	var sq, sr *int
	var targetQ int
	if err := pool.QueryRow(ctx, `SELECT status, stance, sentry_q, sentry_r, target_q FROM units WHERE id=$1`, f.unitID).
		Scan(&status, &stance, &sq, &sr, &targetQ); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if status != "marching" || targetQ != 4 {
		t.Fatalf("unit status=%s target_q=%d, want still marching to 4 — a stance must not touch the course", status, targetQ)
	}
	if stance == nil || *stance != "sentry" {
		t.Fatalf("stance after delivery = %v, want sentry", stance)
	}
	if sq != nil || sr != nil {
		t.Errorf("sentry centre set while marching (%v,%v), want NULL until it stops", sq, sr)
	}

	// (5) idempotent replay: same event again — no second stance event, no error.
	var nEvents int
	countEvents := func() int {
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type=$2`,
			f.unitID, string(unit.EventUnitStanceChanged)).Scan(&nEvents)
		return nEvents
	}
	before := countEvents()
	if before != 1 {
		t.Fatalf("stance events after first delivery = %d, want 1", before)
	}
	if err := odh.Handle(ctx, evt); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if after := countEvents(); after != before {
		t.Errorf("stance events after replay = %d, want %d (delivery not idempotent)", after, before)
	}

	// The march ends: the sentry holds the hex it stopped on.
	if _, err := pool.Exec(ctx, `UPDATE units SET arrives_at = now() WHERE id=$1`, f.unitID); err != nil {
		t.Fatalf("fast-forward march: %v", err)
	}
	payload, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID})
	ah := combat.NewUnitArrivalHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clk), clk, economy.SitosConfig{})
	if err := ah.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("arrival: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, stance, sentry_q, sentry_r FROM units WHERE id=$1`, f.unitID).
		Scan(&status, &stance, &sq, &sr); err != nil {
		t.Fatalf("read unit after arrival: %v", err)
	}
	if status != "positioned" || stance == nil || *stance != "sentry" || sq == nil || sr == nil || *sq != 4 || *sr != 0 {
		t.Errorf("after arrival: status=%s stance=%v centre=(%v,%v), want positioned sentry holding (4,0)", status, stance, sq, sr)
	}
}

// (3) The unit has already stopped when the Runner arrives: the stance
// applies where it stands (fortify here — it digs in at the destination).
func TestStanceMarching_DeliveredAfterArrival(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupStanceMarchingWorld(t)

	d := dispatchStance(t, f, router, "fortify")
	// The unit finishes its march before the Runner lands.
	if _, err := pool.Exec(ctx,
		`UPDATE units SET status='positioned', q=4, r=0, target_q=NULL, target_r=NULL,
		   departs_at=NULL, arrives_at=NULL, depart_tick=NULL, arrive_tick=NULL WHERE id=$1`, f.unitID); err != nil {
		t.Fatalf("simulate arrival: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, loadDelivery(t, d.MessengerID)); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	var status string
	var stance *string
	var q int
	_ = pool.QueryRow(ctx, `SELECT status, stance, q FROM units WHERE id=$1`, f.unitID).Scan(&status, &stance, &q)
	if status != "positioned" || q != 4 || stance == nil || *stance != "fortify" {
		t.Errorf("after delivery: status=%s q=%d stance=%v, want positioned at 4 with fortify", status, q, stance)
	}
}

// (4) Coexistence: a stance Runner and a redirect Runner may pursue the same
// unit at once — the recall/redirect 409 guard is scoped to those verbs, and
// stance orders stay latest-delivered-wins. A second recall/redirect is still
// refused while one is in flight.
func TestStanceMarching_CoexistsWithRedirect(t *testing.T) {
	f, router := setupStanceMarchingWorld(t)
	base := "/worlds/" + f.worldID.String() + "/units/" + f.unitID.String()

	if rec := postJSON(t, router, f.accessToken, base+"/recall", map[string]any{"target_q": 2, "target_r": 1}); rec.Code != http.StatusAccepted {
		t.Fatalf("redirect = %d %q, want 202", rec.Code, rec.Body.String())
	}
	dispatchStance(t, f, router, "storm") // 202 despite the redirect in flight
	dispatchStance(t, f, router, "none")  // and a second stance: latest-delivered-wins
	if rec := postJSON(t, router, f.accessToken, base+"/recall", nil); rec.Code != http.StatusConflict {
		t.Errorf("second recall while a redirect is in flight = %d, want 409 (guard must still hold)", rec.Code)
	}
}

// Where redirect must refuse (no Runner from the far capital (6,5) can catch
// the unit before its march ends — TestRecall_UndeliverableFromFarCapital),
// a stance is still deliverable: the Runner is aimed at the march's
// destination and reaches the unit after it has stopped.
func TestStanceMarching_NoInterceptAimsAtDestination(t *testing.T) {
	pool := unitLoadTestPool(t)
	f, router := setupInterceptWorld(t, 6, 5)
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)

	rec := postJSON(t, router, f.accessToken,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/stance", map[string]any{"stance": "fortify"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("stance = %d %q, want 202", rec.Code, rec.Body.String())
	}
	var d stanceDispatch
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if d.CatchUp != "at_destination" || d.InterceptQ != 8 || d.InterceptR != 0 {
		t.Errorf("catch_up=%s aim=(%d,%d), want at_destination (8,0)", d.CatchUp, d.InterceptQ, d.InterceptR)
	}
	if d.CourierArrivesAt.Before(f.arrivesAt) {
		t.Errorf("courier arrives %v before the march ends %v — then an intercept existed", d.CourierArrivesAt, f.arrivesAt)
	}
}

// The frozen "stance" verb still refuses a marching unit at delivery — only
// the new verb pursues. (An in-flight "stance" order aimed at a unit that has
// since set out keeps its old meaning.)
func TestStanceMarching_FrozenStanceVerbStillRefusesMarching(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, _ := setupStanceMarchingWorld(t)
	_, err := combat.SetStance(ctx, pool, events.NewStore(pool), combat.StanceOrder{
		WorldID: f.worldID, PlayerID: f.playerID, UnitID: f.unitID, Stance: "sentry",
	})
	var rej *combat.OrderReject
	if err == nil || !asReject(err, &rej) || rej.Status != http.StatusUnprocessableEntity {
		t.Fatalf("SetStance(marching) err = %v, want 422 reject", err)
	}
}
