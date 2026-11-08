package handlers

// Murarna faller (steg 5c): The Walls Fall, Rite side. Nothing falls at the cast; the target must be a
// foreign city of the caster's own map with a REMEMBERED wall; the owner is warned (never by whom) only
// when in good standing; the caster learns nothing about the result.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

type wallNoticeRecorder struct {
	to     []uuid.UUID
	kinds  []string
	bodies []any
}

func (r *wallNoticeRecorder) BroadcastEvent(uuid.UUID, string, any) {}
func (r *wallNoticeRecorder) NotifyPlayer(_ context.Context, _, p uuid.UUID, kind string, _ int, body any) error {
	r.to, r.kinds, r.bodies = append(r.to, p), append(r.kinds, kind), append(r.bodies, body)
	return nil
}
func (r *wallNoticeRecorder) of(kind string) (n int) {
	for _, k := range r.kinds {
		if k == kind {
			n++
		}
	}
	return
}

var nextForeignQ int

type foreignCity struct{ id, owner uuid.UUID }

// foreignWalled makes another Wanax (kharis as given) with a city of true wall `wall`; the caster remembers it
// with wall `seen`. Returns the city and its owner.
func (f alphaFixture) foreignWalled(t *testing.T, name string, wall, seen int, kharisAmount float64) foreignCity {
	t.Helper()
	ctx := context.Background()
	var fc foreignCity
	nextForeignQ++
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash,wanax_name) VALUES($1,'x',$2) RETURNING id`, "foe-"+uuid.NewString(), "Foe "+uuid.NewString()[:8]).Scan(&fc.owner))
	_, err := f.pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,0,100)`, fc.owner, f.world, kharisAmount)
	must(err)
	var prov uuid.UUID
	must(f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,20,'plains') RETURNING id`, f.world, 20+nextForeignQ).Scan(&prov))
	must(f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital,wall_level) VALUES($1,$2,$3,'achaean',$4,'capital',true,$5) RETURNING id`, f.world, prov, name, fc.owner, wall).Scan(&fc.id))
	if wall >= 1 {
		_, err = f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'wall',$2)`, fc.id, wall)
		must(err)
	}
	_, err = f.pool.Exec(ctx, `INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick)
	  VALUES($1,$2,$3,$7,20,$4,$5,'Foe Wanax',true,'active',$6,500,90)`, f.world, f.player, fc.id, name, fc.owner, seen, 20+nextForeignQ)
	must(err)
	return fc
}

func (f alphaFixture) trueWall(t *testing.T, id uuid.UUID) (n int) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(), `SELECT wall_level FROM settlements WHERE id=$1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return
}

