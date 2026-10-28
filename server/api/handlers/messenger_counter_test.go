package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

func counterFixture(t *testing.T) (*replyFixture, *events.Scheduler, *events.Store) {
	t.Helper()
	f, sched, store := integrationTradeFixture(t)
	h := NewMessengerHandler(f.pool, sched, clock.NewTestClock(time.Now()), nil)
	f.router.Post("/worlds/{worldID}/messengers/{messengerID}/trade-counter", h.TradeCounter)
	f.router.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", h.ListSent)
	return f, sched, store
}
func counterTerms(kind string) map[string]any {
	if kind == "buy" {
		return map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": 2, "want_silver": 7}
	}
	return map[string]any{"kind": "buy", "want_good": "copper", "want_qty": 2, "offer_silver": 7}
}
func counterPost(t *testing.T, f *replyFixture, old, kind string) (int, map[string]any) {
	t.Helper()
	return f.post(t, f.recvToken, "/worlds/"+f.worldID.String()+"/messengers/"+old+"/trade-counter", map[string]any{"message": "A concrete counteroffer", "trade_offer": counterTerms(kind)})
}
func counterStock(t *testing.T, f *replyFixture, city uuid.UUID, good string) float64 {
	t.Helper()
	var n float64
	if err := f.pool.QueryRow(context.Background(), `SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, city, good).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func counterSnapshot(t *testing.T, f *replyFixture) string {
	t.Helper()
	var raw []byte
	err := f.pool.QueryRow(context.Background(), `SELECT json_build_object('messengers',(SELECT json_agg(row_to_json(m) ORDER BY id) FROM messengers m WHERE world_id=$1),'goods',(SELECT json_agg(row_to_json(g) ORDER BY settlement_id,good_key) FROM settlement_goods g WHERE settlement_id IN($2,$3)),'scheduled',(SELECT count(*) FROM scheduled_events WHERE world_id=$1),'events',(SELECT count(*) FROM events WHERE world_id=$1),'archive',(SELECT json_agg(row_to_json(a) ORDER BY id) FROM correspondence_letters a WHERE id IN(SELECT id FROM messengers WHERE world_id=$1)))`, f.worldID, f.originID, f.destID).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func counterHTTPGet(t *testing.T, f *replyFixture, token, path string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET %s=%d %s", path, rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCounterOfferAtomicEscrowPhysicalReturnAndIndependentConsent(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			f, sched, store := counterFixture(t)
			old, caseID, original := integrationSendOffer(t, f, store, sched, kind, "Negotiation")
			oldGood, oldHeld := "silver", float64(5)
			newGood, newHeld := "copper", float64(2)
			if kind == "sell" {
				oldGood, oldHeld = "copper", 2
				newGood, newHeld = "silver", 7
			}
			oldAmount := counterStock(t, f, f.originID, oldGood)
			newAmount := counterStock(t, f, f.destID, newGood)
			code, v := counterPost(t, f, old, kind)
			if code != 201 {
				t.Fatalf("atomic counter: %d %v", code, v)
			}
			next := v["id"].(string)
			if v["case_id"] != caseID {
				t.Fatalf("counter left original case: %v", v)
			}
			var parent, status, oldStatus, refund string
			var origin, dest uuid.UUID
			if err := f.pool.QueryRow(context.Background(), `SELECT trade_offer->>'counter_offer_id',trade_offer->>'status',origin_id,destination_id FROM messengers WHERE id=$1`, next).Scan(&parent, &status, &origin, &dest); err != nil {
				t.Fatal(err)
			}
			if parent != old || status != "pending" || origin != f.destID || dest != f.originID {
				t.Fatalf("bad linked counter: %s %s %s %s", parent, status, origin, dest)
			}
			if err := f.pool.QueryRow(context.Background(), `SELECT trade_offer->>'status',trade_offer->>'refund_on_return' FROM messengers WHERE id=$1`, old).Scan(&oldStatus, &refund); err != nil {
				t.Fatal(err)
			}
			if oldStatus != "declined" || refund != "true" {
				t.Fatalf("original reservation not carried home: %s/%s", oldStatus, refund)
			}
			if got := counterStock(t, f, f.originID, oldGood); got != oldAmount {
				t.Fatalf("old escrow refunded before bearer returned: %g != %g", got, oldAmount)
			}
			if got := counterStock(t, f, f.destID, newGood); got != newAmount-newHeld {
				t.Fatalf("new escrow not reserved exactly once: %g", got)
			}
			var audits int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE world_id=$1 AND event_type='TradeOfferCounteredV1' AND payload->>'old_messenger_id'=$2 AND payload->>'messenger_id'=$3`, f.worldID, old, next).Scan(&audits); err != nil || audits != 1 {
				t.Fatalf("counter audit=%d err=%v", audits, err)
			}
			// Both outgoing readers freeze the old outcome until the original bearer is home.
			before := integrationArchiveOffer(t, f, f.senderToken, caseID)
			if !reflect.DeepEqual(before, original) {
				t.Fatalf("old sender learned counter resolution remotely: %v", before)
			}
			sent := counterHTTPGet(t, f, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers")
			if !reflect.DeepEqual(sent[0]["trade_offer"], original) {
				t.Fatalf("legacy outbox leaked remote counter: %v", sent)
			}
			// A retry cannot reserve another payment or create another bearer/event.
			snapshot := counterSnapshot(t, f)
			code, v = counterPost(t, f, old, kind)
			if code != 409 {
				t.Fatalf("duplicate counter: %d %v", code, v)
			}
			if counterSnapshot(t, f) != snapshot {
				t.Fatal("duplicate counter changed state")
			}
			// Return is the existing physical refund owner and remains idempotent.
			ret := integrationEvent(t, f, old, events.ScheduledMessengerReturn)
			rh := messenger.NewReturnHandler(f.pool, store, nil)
			if err := rh.Handle(context.Background(), ret); err != nil {
				t.Fatal(err)
			}
			if err := rh.Handle(context.Background(), ret); err != nil {
				t.Fatal(err)
			}
			if got := counterStock(t, f, f.originID, oldGood); got != oldAmount+oldHeld {
				t.Fatalf("old escrow refund was lost or doubled: %g", got)
			}
			// New offer still requires physical delivery and a separate explicit accept.
			code, v = f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/messengers/"+next+"/trade-accept", map[string]any{})
			if code != 404 {
				t.Fatalf("undelivered counter accepted: %d %v", code, v)
			}
			arrival := integrationEvent(t, f, next, events.ScheduledMessengerArrival)
			if err := messenger.NewArrivalHandler(f.pool, sched, store, nil).Handle(context.Background(), arrival); err != nil {
				t.Fatal(err)
			}
			code, v = f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/messengers/"+next+"/trade-accept", map[string]any{})
			if code != 200 {
				t.Fatalf("counter explicit accept: %d %v", code, v)
			}
		})
	}
}

