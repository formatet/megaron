package handlers

// An order the player sent stays visible under its unit until the Runner has
// delivered it (ListUnits orders_en_route). DB integration test.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

type listedUnits struct {
	Units []struct {
		ID            uuid.UUID `json:"id"`
		OrdersEnRoute []struct {
			Verb      string    `json:"verb"`
			TargetQ   *int      `json:"target_q"`
			TargetR   *int      `json:"target_r"`
			ArrivesAt time.Time `json:"arrives_at"`
		} `json:"orders_en_route"`
	} `json:"units"`
}

func TestListUnits_OrdersEnRoute(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupRecallCourierWorld(t)
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Get("/worlds/{worldID}/units", uh.ListUnits)

	list := func(token string) listedUnits {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list units = %d %s", rec.Code, rec.Body.String())
		}
		var l listedUnits
		if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return l
	}
	mineEnRoute := func() int {
		for _, u := range list(f.accessToken).Units {
			if u.ID == f.unitID {
				return len(u.OrdersEnRoute)
			}
		}
		t.Fatalf("unit missing from list")
		return -1
	}

	if n := mineEnRoute(); n != 0 {
		t.Fatalf("orders_en_route before any order = %d", n)
	}

	// Redirect the marching unit to (2,1): the Runner is dispatched.
	rec := postJSON(t, router, f.accessToken,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall",
		map[string]any{"target_q": 2, "target_r": 1})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("redirect = %d %s", rec.Code, rec.Body.String())
	}
	var d struct {
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &d)

	for _, u := range list(f.accessToken).Units {
		if u.ID != f.unitID {
			continue
		}
		if len(u.OrdersEnRoute) != 1 {
			t.Fatalf("orders_en_route = %v, want exactly the redirect", u.OrdersEnRoute)
		}
		o := u.OrdersEnRoute[0]
		if o.Verb != "redirect" || o.TargetQ == nil || *o.TargetQ != 2 || o.TargetR == nil || *o.TargetR != 1 {
			t.Fatalf("order = %+v, want redirect to (2,1)", o)
		}
		if o.ArrivesAt.IsZero() {
			t.Fatalf("order has no arrives_at")
		}
	}

	// Another Wanax's runner whose payload names MY unit must not show on it,
	// and the other Wanax sees none of my units.
	authSvc := auth.NewService(pool, "test-secret")
	otherTok, _, err := authSvc.Register(ctx, "orders-other-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register other: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(otherTok)
	if err != nil {
		t.Fatalf("validate other: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_unit_id, origin_q, origin_r, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at, order_payload)
		 VALUES ($1,$2,$3::uuid,0,0,'foreign','outbound','order',0,0,0,0, now() + interval '1 hour',
		         jsonb_build_object('unit_id', $3::uuid::text, 'verb', 'march', 'march', jsonb_build_object('TargetQ', 3, 'TargetR', 0)))`,
		f.worldID, claims.PlayerID, f.unitID); err != nil {
		t.Fatalf("insert foreign runner: %v", err)
	}
	if n := mineEnRoute(); n != 1 {
		t.Fatalf("a foreign Wanax's runner leaked onto my unit: %d orders", n)
	}
	if n := len(list(otherTok).Units); n != 0 {
		t.Fatalf("other player sees %d units", n)
	}

	// Delivered (or lost): the redirect is gone, only the foreign runner remains
	// unshown.
	for _, st := range []string{"arrived", "lost"} {
		if _, err := pool.Exec(ctx, `UPDATE messengers SET status=$2 WHERE id=$1`, d.MessengerID, st); err != nil {
			t.Fatalf("mark %s: %v", st, err)
		}
		if n := mineEnRoute(); n != 0 {
			t.Fatalf("after %s: orders_en_route = %d, want 0", st, n)
		}
	}
}
