package handlers

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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Exercise the production HTTP dispatch paths, not a hand-built envelope: each
// must opt in both the persisted row (passage resume) and the actual land job.
func TestOrderPursuitAPI_DispatchOptInPersists(t *testing.T) {
	for _, verb := range []string{"march", "recall", "redirect", "stance", "stance_pursuit"} {
		t.Run(verb, func(t *testing.T) {
			pool := unitLoadTestPool(t)
			ctx := context.Background()
			f, router := setupRecallCourierWorld(t)
			clk := clock.NewTestClock(time.Now())
			uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
			router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
			router.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
			if verb == "march" || verb == "stance" {
				if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',q=4,r=0,target_q=NULL,target_r=NULL,departs_at=NULL,arrives_at=NULL WHERE id=$1`, f.unitID); err != nil {
					t.Fatal(err)
				}
			}
			endpoint := "recall"
			body := map[string]any{}
			switch verb {
			case "march":
				endpoint = "march"
				body = map[string]any{"target_q": 2, "target_r": 1}
			case "redirect":
				body = map[string]any{"target_q": 2, "target_r": 1}
			case "stance", "stance_pursuit":
				endpoint = "stance"
				body = map[string]any{"stance": "storm"}
			}
			rec := postJSON(t, router, f.accessToken, "/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/"+endpoint, body)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("dispatch=%d %s", rec.Code, rec.Body.String())
			}
			var receipt struct {
				MessengerID uuid.UUID `json:"messenger_id"`
				Follow      bool      `json:"following_unit"`
				Legs        int       `json:"pursuit_legs"`
				Wait        bool      `json:"following_wait"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			if !receipt.Follow || receipt.Legs != 0 || receipt.Wait {
				t.Fatalf("receipt lost new land-order contract: %s", rec.Body.String())
			}
			var persisted, scheduled []byte
			var generation int
			if err := pool.QueryRow(ctx, `SELECT order_payload,passage_generation FROM messengers WHERE id=$1`, receipt.MessengerID).Scan(&persisted, &generation); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='OrderDeliveryV2' AND payload->>'messenger_id'=$1`, receipt.MessengerID.String()).Scan(&scheduled); err != nil {
				t.Fatal(err)
			}
			for _, arm := range []struct {
				name string
				raw  []byte
			}{{"persisted", persisted}, {"scheduled", scheduled}} {
				var payload map[string]any
				if err := json.Unmarshal(arm.raw, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["following_unit"] != true || payload["unit_id"] != f.unitID.String() || payload["verb"] != verb {
					t.Fatalf("%s payload lost pursuit identity: %s", arm.name, arm.raw)
				}
			}
			var job map[string]any
			json.Unmarshal(scheduled, &job)
			if job["passage_generation"] != float64(generation) {
				t.Fatalf("job generation %v != row %d", job["passage_generation"], generation)
			}
		})
	}
}