func (f alphaFixture) scheduledFalls(t *testing.T) (due []int, payloads []combat.WallsFallPayload) {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT due_tick, payload FROM scheduled_events WHERE world_id=$1 AND event_type='WallsFall' ORDER BY id`, f.world)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var d int
		var raw []byte
		_ = rows.Scan(&d, &raw)
		var p combat.WallsFallPayload
		_ = json.Unmarshal(raw, &p)
		due, payloads = append(due, d), append(payloads, p)
	}
	return
}

func (f alphaFixture) castWalls(city uuid.UUID, target string) (code int, body string) {
	rec := f.castBody(city, map[string]any{"prayer": religion.PrayerPotniaWallsFall, "target": target})
	return rec.Code, rec.Body.String()
}

func TestWallsFallPrayer_AnsweredNowFallsThreeTicksLaterNotBefore(t *testing.T) {
	f := newAlphaFixture(t)
	fc := f.foreignWalled(t, "Mycenae", 2, 2, 10)
	lv, si := f.stock(t, f.cities[0], "livestock"), f.stock(t, f.cities[0], "silver")
	code, body := f.castWalls(f.cities[0], "mycenae")
	if code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	if got := lv - f.stock(t, f.cities[0], "livestock"); got != 12 {
		t.Fatalf("livestock paid %v, want 12", got)
	}
	if got := si - f.stock(t, f.cities[0], "silver"); got != 8 {
		t.Fatalf("silver paid %v, want 8", got)
	}
	if w := f.trueWall(t, fc.id); w != 2 {
		t.Fatalf("the wall is %d straight after the cast, want 2 (it falls 3 ticks later)", w)
	}
	due, payloads := f.scheduledFalls(t)
	if len(due) != 1 || due[0] != 103 || payloads[0].SettlementID != fc.id || payloads[0].CastEventID == 0 {
		t.Fatalf("scheduled %v %+v, want one fall due at tick 103", due, payloads)
	}
	ev := f.lastRite(t)["effect"].(map[string]any)
	if ev["target_settlement_id"] != fc.id.String() || ev["falls_tick"] != float64(103) {
		t.Fatalf("recorded effect %v", ev)
	}
	// Carried out when due: one level, in both stores.
	h := combat.NewWallsFallHandler(f.pool, events.NewStore(f.pool), nil)
	raw, _ := json.Marshal(payloads[0])
	if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	var row int
	_ = f.pool.QueryRow(context.Background(), `SELECT level FROM buildings WHERE settlement_id=$1 AND building_type='wall'`, fc.id).Scan(&row)
	if w := f.trueWall(t, fc.id); w != 1 || row != 1 {
		t.Fatalf("after the fall: wall %d, building row %d, want 1 and 1", w, row)
	}
}

func TestWallsFallPrayer_TheCasterIsNeverToldTheResultOrTheNewLevel(t *testing.T) {
	f := newAlphaFixture(t)
	fc := f.foreignWalled(t, "Tiryns", 2, 2, 10)
	code, body := f.castWalls(f.cities[0], fc.id.String())
	if code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(body), &resp)
	shown := resp["effect"].(map[string]any)
	if shown["heard"] != true || len(shown) != 1 || strings.Contains(strings.ToLower(body), "wall level") {
		t.Fatalf("shown effect %v / %s", shown, body)
	}
	_, payloads := f.scheduledFalls(t)
	raw, _ := json.Marshal(payloads[0])
	if err := combat.NewWallsFallHandler(f.pool, events.NewStore(f.pool), nil).Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	// The memory is untouched until the city is seen again.
	var seen int
	_ = f.pool.QueryRow(context.Background(), `SELECT wall_level FROM player_seen_settlements WHERE player_id=$1 AND settlement_id=$2`, f.player, fc.id).Scan(&seen)
	if seen != 2 {
		t.Fatalf("the caster's memory of the wall changed to %d", seen)
	}
}

func TestWallsFallPrayer_TwoPrayersFallTwoLevels(t *testing.T) {
	f := newAlphaFixture(t)
	fc := f.foreignWalled(t, "Pylos", 3, 3, 10)
	if code, body := f.castWalls(f.cities[0], "Pylos"); code != 200 {
		t.Fatalf("first %d %s", code, body)
	}
	f.clearRest(t)
	if code, body := f.castWalls(f.cities[0], "Pylos"); code != 200 {
		t.Fatalf("second %d %s", code, body)
	}
	_, payloads := f.scheduledFalls(t)
	if len(payloads) != 2 || payloads[0].CastEventID == payloads[1].CastEventID {
		t.Fatalf("scheduled %+v", payloads)
	}
	h := combat.NewWallsFallHandler(f.pool, events.NewStore(f.pool), nil)
	for _, p := range payloads {
		raw, _ := json.Marshal(p)
		if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	if w := f.trueWall(t, fc.id); w != 1 {
		t.Fatalf("wall %d after two prayers, want 1", w)
	}
}

func TestWallsFallPrayer_RefusesOwnUnknownAndUnwalledMemoryWithoutTakingTheGift(t *testing.T) {
	f := newAlphaFixture(t)
	f.foreignWalled(t, "Remembered Bare", 2, 0, 10) // really walled, remembered as bare
	unknown := f.foreignWalled(t, "Unknown", 2, 2, 10)
	_, _ = f.pool.Exec(context.Background(), `DELETE FROM player_seen_settlements WHERE settlement_id=$1`, unknown.id)
	// The caster's own city is in the caster's memory too, with a wall: still refused.
	if _, err := f.pool.Exec(context.Background(), `UPDATE settlements SET name='Own Walled', wall_level=2 WHERE id=$1`, f.cities[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO player_seen_settlements(world_id,player_id,settlement_id,q,r,name,owner_id,owner_name,is_capital,state,wall_level,population,seen_tick)
	  VALUES($1,$2,$3,1,0,'Own Walled',$2,'me',false,'active',2,500,90)`, f.world, f.player, f.cities[1]); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"own city by name in memory": "Own Walled", "own city by name": "Alpha City", "own city by id": f.cities[1].String(), "unknown city": "Unknown",
		"remembered wall 0": "Remembered Bare", "nothing": "", "no such": "Nowhere"}
	for name, target := range cases {
		lv := f.stock(t, f.cities[0], "livestock")
		code, body := f.castWalls(f.cities[0], target)
		if code < 400 || code >= 500 {
			t.Fatalf("%s: status %d %s, want 4xx", name, code, body)
		}
		if f.stock(t, f.cities[0], "livestock") != lv {
			t.Fatalf("%s: the gift was taken", name)
		}
	}
	if due, _ := f.scheduledFalls(t); len(due) != 0 {
		t.Fatalf("%d falls scheduled by refused prayers", len(due))
	}
}

