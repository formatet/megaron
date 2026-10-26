package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGossipShowsTrueOmensAndOthersTradeOnly(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		text, category string
		facts          any
		own            bool
	}{
		{"old atmospheric omen", "omen", nil, false}, {"true storm omen", "omen", `{"true":true,"category":"storm","precision":1}`, false},
		{"my own tin", "economy", nil, true}, {"other tin", "economy", nil, false},
	} {
		var owner any
		if tc.own {
			owner = f.playerA
		} else if tc.category == "economy" {
			owner = f.playerB
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO gossip_events(world_id,recipient_id,source_region,category,text,fact_data,transport_owner_id) VALUES($1,$2,'Here',$3,$4,$5,$6)`, f.worldID, f.playerA, tc.category, tc.text, tc.facts, owner); err != nil {
			t.Fatal(err)
		}
	}
	h := NewSettlementHandler(f.pool, nil, nil, clock.NewTestClock(time.Now()))
	f.router.Get("/worlds/{worldID}/gossip", h.Gossip)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/gossip", nil)
	req.Header.Set("Authorization", "Bearer "+f.tokenA)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var rows []struct {
		Text string         `json:"text"`
		Omen map[string]any `json:"omen"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("feed leaked own trade or unsupported old omen: %s", rec.Body.String())
	}
	for _, row := range rows {
		if row.Text == "true storm omen" && row.Omen["true"] != true {
			t.Fatal("omen lost actual outcome metadata")
		}
		if row.Text != "true storm omen" && row.Text != "other tin" {
			t.Fatalf("wrong feed row: %+v", row)
		}
	}
}
