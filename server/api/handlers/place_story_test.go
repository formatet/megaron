package handlers

// Telling of places (megaron_plan_vagbeskrivningar v1, §Prov): a letter tells
// of a named place; the recipient learns exactly the hexes the teller had seen
// when the Runner left, cities as the teller last saw them, and no trade right.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
)

func TestTellPlace_RecipientHearsWhatTheTellerSaw(t *testing.T) {
	f := setupReplyFixture(t)
	ctx := context.Background()

	// Three cedar forests far from both cities (row r=12, r=16, r=20 —
	// more than six hexes from either, so no reach rule touches them).
	for _, r := range []int{12, 16, 20} {
		for q := 0; q < 12; q++ {
			if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'forest_cedar')`, f.worldID, q, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	tiles, err := loadGeography(ctx, f.pool, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	anchorOf := func(r int) [2]int {
		for _, p := range world.Places(tiles) {
			if p.Kind == "forest" && p.Hexes[0][1] == r {
				return p.Anchor
			}
		}
		t.Fatalf("no forest on row %d", r)
		return [2]int{}
	}
	named, unnamed, unseen := anchorOf(12), anchorOf(16), anchorOf(20)
	for _, n := range []struct {
		a    [2]int
		name string
	}{{named, "Dark Wood"}, {unseen, "Far Wood"}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO place_names (world_id, kind, anchor_q, anchor_r, name, named_tick) VALUES ($1, 'forest', $2, $3, $4, 1)`, f.worldID, n.a[0], n.a[1], n.name); err != nil {
			t.Fatal(err)
		}
	}

	// A third Wanax's city inside Dark Wood, seen by the teller, then taken
	// while nobody looks.
	newPlayer := func(name string) uuid.UUID {
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`, "p-"+uuid.NewString(), name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	firstOwner, laterOwner := newPlayer("Wanax First"), newPlayer("Wanax Later")
	var provID, cedarhold uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 12, 'forest_cedar') RETURNING id`, f.worldID).Scan(&provID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state) VALUES ($1, $2, 'Cedarhold', 'achaean', $3, 'capital', true, 'active') RETURNING id`, f.worldID, provID, firstOwner).Scan(&cedarhold); err != nil {
		t.Fatal(err)
	}
	remember := func(hexes ...province.MapPosition) {
		for _, h := range hexes {
			if _, err := f.pool.Exec(ctx, `INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, f.worldID, f.senderID, h.Q, h.R); err != nil {
				t.Fatal(err)
			}
		}
		if err := province.RecordSeenSettlements(ctx, f.pool, f.worldID, f.senderID, hexes); err != nil {
			t.Fatal(err)
		}
	}
	// The teller has seen 3 of Dark Wood's 12 hexes, and one of the unnamed wood.
	remember(province.MapPosition{Q: 0, R: 12}, province.MapPosition{Q: 1, R: 12}, province.MapPosition{Q: 2, R: 12}, province.MapPosition{Q: 0, R: 16})
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id = $2 WHERE id = $1`, cedarhold, laterOwner); err != nil {
		t.Fatal(err)
	}

	send := func(a [2]int) (int, map[string]any) {
		return f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers",
			map[string]any{"destination_id": f.destID.String(), "message": "hear of this",
				"tell_place": map[string]any{"kind": "forest", "anchor_q": a[0], "anchor_r": a[1]}})
	}
	if code, body := send(unnamed); code != http.StatusUnprocessableEntity {
		t.Errorf("unnamed place: %d %v, want 422", code, body)
	}
	if code, body := send(unseen); code != http.StatusUnprocessableEntity {
		t.Errorf("named place the teller never saw: %d %v, want 422", code, body)
	}
	code, body := send(named)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("tell Dark Wood: %d %v", code, body)
	}
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM messengers WHERE world_id = $1 AND told_place IS NOT NULL`, f.worldID).Scan(&messengerID); err != nil {
		t.Fatalf("one messenger carries the story: %v", err)
	}

	// Seen after the Runner left: not in the story (frozen at sending).
	remember(province.MapPosition{Q: 3, R: 12})

	sched := events.NewScheduler(f.pool, clock.NewTestClock(time.Now()))
	var generation int
	if err := f.pool.QueryRow(ctx, `SELECT passage_generation FROM messengers WHERE id = $1`, messengerID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(messenger.ArrivalPayload{MessengerID: messengerID, PassageGeneration: generation})
	arrival := messenger.NewArrivalHandler(f.pool, sched, events.NewStore(f.pool), nil)
	for i := 0; i < 2; i++ { // delivery replayed: same rows, one event
		if err := arrival.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1, Payload: payload}); err != nil {
			t.Fatalf("deliver #%d: %v", i+1, err)
		}
	}

	rows, err := f.pool.Query(ctx, `SELECT q, r, told_by FROM player_heard_tiles WHERE world_id = $1 AND player_id = $2 AND source = 'told'`, f.worldID, f.receiverID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var q, r int
		var by uuid.UUID
		if err := rows.Scan(&q, &r, &by); err != nil {
			t.Fatal(err)
		}
		if by != f.senderID {
			t.Errorf("told_by = %v, want the teller", by)
		}
		got = append(got, fmt.Sprintf("%d,%d", q, r))
	}
	rows.Close()
	sort.Strings(got)
	if fmt.Sprint(got) != "[0,12 1,12 2,12]" {
		t.Errorf("recipient heard %v, want exactly the 3 hexes seen when the Runner left", got)
	}
	var events int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id = $1 AND event_type = 'PlaceToldV1'`, f.worldID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("PlaceToldV1 events = %d, want 1 after a replayed delivery", events)
	}

	// The told city reads as the teller last saw it, and opens no trade.
	wh := NewWorldHandler(f.pool, nil, clock.NewTestClock(time.Now()))
	var found bool
	for _, c := range wh.loadCities(ctx, f.worldID, f.receiverID) {
		if c.SettlementID == cedarhold.String() {
			found = true
			if c.Owner != "Wanax First" || c.Knowledge != "heard" || c.LastSeenAt == nil {
				t.Errorf("told Cedarhold = owner %q knowledge %q last_seen %v, want Wanax First / heard / set", c.Owner, c.Knowledge, c.LastSeenAt)
			}
		}
	}
	if !found {
		t.Error("told Cedarhold missing from the recipient's city list")
	}
	if province.VisibleFrom(province.MapPosition{Q: 1, R: 12}, province.VisibleOrigins(ctx, f.pool, f.worldID, f.receiverID), 6) {
		t.Error("a told city must not open trade (VisibleOrigins)")
	}

	// /map marks the told hexes.
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/map", nil)
	req.Header.Set("Authorization", "Bearer "+f.recvToken)
	rec := httptest.NewRecorder()
	f.router.Get("/worlds/{worldID}/map", wh.Map)
	f.router.ServeHTTP(rec, req)
	var tilesOut []struct {
		Q, R   int
		Tier   string `json:"tier"`
		ToldBy string `json:"told_by"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tilesOut); err != nil {
		t.Fatalf("decode /map: %v (%d)", err, rec.Code)
	}
	var marked int
	for _, tl := range tilesOut {
		if tl.ToldBy != "" {
			marked++
			if tl.R != 12 || tl.Tier != "remembered" {
				t.Errorf("told_by on %d,%d tier %s", tl.Q, tl.R, tl.Tier)
			}
		}
	}
	if marked != 3 {
		t.Errorf("/map marks %d told hexes, want 3", marked)
	}
}