func TestWallsFallPrayer_OmenOnlyWhenTheOwnerStandsWellAndNeverNamesTheCaster(t *testing.T) {
	for _, c := range []struct {
		name   string
		kharis float64
		warned int
	}{{"exactly at the threshold", kharis.BlessThreshold, 1}, {"just below", kharis.BlessThreshold - 1, 0}} {
		t.Run(c.name, func(t *testing.T) {
			f := newAlphaFixture(t)
			hub := &wallNoticeRecorder{}
			f.h.Hub = hub
			fc := f.foreignWalled(t, "Thebes", 2, 2, c.kharis)
			if code, body := f.castWalls(f.cities[0], "Thebes"); code != 200 {
				t.Fatalf("cast %d %s", code, body)
			}
			if got := hub.of(NoticeWallsOmen); got != c.warned {
				t.Fatalf("%d omen notices, want %d (notices %v)", got, c.warned, hub.kinds)
			}
			for i, k := range hub.kinds {
				if k != NoticeWallsOmen {
					continue
				}
				if hub.to[i] != fc.owner {
					t.Fatalf("the omen went to %v, want the city's owner", hub.to[i])
				}
				raw, _ := json.Marshal(hub.bodies[i])
				text := strings.ToLower(string(raw))
				if strings.Contains(text, f.player.String()) || strings.Contains(text, "alpha city") || strings.Contains(text, f.cities[0].String()) {
					t.Fatalf("the omen names the praying Wanax: %s", raw)
				}
			}
			// The caster's own RiteAnswered carries no recipient mix-up: omen never goes to the caster.
			for i, k := range hub.kinds {
				if k == NoticeWallsOmen && hub.to[i] == f.player {
					t.Fatal("the omen reached the caster")
				}
			}
		})
	}
}

func TestWallsFallPrayer_CatalogueOffersOnlyRememberedWalledForeignCities(t *testing.T) {
	// The catalogue is the same list the check uses: a foreign city remembered with a wall.
	f := newAlphaFixture(t)
	f.foreignWalled(t, "Walled", 2, 2, 10)
	f.foreignWalled(t, "Bare", 2, 0, 10)
	ph := &ProvinceHandler{pool: f.pool}
	targets, err := ph.loadWallsFallTargets(context.Background(), f.player, f.world)
	if err != nil {
		t.Fatal(err)
	}
	if targets.Kind != "city" || len(targets.Cities) != 1 || targets.Cities[0].Name != "Walled" {
		t.Fatalf("targets %+v", targets)
	}
	if got, want := targets.Cities[0].Where, "Foe Wanax's, wall 2 when last seen"; got != want {
		t.Fatalf("where %q, want %q", got, want)
	}
}