func TestCounterOfferRollsBackLateDispatchAndEscrowFailures(t *testing.T) {
	for _, failure := range []string{"new-escrow", "late-audit"} {
		t.Run(failure, func(t *testing.T) {
			f, sched, store := counterFixture(t)
			old, _, _ := integrationSendOffer(t, f, store, sched, "buy", "Rollback")
			if failure == "new-escrow" {
				if _, err := f.pool.Exec(context.Background(), `UPDATE settlement_goods SET amount=1,rate=0 WHERE settlement_id=$1 AND good_key='copper'`, f.destID); err != nil {
					t.Fatal(err)
				}
			} else {
				name := "counter_audit_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
				if _, err := f.pool.Exec(context.Background(), fmt.Sprintf(`ALTER TABLE events ADD CONSTRAINT %s CHECK(world_id<>'%s' OR event_type<>'TradeOfferCounteredV1')`, name, f.worldID)); err != nil {
					t.Fatal(err)
				}
				defer f.pool.Exec(context.Background(), `ALTER TABLE events DROP CONSTRAINT IF EXISTS `+name)
			}
			before := counterSnapshot(t, f)
			code, v := counterPost(t, f, old, "buy")
			want := 422
			if failure == "late-audit" {
				want = 500
			}
			if code != want {
				t.Fatalf("forced %s=%d %v want%d", failure, code, v, want)
			}
			if after := counterSnapshot(t, f); after != before {
				t.Fatalf("failed counter left partial writes or reservation: before=%s after=%s", before, after)
			}
		})
	}
}

