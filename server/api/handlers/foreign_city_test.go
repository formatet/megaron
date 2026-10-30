package handlers

// Another ruler's city as the viewer knows it (megaron_kontrakt_frammande_stad,
// Timothy 2026-10-30). Real chi.Mux + auth.Middleware, gated by DATABASE_URL.
// Each check below is the named red test for one row of the contract's table.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// foreignWhitelist is every key a foreign settlement body may carry.
var foreignWhitelist = map[string]bool{
	"id": true, "name": true, "owner_id": true, "owner_name": true, "is_capital": true, "state": true,
	"walls": true, "size": true, "culture": true, "besieged": true, "besieged_by": true,
	"buildings": true, "intel": true, "market_board": true,
}

func TestForeignCity_LevelsAndWhitelist(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'archived', 40) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	token, _, err := authSvc.Register(ctx, "foreign-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	viewerID := claims.PlayerID
	newPlayer := func(wanax string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`,
			"p-"+uuid.New().String(), wanax,
		).Scan(&id); err != nil {
			t.Fatalf("create player: %v", err)
		}
		return id
	}
	far, other := newPlayer("Wanax Far"), newPlayer("Wanax Other")
	type cityIDs struct{ province, settlement uuid.UUID }
	city := func(q, r int, name string, owner uuid.UUID, pop int) cityIDs {
		var c cityIDs
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, q, r,
		).Scan(&c.province); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, population, wall_level)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 1) RETURNING id`,
			worldID, c.province, name, owner, pop,
		).Scan(&c.settlement); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		for _, g := range []struct {
			good string
			n    float64
		}{{"oil", 900}, {"wine", 10}, {"silver", 5000}, {"grain", 50}} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, $2, $3, 0, 1000, 0)`,
				c.settlement, g.good, g.n); err != nil {
				t.Fatalf("goods %s: %v", name, err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'silo', 2)`, c.settlement); err != nil {
			t.Fatalf("building %s: %v", name, err)
		}
		// The silo trigger lifts food caps; pin one store ceiling for the words.
		if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET cap = 1000 WHERE settlement_id = $1`, c.settlement); err != nil {
			t.Fatalf("cap %s: %v", name, err)
		}
		return c
	}
	remember := func(q, r int) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			worldID, viewerID, q, r); err != nil {
			t.Fatalf("remember: %v", err)
		}
		if err := province.RecordSeenSettlements(ctx, pool, worldID, viewerID, []province.MapPosition{{Q: q, R: r}}); err != nil {
			t.Fatalf("record seen: %v", err)
		}
	}

	// Viewer's capital at (0,0) sees 3 hexes over plains.
	city(0, 0, "Viewerton", viewerID, 2000)
	hidden := city(20, 0, "Hidden", far, 3000)
	near := city(2, 0, "Nearby", far, 3000)
	remembered := city(8, 0, "Remembered", far, 3000)
	remember(8, 0)
	marketon := city(12, 0, "Marketon", far, 3000)
	remember(12, 0)

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), nil, nil)
	router := chi.NewRouter()
	router.Use(auth.Middleware(authSvc))
	router.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	router.Get("/worlds/{worldID}/provinces/{provinceID}/army", ph.GetArmy)
	router.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)
	router.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)
	router.Get("/worlds/{worldID}/provinces/{provinceID}/ticklog", ph.Ticklog)
	raw := func(path string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		body, _ := io.ReadAll(rec.Body)
		return rec.Code, string(body)
	}
	settlementOf := func(provinceID uuid.UUID) map[string]any {
		t.Helper()
		code, body := raw("/provinces/" + provinceID.String())
		if code != http.StatusOK {
			t.Fatalf("GET province %s = %d %s", provinceID, code, body)
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		s, _ := p["settlement"].(map[string]any)
		if s == nil {
			t.Fatalf("no settlement in %s", body)
		}
		var keys []string
		for k := range s {
			if !foreignWhitelist[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			t.Errorf("foreign body carries keys outside the whitelist: %v", keys)
		}
		return s
	}
	intelOf := func(s map[string]any) map[string]any { m, _ := s["intel"].(map[string]any); return m }

	t.Run("unknown city is the same 404 as an id that does not exist", func(t *testing.T) {
		codeMissing, bodyMissing := raw("/provinces/" + uuid.New().String())
		code, body := raw("/provinces/" + hidden.province.String())
		if code != codeMissing || body != bodyMissing {
			t.Fatalf("unknown city = %d %s, missing id = %d %s", code, body, codeMissing, bodyMissing)
		}
		code, body = raw("/provinces/" + hidden.settlement.String())
		if code != codeMissing || body != bodyMissing {
			t.Fatalf("foreign settlement id as province id = %d %s, want the plain 404", code, body)
		}
	})

	t.Run("seen live: words and types, no level, no stores", func(t *testing.T) {
		s := settlementOf(near.province)
		if s["size"] != "town" || s["walls"] != float64(1) || s["name"] != "Nearby" || s["owner_name"] != "Wanax Far" {
			t.Fatalf("live identity = %v", s)
		}
		blds, _ := s["buildings"].([]any)
		if len(blds) != 1 {
			t.Fatalf("live buildings = %v", s["buildings"])
		}
		if _, hasLevel := blds[0].(map[string]any)["level"]; hasLevel {
			t.Fatal("a building seen from outside shows its level")
		}
		in := intelOf(s)
		if in["level"] != "seen" || in["seen_live"] != true || in["seen_at"] == nil || in["contact"] != nil {
			t.Fatalf("live intel = %v", in)
		}
		if _, ok := s["besieged"]; !ok {
			t.Fatal("a siege in sight is not shown")
		}
	})

	t.Run("sibling reads hide another ruler's city", func(t *testing.T) {
		p := "/provinces/" + near.province.String()
		if code, body := raw(p + "/goods"); code != 200 || body != "[]\n" {
			t.Fatalf("foreign goods = %d %s", code, body)
		}
		if code, body := raw(p + "/buildings"); code != 200 || body != "[]\n" {
			t.Fatalf("foreign buildings = %d %s", code, body)
		}
		if code, _ := raw(p + "/ticklog"); code != http.StatusNotFound {
			t.Fatalf("foreign ticklog = %d", code)
		}
		if code, _ := raw(p + "/army"); code != http.StatusOK {
			t.Fatalf("army of a city in live sight = %d, want 200", code)
		}
		if code, _ := raw("/provinces/" + remembered.province.String() + "/army"); code != http.StatusNotFound {
			t.Fatalf("army of a remembered city = %d, want 404", code)
		}
	})

	t.Run("memory: as last seen, size from then, no buildings, no siege", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 12000, besieged = true WHERE id = $1`, remembered.settlement); err != nil {
			t.Fatal(err)
		}
		s := settlementOf(remembered.province)
		if s["size"] != "town" {
			t.Fatalf("remembered size = %v, want the size when seen", s["size"])
		}
		if _, ok := s["buildings"]; ok {
			t.Fatal("a remembered city lists buildings")
		}
		if _, ok := s["besieged"]; ok {
			t.Fatal("a siege is shown out of memory")
		}
		in := intelOf(s)
		if in["level"] != "seen" || in["seen_live"] != nil || in["seen_at"] == nil {
			t.Fatalf("memory intel = %v", in)
		}
	})

	t.Run("contact: coarse words with date and source, levels, never numbers", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE settlements SET food_unmet_amount = 5 WHERE id = $1`, remembered.settlement); err != nil {
			t.Fatal(err)
		}
		if err := economy.RecordContact(ctx, pool, worldID, viewerID, remembered.settlement, economy.ContactMessenger, 45); err != nil {
			t.Fatal(err)
		}
		// An older contact must not overwrite the newer one.
		if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 500, food_unmet_amount = 0 WHERE id = $1`, remembered.settlement); err != nil {
			t.Fatal(err)
		}
		if err := economy.RecordContact(ctx, pool, worldID, viewerID, remembered.settlement, economy.ContactCaravan, 43); err != nil {
			t.Fatal(err)
		}
		var goodsText string
		if err := pool.QueryRow(ctx, `SELECT goods::text FROM player_contacted_settlements WHERE player_id = $1 AND settlement_id = $2`,
			viewerID, remembered.settlement).Scan(&goodsText); err != nil {
			t.Fatal(err)
		}
		for _, ch := range goodsText {
			if ch >= '0' && ch <= '9' {
				t.Fatalf("contact snapshot stored a number: %s", goodsText)
			}
		}
		s := settlementOf(remembered.province)
		in := intelOf(s)
		c, _ := in["contact"].(map[string]any)
		if in["level"] != "contact" || in["contact_source"] != "messenger" || in["contact_at"] == nil || c == nil {
			t.Fatalf("contact intel = %v", in)
		}
		if c["size"] != "great city" || c["food"] != "starving" {
			t.Fatalf("contact words = %v", c)
		}
		goods, _ := json.Marshal(c["goods"])
		if string(goods) != `[{"good":"oil","word":"plenty of"},{"good":"grain","word":"some"},{"good":"wine","word":"some"}]` {
			t.Fatalf("contact goods = %s", goods)
		}
		blds, _ := c["buildings"].([]any)
		if len(blds) != 1 || blds[0].(map[string]any)["level"] != float64(2) {
			t.Fatalf("contact buildings = %v", c["buildings"])
		}
	})

	t.Run("lifecycle: the contact keeps the ruler of its day", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id = $2 WHERE id = $1`, remembered.settlement, other); err != nil {
			t.Fatal(err)
		}
		c, _ := intelOf(settlementOf(remembered.province))["contact"].(map[string]any)
		if c["owner_name"] != "Wanax Far" {
			t.Fatalf("contact owner after a change of ruler = %v, want the ruler then", c["owner_name"])
		}
	})

	t.Run("deposits around a foreign city only on hexes the viewer knows", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit, tin_deposit) VALUES ($1, 9, 0, 'plains', true, false), ($1, 7, 0, 'plains', false, true)`,
			worldID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, 7, 0)`, worldID, viewerID); err != nil {
			t.Fatal(err)
		}
		_, body := raw("/provinces/" + remembered.province.String())
		var p struct {
			Catchment []string `json:"catchment_deposits"`
		}
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		if len(p.Catchment) != 1 || p.Catchment[0] != "tin" {
			t.Fatalf("catchment deposits = %v, want only tin (the remembered hex), never the unseen copper", p.Catchment)
		}
	})

	t.Run("market board: only the viewer's own last reading", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'market', 1)`, marketon.settlement); err != nil {
			t.Fatal(err)
		}
		if err := economy.WriteMarketBoard(ctx, pool, worldID, marketon.settlement, []string{"tin"}, nil, 41); err != nil {
			t.Fatal(err)
		}
		if _, ok := settlementOf(marketon.province)["market_board"]; ok {
			t.Fatal("a visitor without a reading sees the board")
		}
		b, err := economy.LoadMarketBoard(ctx, pool, worldID, marketon.settlement)
		if err != nil || b == nil {
			t.Fatalf("load board: %v", err)
		}
		if err := economy.RecordBoardReading(ctx, pool, worldID, viewerID, *b, 42); err != nil {
			t.Fatal(err)
		}
		if err := economy.WriteMarketBoard(ctx, pool, worldID, marketon.settlement, nil, []string{"wine"}, 43); err != nil {
			t.Fatal(err)
		}
		board, _ := settlementOf(marketon.province)["market_board"].(map[string]any)
		got, _ := json.Marshal([]any{board["buys"], board["sells"]})
		if string(got) != `[["tin"],[]]` {
			t.Fatalf("visitor's board = %s, want the old reading (buys tin), not the board as it stands", got)
		}
	})
}
