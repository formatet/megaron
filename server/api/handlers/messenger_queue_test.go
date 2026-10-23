package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type queueTestPage struct {
	Items []struct {
		ID         uuid.UUID  `json:"id"`
		Group      string     `json:"queue_group"`
		State      string     `json:"offer_state"`
		Actions    []string   `json:"actions"`
		Affordable *bool      `json:"affordable"`
		Missing    string     `json:"missing_payment"`
		ReadAt     *time.Time `json:"read_at"`
	} `json:"items"`
	Next string `json:"next_cursor"`
}

func TestCorrespondenceQueueKeepsExpiredAndInsolventWithLiveActions(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	clk := clock.NewTestClock(now)
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	ids := map[string]uuid.UUID{}
	for i, state := range []string{"pending", "expired", "declined", "accepted", "plain", "elapsed", "boundary", "withdrawn", "undelivered"} {
		var offer any = fmt.Sprintf(`{"kind":"sell","offer_good":"grain","offer_qty":1,"want_silver":999999,"status":%q}`, state)
		if state == "plain" {
			offer = nil
		}
		if state == "elapsed" || state == "boundary" || state == "withdrawn" || state == "undelivered" {
			offer = `{"kind":"buy","want_good":"grain","want_qty":1,"offer_silver":1,"status":"pending"}`
		}
		expires := now.Add(time.Hour)
		if state == "elapsed" {
			expires = now.Add(-time.Second)
		}
		if state == "boundary" {
			expires = now
		}
		status := "delivered"
		if state == "undelivered" {
			status = "outbound"
		}
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at,trade_offer,expires_at,withdrawn) VALUES($1,$2,$3,$4,$5,$6,3,0,$7,$7,$8::jsonb,$9,$10) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, state, status, now.Add(time.Duration(-i)*time.Second), offer, expires, state == "withdrawn").Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[state] = id
	}
	get := func(token string) (*httptest.ResponseRecorder, queueTestPage) {
		req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		var page queueTestPage
		if rec.Code != 200 {
			t.Fatalf("queue %d %s", rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("queue contract: %v: %s", err, rec.Body)
		}
		return rec, page
	}
	_, p := get(f.recvToken)
	if len(p.Items) != 7 {
		t.Fatalf("delivered offers missing or private letters exposed: got %d want7", len(p.Items))
	}
	for _, m := range p.Items {
		switch m.ID {
		case ids["pending"]:
			if m.Group != "waiting_you" || m.State != "pending" || m.Affordable == nil || *m.Affordable || m.Missing == "" || !slices.Contains(m.Actions, "trade_decline") || slices.Contains(m.Actions, "trade_accept") {
				t.Fatalf("insolvent row/action wrong: %+v", m)
			}
		case ids["elapsed"], ids["boundary"], ids["expired"]:
			if m.State != "expired" || m.Group != "completed" || slices.Contains(m.Actions, "trade_accept") || slices.Contains(m.Actions, "trade_decline") || slices.Contains(m.Actions, "reply") {
				t.Fatalf("expired offer still actionable: %+v", m)
			}
		case ids["plain"]:
			if m.Group != "waiting_you" || !slices.Contains(m.Actions, "reply") {
				t.Fatalf("plain reply missing: %+v", m)
			}
		}
		if m.ReadAt != nil {
			t.Fatal("listing marked a letter read")
		}
	}
	for _, id := range []uuid.UUID{ids["plain"], ids["expired"]} {
		if _, err := events.NewStore(f.pool).Append(ctx, f.destID, events.StreamProvince, "MessengerArrived", map[string]any{"messenger_id": id}, f.worldID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='returning' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}
	_, archived := get(f.recvToken)
	if len(archived.Items) != 7 {
		t.Fatal("returning bearer erased delivered correspondence")
	}
	for _, m := range archived.Items {
		if m.ID == ids["plain"] && (m.Group != "completed" || slices.Contains(m.Actions, "reply")) {
			t.Fatalf("departed runner still accepts reply: %+v", m)
		}
	}
	_, other := get(f.senderToken)
	if len(other.Items) != 0 {
		t.Fatal("sender received recipient queue")
	}
}

func TestCorrespondenceQueueKeysetSurvivesEqualTimesNewMailAndResolution(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	clk := clock.NewTestClock(now)
	h := NewMessengerHandler(f.pool, nil, clk, nil)
	f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	insert := func(at time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'letter','delivered',3,0,$5,$5) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, at).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	want := map[uuid.UUID]bool{}
	for i := 0; i < 35; i++ {
		want[insert(now)] = true
	}
	pendingTx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pendingTx.Rollback(ctx)
	if _, err := pendingTx.Exec(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'in flight TX','delivered',3,0,$5,$5)`, f.worldID, f.senderID, f.originID, f.destID, now); err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/messengers/queue?limit=7"
	get := func(path, token string) (*httptest.ResponseRecorder, queueTestPage) {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		var p queueTestPage
		if rec.Code == 200 {
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
		}
		return rec, p
	}
	seen := map[uuid.UUID]bool{}
	var cursor string
	for pageNum := 0; pageNum < 6; pageNum++ {
		rec, p := get(path, f.recvToken)
		if rec.Code != 200 {
			t.Fatalf("page %d: %d %s", pageNum, rec.Code, rec.Body)
		}
		if len(p.Items) != 7 {
			t.Fatalf("page %d has %d rows", pageNum, len(p.Items))
		}
		for _, m := range p.Items {
			if !want[m.ID] || seen[m.ID] {
				t.Fatalf("insert appeared or repeated %s", m.ID)
			}
			seen[m.ID] = true
		}
		if pageNum == 0 {
			cursor = p.Next
			if err := pendingTx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			insert(now.Add(time.Second))
			insert(now) // even backdated/equal-time new mail belongs to a refresh
			// Changing action state must not move a row around the cursor.
			if _, err := f.pool.Exec(ctx, `UPDATE messengers SET read_at=$2 WHERE world_id=$1`, f.worldID, now); err != nil {
				t.Fatal(err)
			}
		}
		if p.Next == "" {
			break
		}
		path = "/worlds/" + f.worldID.String() + "/messengers/queue?limit=7&cursor=" + url.QueryEscape(p.Next)
	}
	if len(seen) != 35 {
		t.Fatalf("truncated queue %d/35", len(seen))
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?limit=x", "?cursor=garbage"} {
		rec, _ := get("/worlds/"+f.worldID.String()+"/messengers/queue"+q, f.recvToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad query %s: %d", q, rec.Code)
		}
	}
	rec, _ := get("/worlds/"+f.worldID.String()+"/messengers/queue?cursor="+url.QueryEscape(cursor), f.senderToken)
	if rec.Code != 400 {
		t.Fatalf("cross-player cursor %d", rec.Code)
	}
	// Exercise missing auth against the exact new handler route.
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue", nil))
	if rec.Code != 401 {
		t.Fatalf("unauthenticated %d", rec.Code)
	}
}

func TestCorrespondenceQueueDeadlineBlocksLateAcceptAndDeclineBeforeWorker(t *testing.T) {
	for _, verb := range []string{"trade-accept", "trade-decline"} {
		for _, kind := range []string{"buy", "sell"} {
			t.Run(verb+"/"+kind, func(t *testing.T) {
				f := setupReplyFixture(t)
				ctx := context.Background()
				now := time.Now().UTC().Truncate(time.Microsecond)
				clk := clock.NewTestClock(now)
				h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
				f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-accept", h.TradeAccept)
				f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-decline", h.TradeDecline)
				if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',100,0,1000,current_world_tick()),($1,'grain',100,0,1000,current_world_tick()),($2,'silver',100,0,1000,current_world_tick()),($2,'grain',100,0,1000,current_world_tick()) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=100,rate=0`, f.originID, f.destID); err != nil {
					t.Fatal(err)
				}
				var id uuid.UUID
				offer := fmt.Sprintf(`{"kind":%q,"want_good":"grain","want_qty":1,"offer_silver":1,"offer_good":"grain","offer_qty":1,"want_silver":1,"status":"pending"}`, kind)
				if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at,expires_at,trade_offer) VALUES($1,$2,$3,$4,'late offer','delivered',3,0,$5,$5,$5,$6::jsonb) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, now, offer).Scan(&id); err != nil {
					t.Fatal(err)
				}
				code, resp := f.post(t, f.recvToken, "/worlds/"+f.worldID.String()+"/messengers/"+id.String()+"/"+verb, map[string]any{})
				if code != 409 {
					t.Fatalf("deadline passed but %s %s returned %d: %v", kind, verb, code, resp)
				}
				var state string
				var goods float64
				var n int
				if err := f.pool.QueryRow(ctx, `SELECT trade_offer->>'status' FROM messengers WHERE id=$1`, id).Scan(&state); err != nil {
					t.Fatal(err)
				}
				if err := f.pool.QueryRow(ctx, `SELECT sum(amount) FROM settlement_goods WHERE settlement_id IN($1,$2) AND good_key IN('silver','grain')`, f.originID, f.destID).Scan(&goods); err != nil {
					t.Fatal(err)
				}
				if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1`, f.worldID).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if state != "pending" || goods != 400 || n != 0 {
					t.Fatalf("late request mutated state=%s goods=%g scheduled=%d", state, goods, n)
				}
			})
		}
	}
}

func TestCorrespondenceQueueMigrationBackfillsWithoutChangingLetters(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'preserve me','delivered',3,0,now(),now()) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, direction := range []string{"down", "up"} {
		raw, err := os.ReadFile("../../db/migrations/206_correspondence_membership." + direction + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	var text, status string
	var owner uuid.UUID
	var xid string
	if err := tx.QueryRow(ctx, `SELECT m.message_text,m.status,ds.owner_id,m.correspondence_created_xid::text FROM messengers m JOIN settlements ds ON ds.id=m.destination_id WHERE m.id=$1`, id).Scan(&text, &status, &owner, &xid); err != nil {
		t.Fatal(err)
	}
	if text != "preserve me" || status != "delivered" || owner != f.receiverID || xid == "" {
		t.Fatal("migration changed letter/authority or omitted backfill")
	}
	var after string
	if _, err := tx.Exec(ctx, `UPDATE messengers SET read_at=now(),status='returning' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT correspondence_created_xid::text FROM messengers WHERE id=$1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != xid {
		t.Fatal("read/status changed insertion identity")
	}
}