// ListUnits is the source used by Keryx and the War card. Exercise own/foreign
// payloads and each temporary waiting state through its real JSON response.
func TestOrderPursuitAPI_ListKeepsMetadataPrivateAndWaiting(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupRecallCourierWorld(t)
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	router.Get("/worlds/{worldID}/units", uh.ListUnits)
	rec := postJSON(t, router, f.accessToken, "/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", map[string]any{"target_q": 2, "target_r": 1})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("dispatch %d %s", rec.Code, rec.Body.String())
	}
	var receipt struct {
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(pool, "test-secret")
	otherToken, _, err := authSvc.Register(ctx, "pursuit-other-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := authSvc.ValidateAccessToken(otherToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messengers(world_id,sender_id,origin_unit_id,origin_q,origin_r,message_text,status,kind,hex_q,hex_r,dest_q,dest_r,arrives_at,order_payload)
 SELECT world_id,$2, $3,0,0,'foreign','outbound','order',0,0,2,0,arrives_at,order_payload || '{"pursuit_legs":99,"following_wait":true}'::jsonb FROM messengers WHERE id=$1`, receipt.MessengerID, claims.PlayerID, f.unitID); err != nil {
		t.Fatal(err)
	}
	get := func(token string) []struct {
		ID     uuid.UUID        `json:"id"`
		Orders []map[string]any `json:"orders_en_route"`
	} {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("list %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			Units []struct {
				ID     uuid.UUID        `json:"id"`
				Orders []map[string]any `json:"orders_en_route"`
			} `json:"units"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Units
	}
	for _, state := range []string{"travel", "waiting", "awaiting_passage", "returning_sealed"} {
		t.Run(state, func(t *testing.T) {
			var passage any
			if state != "travel" && state != "waiting" {
				passage = state
			}
			if _, err := pool.Exec(ctx, `UPDATE messengers SET order_payload=order_payload || jsonb_build_object('pursuit_legs',2,'following_wait',$2::boolean),passage_status=$3 WHERE id=$1`, receipt.MessengerID, state == "waiting", passage); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, u := range get(f.accessToken) {
				if u.ID != f.unitID {
					continue
				}
				found = true
				if len(u.Orders) != 1 {
					t.Fatalf("foreign order leaked or own lost: %+v", u.Orders)
				}
				o := u.Orders[0]
				if o["following_unit"] != true || o["pursuit_legs"] != float64(2) || o["following_wait"] != (state != "travel") || o["target_q"] != float64(2) || o["target_r"] != float64(1) {
					t.Fatalf("metadata/intent lost: %+v", o)
				}
			}
			if !found {
				t.Fatal("own unit absent")
			}
			if len(get(otherToken)) != 0 {
				t.Fatal("foreign player sees owned units/order metadata")
			}
		})
	}
}

func TestOrderPursuitAPI_MapMetadataOnlyForOwner(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	f, _ := setupRecallCourierWorld(t)
	authSvc := auth.NewService(pool, "test-secret")
	now := time.Now().UTC().Truncate(time.Second)
	viewerID, viewerToken := registerViewer(t, ctx, authSvc, "pursuit-viewer")
	viewerProv := insertProvince(t, ctx, pool, f.worldID, 1, 1)
	insertSettlement(t, ctx, pool, f.worldID, viewerProv, "Viewer", viewerID, true)
	var mid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_unit_id,origin_q,origin_r,message_text,status,kind,hex_q,hex_r,dest_q,dest_r,sent_at,arrives_at,order_payload)
 VALUES($1,$2,$3,0,0,'private order','outbound','order',0,0,2,0,$4,$5,jsonb_build_object('unit_id',$3::uuid::text,'verb','redirect','following_unit',true,'pursuit_legs',2,'following_wait',true)) RETURNING id`, f.worldID, f.playerID, f.unitID, now.Add(-time.Hour), now.Add(time.Hour)).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(now))
	// Use raw JSON: omitted secrets must be absent, rather than decoding to zero.
	for _, arm := range []struct {
		name, token string
		own         bool
	}{{"owner", f.accessToken, true}, {"foreign visible", viewerToken, false}} {
		t.Run(arm.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(auth.Middleware(authSvc))
			r.Get("/worlds/{worldID}/messengers", wh.MapMessengers)
			req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/messengers", nil)
			req.Header.Set("Authorization", "Bearer "+arm.token)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("map %d %s", rec.Code, rec.Body.String())
			}
			var out []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			var marker map[string]any
			for _, m := range out {
				if m["id"] == mid.String() {
					marker = m
				}
			}
			if marker == nil {
				t.Fatalf("runner must actually be visible to %s, got %s", arm.name, rec.Body.String())
			}
			if marker["own"] != arm.own {
				t.Fatalf("wrong own marker %+v", marker)
			}
			if arm.own {
				if marker["order_unit_id"] != f.unitID.String() || marker["following_unit"] != true || marker["pursuit_legs"] != float64(2) || marker["following_wait"] != true {
					t.Fatalf("owner metadata missing %+v", marker)
				}
			} else {
				for _, field := range []string{"order_unit_id", "following_unit", "pursuit_legs", "following_wait"} {
					if _, ok := marker[field]; ok {
						t.Fatalf("foreign marker leaked %s: %+v", field, marker)
					}
				}
			}
		})
	}
}
