package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/economy"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func giftFixture(t *testing.T, hidden bool) *tradeInternalFixture {
	t.Helper()
	f := setupTradeInternalFixture(t)
	ctx := context.Background()
	var recipient uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "recipient-"+uuid.NewString()).Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, recipient); err != nil {
		t.Fatal(err)
	}
	if hidden {
		if _, err := f.pool.Exec(ctx, `UPDATE provinces SET map_q=20 WHERE id=(SELECT province_id FROM settlements WHERE id=$1)`, f.destID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) SELECT $1,q,0,'plains' FROM generate_series(4,20) q`, f.worldID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestGiftContactedForeignCityDispatch(t *testing.T) {
	f := giftFixture(t, false)
	code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
	if code != 201 {
		t.Fatalf("gift to contacted foreign city returned %d: %v, want 201", code, body)
	}
	var amount float64
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.originID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 99950 {
		t.Fatalf("origin amount=%v, want 99950", amount)
	}
	var interceptable bool
	if err := f.pool.QueryRow(context.Background(), `SELECT interceptable FROM transports WHERE id=$1`, body["transport_id"]).Scan(&interceptable); err != nil {
		t.Fatal(err)
	}
	if !interceptable {
		t.Fatal("gift caravan cannot be raided")
	}
}

func TestGiftHiddenForeignCityDeniedWithoutDebit(t *testing.T) {
	f := giftFixture(t, true)
	code, body := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 50})
	if code != 403 {
		t.Fatalf("hidden destination returned %d: %v, want 403", code, body)
	}
	var amount float64
	var movers int
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.originID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&movers); err != nil {
		t.Fatal(err)
	}
	if amount != 100000 || movers != 0 {
		t.Fatalf("refusal changed amount=%v movers=%d", amount, movers)
	}
}

func TestGiftDestinationListMatchesLetterFOW(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "contacted", true: "hidden"}[hidden], func(t *testing.T) {
			f := giftFixture(t, hidden)
			ph := NewProvinceHandler(f.pool, f.scheduler, f.clk, economy.SitosConfig{}, nil, nil)
			f.router.Get("/worlds/{worldID}/provinces/{provinceID}/trade/destinations", ph.TransferDestinations)
			request := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade/destinations", nil)
			request.Header.Set("Authorization", "Bearer "+f.token)
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("destinations %d: %s", response.Code, response.Body.String())
			}
			var rows []struct {
				ID        uuid.UUID `json:"settlement_id"`
				Owner     uuid.UUID `json:"owner_id"`
				Name      string    `json:"name"`
				OwnerName string    `json:"owner_name"`
				Own       bool      `json:"own"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			if hidden {
				if len(rows) != 0 {
					t.Fatalf("hidden destination leaked: %+v", rows)
				}
				return
			}
			if len(rows) != 1 || rows[0].ID != f.destID || rows[0].Own || rows[0].Owner == f.playerID || rows[0].Name != "Dest" || rows[0].OwnerName == "" {
				t.Fatalf("foreign destination lacks city/owner identity: %+v", rows)
			}
		})
	}
}
