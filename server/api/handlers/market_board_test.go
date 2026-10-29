package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/events"
)

func (f *originEyeFixture) put(t *testing.T, token, path string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func boardFixture(t *testing.T) *originEyeFixture {
	f := setupOriginEyeFixture(t)
	sh := NewSettlementHandler(f.pool, events.NewStore(f.pool), nil, f.clk)
	f.router.Put("/worlds/{worldID}/settlements/{settlementID}/market-board", sh.MarketBoard)
	f.router.Get("/worlds/{worldID}/settlements/{settlementID}", sh.Get)
	return f
}

func TestMarketBoardPutRules(t *testing.T) {
	f := boardFixture(t)
	ctx := context.Background()
	owner, ownerToken := f.registerPlayer(t, "board-owner")
	_, strangerToken := f.registerPlayer(t, "board-stranger")
	city := f.settlement(t, "Ugarit", 0, owner)
	path := "/worlds/" + f.worldID.String() + "/settlements/" + city.String() + "/market-board"

	if code, resp := f.put(t, ownerToken, path, map[string]any{"buys": []string{"tin"}}); code != 422 || !strings.Contains(resp["error"].(string), "Market") {
		t.Fatalf("board without a Market: %d %v, want 422 naming the Market", code, resp)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'market',1)`, city); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.put(t, strangerToken, path, map[string]any{"buys": []string{"tin"}}); code != 403 {
		t.Fatalf("stranger wrote the board: %d, want 403", code)
	}
	for _, bad := range []map[string]any{
		{"buys": []string{"tin", "copper", "wine", "oil"}},
		{"buys": []string{"tin"}, "sells": []string{"tin"}},
		{"buys": []string{"silver"}},
	} {
		if code, resp := f.put(t, ownerToken, path, bad); code != 422 {
			t.Fatalf("invalid board %v accepted: %d %v", bad, code, resp)
		}
	}
	if code, resp := f.put(t, ownerToken, path, map[string]any{"buys": []string{"tin"}, "sells": []string{"wine"}}); code != 200 {
		t.Fatalf("valid board refused: %d %v", code, resp)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='MarketBoardWritten'`, city).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("MarketBoardWritten events = %d, want 1", n)
	}
}

// A city heard of from a market board takes a letter, never a trade offer.
func TestHeardCityTakesLettersNotTradeOffers(t *testing.T) {
	f := boardFixture(t)
	ctx := context.Background()
	sender, token := f.registerPlayer(t, "heard-sender")
	other, _ := f.registerPlayer(t, "heard-other")
	origin := f.settlement(t, "Home", 0, sender)
	f.settlement(t, "Near", 3, other) // some foreign city in sight, so the coarse trade gate passes
	far := f.settlement(t, "Ugarit", 20, other)
	for q := 1; q < 20; q++ {
		if q != 3 {
			f.mapTile(t, q, 0, "plains")
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'silver',1000,0,5000,0)`, origin); err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/settlements/" + origin.String() + "/messengers"
	letter := map[string]any{"destination_id": far.String(), "message": "what do you need?"}
	if code, _ := f.post(t, token, path, letter); code != 403 {
		t.Fatalf("letter to an unknown city: %d, want 403", code)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO player_heard_tiles(world_id,player_id,q,r,source,heard_tick) VALUES($1,$2,20,0,'board',0)`, f.worldID, sender); err != nil {
		t.Fatal(err)
	}
	offer := map[string]any{"destination_id": far.String(), "message": "trade?",
		"trade_offer": map[string]any{"kind": "buy", "want_good": "tin", "want_qty": 1, "offer_silver": 1}}
	if code, resp := f.post(t, token, path, offer); code != 403 || !strings.Contains(resp["error"].(string), "only by word") {
		t.Fatalf("trade offer to a heard city: %d %v, want 403 'only by word'", code, resp)
	}
	if code, resp := f.post(t, token, path, letter); code != 201 {
		t.Fatalf("letter to a heard city refused: %d %v", code, resp)
	}
}