func TestCounterOfferDeadlineAuthorityAndCaseGuards(t *testing.T) {
	for _, bad := range []string{"expired", "wrong-person", "captured-destination", "captured-origin", "wrong-case", "plain-letter"} {
		t.Run(bad, func(t *testing.T) {
			f, sched, store := counterFixture(t)
			old, _, _ := integrationSendOffer(t, f, store, sched, "buy", "Guards")
			token := f.recvToken
			body := map[string]any{"message": "Counter", "trade_offer": counterTerms("buy")}
			want := 409
			switch bad {
			case "expired":
				_, _ = f.pool.Exec(context.Background(), `UPDATE messengers SET expires_at=now()-interval '1 hour' WHERE id=$1`, old)
			case "wrong-person":
				token = f.senderToken
				want = 404
			case "captured-destination":
				_, _ = f.pool.Exec(context.Background(), `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, f.senderID)
				token = f.senderToken
				want = 404
			case "captured-origin":
				want = 422
				_, _ = f.pool.Exec(context.Background(), `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.originID, f.receiverID)
			case "wrong-case":
				body["case_id"] = uuid.NewString()
			case "plain-letter":
				delete(body, "trade_offer")
				want = 400
			}
			before := counterSnapshot(t, f)
			code, v := f.post(t, token, "/worlds/"+f.worldID.String()+"/messengers/"+old+"/trade-counter", body)
			if code != want {
				t.Fatalf("%s guard=%d %v want%d", bad, code, v, want)
			}
			if after := counterSnapshot(t, f); after != before {
				t.Fatalf("%s guard changed original offer", bad)
			}
		})
	}
}

func TestCounterOfferSerializesConcurrentAcceptAndDuplicate(t *testing.T) {
	for _, rival := range []string{"accept", "counter"} {
		t.Run(rival, func(t *testing.T) {
			f, sched, store := counterFixture(t)
			old, _, _ := integrationSendOffer(t, f, store, sched, "buy", "Race")
			gate := make(chan struct{})
			codes := make(chan int, 2)
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-gate
					path := "/worlds/" + f.worldID.String() + "/messengers/" + old + "/trade-counter"
					body := map[string]any{"message": "Race counter", "trade_offer": counterTerms("buy")}
					if i == 1 && rival == "accept" {
						path = "/worlds/" + f.worldID.String() + "/messengers/" + old + "/trade-accept"
						body = map[string]any{}
					}
					code, _ := f.post(t, f.recvToken, path, body)
					codes <- code
				}(i)
			}
			close(gate)
			wg.Wait()
			close(codes)
			wins, conflicts := 0, 0
			for code := range codes {
				if code == http.StatusCreated || code == http.StatusOK {
					wins++
				} else if code == http.StatusConflict {
					conflicts++
				} else {
					t.Errorf("unexpected concurrent result %d", code)
				}
			}
			if wins != 1 || conflicts != 1 {
				t.Fatalf("race accepted multiple final actions: wins=%d conflicts=%d", wins, conflicts)
			}
			var children int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM messengers WHERE trade_offer->>'counter_offer_id'=$1`, old).Scan(&children); err != nil {
				t.Fatal(err)
			}
			if children > 1 {
				t.Fatal("multiple counter bearers for one original")
			}
		})
	}
}

func TestCounterOfferTxRechecksOriginalSenderOwnershipAndExactPair(t *testing.T) {
	f, sched, store := counterFixture(t)
	old, caseID, _ := integrationSendOffer(t, f, store, sched, "buy", "Locked identity")
	ctx := context.Background()
	oldID := uuid.MustParse(old)
	for _, bad := range []string{"pair", "owner"} {
		t.Run(bad, func(t *testing.T) {
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			origin, dest := f.destID, f.originID
			if bad == "pair" {
				origin, dest = dest, origin
			} else {
				if _, err = tx.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.originID, f.receiverID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = lockCounterOfferTx(ctx, tx, f.worldID, oldID, f.receiverID, origin, dest, caseID, time.Now()); err == nil {
				t.Fatal("locked counter accepted a changed original party or city pair")
			}
		})
	}
}

// The replacement step must refuse an original that is no longer pending even
// if a caller skipped lockCounterOfferTx: the guarded UPDATE is the backstop.
func TestCounterOfferFinishRefusesNonPendingOriginal(t *testing.T) {
	f, sched, store := counterFixture(t)
	old, caseID, _ := integrationSendOffer(t, f, store, sched, "buy", "Already answered")
	ctx := context.Background()
	oldID := uuid.MustParse(old)
	caseUUID, _ := uuid.Parse(caseID)
	h := NewMessengerHandler(f.pool, sched, clock.NewTestClock(time.Now()), nil)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE messengers SET trade_offer=trade_offer||'{"status":"accepted"}'::jsonb WHERE id=$1`, oldID); err != nil {
		t.Fatal(err)
	}
	err = h.finishCounterOfferTx(ctx, tx, f.worldID, oldID, uuid.New(), f.receiverID, f.destID, f.originID, caseUUID, []byte(`{}`))
	if !errors.Is(err, errCounterUnavailable) {
		t.Fatalf("finish replaced an original that was no longer pending: %v", err)
	}
}