func TestCorrespondenceQueueLogicalRestoreDoesNotHideHistoricalLetters(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	h := NewMessengerHandler(f.pool, nil, clock.NewTestClock(now), nil)
	f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at,correspondence_created_xid,correspondence_created_epoch) VALUES($1,$2,$3,$4,'restored letter','delivered',3,0,$5,$5,'999999999999'::xid8,pg_postmaster_start_time()-interval '1 day') RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, now.Add(-time.Second)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue?limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+f.recvToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var p queueTestPage
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(p.Items) != 1 || p.Items[0].ID != id {
		t.Fatalf("logical restore hid delivered history: %d %s", rec.Code, rec.Body)
	}
	// Restarted DB processes invalidate snapshots, not the restored letters.
	cursor := correspondenceCursor{Version: 1, World: f.worldID, Player: f.receiverID, Upper: now, Before: now, ID: id, Snapshot: "1:2:", Epoch: now.Add(-24 * time.Hour)}
	raw, _ := json.Marshal(cursor)
	req = httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue?cursor="+base64.RawURLEncoding.EncodeToString(raw), nil)
	req.Header.Set("Authorization", "Bearer "+f.recvToken)
	rec = httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("old DB snapshot accepted: %d %s", rec.Code, rec.Body)
	}
}

func TestCorrespondenceQueueInsertionClockIsIndependentOfPausedGameClock(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	var id uuid.UUID
	// sent_at is database wall-time bookkeeping. The game's pause-adjusted
	// clock can be far behind it after downtime without hiding delivered mail.
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,arrives_at) VALUES($1,$2,$3,$4,'delivered after pause','delivered',3,0,now()) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewMessengerHandler(f.pool, nil, clock.NewTestClock(time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)), nil)
	f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue", nil)
	req.Header.Set("Authorization", "Bearer "+f.recvToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var page queueTestPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(page.Items) != 1 || page.Items[0].ID != id {
		t.Fatalf("game-clock pause hid delivered mail: %d %s", rec.Code, rec.Body)
	}
}
