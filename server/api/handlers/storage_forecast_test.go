package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStorageForecastIsOnlyExposedToAuthenticatedOwner(t *testing.T) {
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	ctx := context.Background()
	svc := auth.NewService(pool, "storage-private-test")
	ownToken, _, err := svc.Register(ctx, "storage-owner-"+uuid.NewString(), "strong-password-test")
	if err != nil {
		t.Fatal(err)
	}
	otherToken, _, err := svc.Register(ctx, "storage-other-"+uuid.NewString(), "strong-password-test")
	if err != nil {
		t.Fatal(err)
	}
	own, err := svc.ValidateAccessToken(ownToken)
	if err != nil {
		t.Fatal(err)
	}
	var wid, pid uuid.UUID
	if err := pool.QueryRow(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1 RETURNING world_id,province_id`, sid, own.PlayerID).Scan(&wid, &pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE players SET agora_exempt=true WHERE id=$1`, own.PlayerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0,cap=60,calc_tick=0 WHERE settlement_id=$1 AND good_key='silver'`, sid); err != nil {
		t.Fatal(err)
	}
	tid := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO transports(id,world_id,owner_id,kind,dest_id,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,status,interceptable) VALUES($1,$2,$3,'internal_transfer',$4,'land',0,0,0,0,now(),now(),20,'in_transit',true)`, tid, wid, own.PlayerID, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES($1,'silver',150)`, tid); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Unix(0, 0)), nil, nil)
	router := chi.NewRouter()
	router.Use(auth.Middleware(svc))
	router.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	for _, tc := range []struct {
		name, token string
		visible     bool
	}{{"owner", ownToken, true}, {"other", otherToken, false}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/provinces/"+pid.String(), nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("Get status%d body%s", rec.Code, rec.Body.String())
			}
			var result struct {
				Settlement struct {
					Storage []economy.StorageForecast `json:"storage"`
				} `json:"settlement"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if !tc.visible {
				if result.Settlement.Storage != nil {
					t.Fatal("foreign storage/known transport forecast exposed")
				}
				return
			}
			found := false
			for _, f := range result.Settlement.Storage {
				if f.GoodKey == "silver" {
					found = true
					if !f.Warning || f.Incoming != 150 || f.FullInTicks == nil || *f.FullInTicks != 20 {
						t.Fatalf("server forecast=%+v", f)
					}
				}
			}
			if !found {
				t.Fatal("missing own silver forecast")
			}
		})
	}
}
