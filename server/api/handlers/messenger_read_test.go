package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMessengerReadReceiptIsAuthorizedIdempotentAndDoesNotReply(t *testing.T) {
	f := setupPlacementFixture(t, nil)
	pool := p10TestPool(t)
	ctx := context.Background()
	authSvc := auth.NewService(pool, "test-secret")
	otherToken, _, err := authSvc.Register(ctx, "read-other-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	var owner, id uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$3,'Read me','outbound',0,0,now(),now()) RETURNING id`, f.worldID, owner, f.settlementID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC))
	h := NewMessengerHandler(pool, nil, clk, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/messengers/{messengerID}/read", h.Read)
	r.Get("/worlds/{worldID}/messengers/inbox", h.Inbox)
	call := func(method, path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	path := "/worlds/" + f.worldID.String() + "/messengers/" + id.String() + "/read"
	if got := call("POST", path, f.accessToken); got.Code != http.StatusNotFound {
		t.Fatalf("in-flight letter read%d", got.Code)
	}
	if _, err := pool.Exec(ctx, `UPDATE messengers SET status='delivered',trade_offer='{"kind":"buy","want_good":"fish","want_qty":1,"offer_silver":1,"status":"pending"}' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if got := call("POST", path, otherToken); got.Code != http.StatusNotFound {
		t.Fatalf("stranger read%d", got.Code)
	}
	var first time.Time
	for i := 0; i < 2; i++ {
		if got := call("POST", path, f.accessToken); got.Code != http.StatusOK {
			t.Fatalf("read%d %s", got.Code, got.Body)
		}
		var read time.Time
		var status, offer string
		if err := pool.QueryRow(ctx, `SELECT read_at,status,trade_offer->>'status' FROM messengers WHERE id=$1`, id).Scan(&read, &status, &offer); err != nil {
			t.Fatal(err)
		}
		if status != "delivered" || offer != "pending" {
			t.Fatalf("reading moved/accepted %s/%s", status, offer)
		}
		if i == 0 {
			first = read
			clk.Advance(time.Hour)
		} else if !read.Equal(first) {
			t.Fatal("replay changed read time")
		}
	}
	got := call("GET", "/worlds/"+f.worldID.String()+"/messengers/inbox", f.accessToken)
	var inbox []map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &inbox); err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0]["read_at"] == nil {
		t.Fatalf("read pending offer disappeared or lacks receipt: %s", got.Body)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1`, f.worldID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("reading scheduled an order")
	}
}
