package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

func TestReturnPostArrivalAndNextTickUseSameBearer(t *testing.T) {
	for _, offset := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f := setupReplyFixture(t)
			ctx := context.Background()
			now := time.Now().UTC()
			clk := clock.NewTestClock(now)
			sched := events.NewScheduler(f.pool, clk)
			h := NewMessengerHandler(f.pool, sched, clk, nil)
			// Use a separate route to ensure this test's controlled clock is injected.
			f.router.Post("/return/{worldID}/{messengerID}", h.Reply)
			f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
			f.router.Get("/worlds/{worldID}/messengers/inbox", h.Inbox)
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=100,last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
				t.Fatal(err)
			}
			var id uuid.UUID
			if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'return post','outbound',3,0,$5,$5) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, now).Scan(&id); err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(messenger.ArrivalPayload{MessengerID: id})
			if err := messenger.NewArrivalHandler(f.pool, sched, events.NewStore(f.pool), nil).Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 100, Payload: payload}); err != nil {
				t.Fatal(err)
			}
			var due int
			if err := f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE world_id=$1 AND event_type='MessengerStayEnd' AND payload->>'messenger_id'=$2`, f.worldID, id.String()).Scan(&due); err != nil {
				t.Fatal(err)
			}
			if due != 102 {
				t.Fatalf("return post departure=%d want102 (arrival and following tick)", due)
			}
			// Do not run StayEnd: even a delayed worker cannot extend the reply window.
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, 100+offset); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/queue", nil)
			req.Header.Set("Authorization", "Bearer "+f.recvToken)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			var page struct {
				Items []struct {
					ID          uuid.UUID  `json:"id"`
					Actions     []string   `json:"actions"`
					ReplyBefore *time.Time `json:"reply_before"`
				} `json:"items"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 || page.Items[0].ReplyBefore == nil || slices.Contains(page.Items[0].Actions, "reply") != (offset < 2) {
				t.Fatalf("queue disagrees with reply window: %s", rec.Body)
			}
			for _, history := range []bool{false, true} {
				path := "/worlds/" + f.worldID.String() + "/messengers/inbox"
				if history {
					path += "?history=true"
				}
				req := httptest.NewRequest("GET", path, nil)
				req.Header.Set("Authorization", "Bearer "+f.recvToken)
				rec := httptest.NewRecorder()
				f.router.ServeHTTP(rec, req)
				var inbox []struct {
					ReplyWindowClosed bool `json:"reply_window_closed"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &inbox); err != nil {
					t.Fatal(err)
				}
				if offset == 2 && !history {
					if len(inbox) != 0 {
						t.Fatal("closed reply in actionable inbox")
					}
				} else if len(inbox) != 1 || inbox[0].ReplyWindowClosed != (offset >= 2) {
					t.Fatalf("history/current window wrong: %s", rec.Body)
				}
			}
			code, resp := f.post(t, f.recvToken, "/return/"+f.worldID.String()+"/"+id.String(), map[string]string{"reply": "With your own bearer"})
			if offset == 2 {
				if code != 409 {
					t.Fatalf("late reply %d %v", code, resp)
				}
			} else if code != 200 {
				t.Fatalf("in-window reply %d %v", code, resp)
			}
			var count int
			var status string
			var reply *string
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id=$1`, f.worldID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(ctx, `SELECT status,reply_text FROM messengers WHERE id=$1`, id).Scan(&status, &reply); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatal("reply created another messenger")
			}
			if offset == 2 {
				if status != "delivered" || reply != nil {
					t.Fatal("late reply changed bearer")
				}
			} else {
				if status != "returning" || reply == nil || *reply != "With your own bearer" {
					t.Fatal("reply did not travel on same bearer")
				}
			}
			if offset < 2 {
				// The stale stay-end retry cannot erase or replace the reply.
				stay, _ := json.Marshal(messenger.StayEndPayload{MessengerID: id})
				if err := messenger.NewStayEndHandler(f.pool, sched, clk).Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 102, Payload: stay}); err != nil {
					t.Fatal(err)
				}
				var returned string
				if err := f.pool.QueryRow(ctx, `SELECT reply_text FROM messengers WHERE id=$1`, id).Scan(&returned); err != nil || returned != "With your own bearer" {
					t.Fatal("stay retry overwrote reply")
				}
			}
		})
	}
}

func TestReturnPostHonorsPreviouslyIssuedStayDeadline(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	now := time.Now()
	clk := clock.NewTestClock(now)
	sched := events.NewScheduler(f.pool, clk)
	h := NewMessengerHandler(f.pool, sched, clk, nil)
	f.router.Post("/return/{worldID}/{messengerID}", h.Reply)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=110 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'legacy stay','delivered',3,0,$5,$5) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := sched.EnqueueTick(ctx, f.worldID, events.ScheduledMessengerStayEnd, messenger.StayEndPayload{MessengerID: id}, 148); err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, f.recvToken, "/return/"+f.worldID.String()+"/"+id.String(), map[string]string{"reply": "Saved old deadline"})
	if code != 200 {
		t.Fatalf("shortened existing stay %d %v", code, resp)
	}
}

func TestReturnPostConcurrentRepliesClaimSingleBearer(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()
	now := time.Now()
	clk := clock.NewTestClock(now)
	sched := events.NewScheduler(f.pool, clk)
	h := NewMessengerHandler(f.pool, sched, clk, nil)
	f.router.Post("/return/{worldID}/{messengerID}", h.Reply)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=101 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'one bearer','delivered',3,0,$5,$5) RETURNING id`, f.worldID, f.senderID, f.originID, f.destID, now).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := sched.EnqueueTick(ctx, f.worldID, events.ScheduledMessengerStayEnd, messenger.StayEndPayload{MessengerID: id}, 102); err != nil {
		t.Fatal(err)
	}
	responses := make(chan int, 2)
	start := make(chan struct{})
	for _, body := range []string{`{"reply":"first"}`, `{"reply":"second"}`} {
		go func(body string) {
			<-start
			req := httptest.NewRequest("POST", "/return/"+f.worldID.String()+"/"+id.String(), strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+f.recvToken)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			responses <- rec.Code
		}(body)
	}
	close(start)
	winners := 0
	for i := 0; i < 2; i++ {
		code := <-responses
		if code == 200 {
			winners++
		} else if code != 403 && code != 409 {
			t.Fatalf("concurrent reply failed %d", code)
		}
	}
	if winners != 1 {
		t.Fatalf("reply winners=%d want1", winners)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='MessengerReturn' AND payload->>'messenger_id'=$2`, f.worldID, id.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("return journeys=%d want1", n)
	}
}
