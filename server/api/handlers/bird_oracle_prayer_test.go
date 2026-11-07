package handlers

// Fågeloraklet (böner steg 5d): Asterion's live sight over one NAMED place of the caster's own map
// for one game day. The eye is the caster's alone, has a fixed radius of 2, leaves grey memory
// behind, and the watched hear nothing.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// oracleCity makes a foreign city (true wall `wall`, remembered as `seen`) with plains tiles around it.
func (f alphaFixture) oracleCity(t *testing.T, name string, wall, seen int) (foreignCity, province.MapPosition) {
	t.Helper()
	fc := f.foreignWalled(t, name, wall, seen, 10)
	pos := province.MapPosition{Q: 20 + nextForeignQ, R: 20}
	f.plainsAround(t, pos, 4)
	return fc, pos
}

func (f alphaFixture) plainsAround(t *testing.T, c province.MapPosition, radius int) {
	t.Helper()
	for _, h := range hexgrid.Disk(hexgrid.Coord{Q: c.Q, R: c.R}, radius) {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,'plains') ON CONFLICT DO NOTHING`, f.world, h.Q, h.R); err != nil {
			t.Fatal(err)
		}
	}
}

func (f alphaFixture) castOracle(target string) (int, string) {
	rec := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerAsterionBirdOracle, "target": target})
	return rec.Code, rec.Body.String()
}

type oracleRow struct {
	q, r        int
	from, until int64
	kind, pName string
}

func (f alphaFixture) sights(t *testing.T) (out []oracleRow) {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT q,r,from_milli,until_milli,place_kind,place_name FROM oracle_sights WHERE world_id=$1 AND player_id=$2 ORDER BY from_milli`, f.world, f.player)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var x oracleRow
		if err := rows.Scan(&x.q, &x.r, &x.from, &x.until, &x.kind, &x.pName); err != nil {
			t.Fatal(err)
		}
		out = append(out, x)
	}
	return
}

func (f alphaFixture) eyesFor(player uuid.UUID, at int64) []province.Eye {
	return province.LoadLiveEyes(context.Background(), f.pool, f.world, player, time.Now(), at)
}

func hasOracleEye(eyes []province.Eye) bool {
	for _, e := range eyes {
		if e.Kind == province.EyeOracle {
			return true
		}
	}
	return false
}

func TestBirdOracle_CastPaysAndTheEyeLastsExactlyOneGameDay(t *testing.T) {
	f := newAlphaFixture(t)
	fc, pos := f.oracleCity(t, "Mycenae", 1, 1)
	si, wi := f.stock(t, f.cities[0], "silver"), f.stock(t, f.cities[0], "wine")
	code, body := f.castOracle("mycenae")
	if code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	if got := si - f.stock(t, f.cities[0], "silver"); got != 10 {
		t.Fatalf("silver paid %v, want 10", got)
	}
	if got := wi - f.stock(t, f.cities[0], "wine"); got != 8 {
		t.Fatalf("wine paid %v, want 8", got)
	}
	rows := f.sights(t)
	if len(rows) != 1 || rows[0].q != pos.Q || rows[0].r != pos.R || rows[0].until-rows[0].from != 1000 || rows[0].kind != "city" || rows[0].pName != "Mycenae" {
		t.Fatalf("oracle_sights %+v, want one city eye at %v lasting 1000 milli", rows, pos)
	}
	ev := f.lastRite(t)["effect"].(map[string]any)
	if ev["place_kind"] != "city" || ev["place_name"] != "Mycenae" || ev["radius"] != float64(2) || ev["until_tick"] != float64(101) ||
		ev["q"] != float64(pos.Q) || ev["r"] != float64(pos.R) {
		t.Fatalf("recorded effect %v", ev)
	}
	// What the eye shows: a foreign army 2 hexes off is seen, 3 hexes off is not.
	near := province.MapPosition{Q: pos.Q + 2, R: pos.R}
	far := province.MapPosition{Q: pos.Q + 3, R: pos.R}
	mid := f.eyesFor(f.player, rows[0].from+500)
	if !province.AnyEyeSees(mid, near, "plains") {
		t.Fatal("an army 2 hexes from the named place is not seen during the day")
	}
	if province.AnyEyeSees(mid, far, "plains") {
		t.Fatal("an army 3 hexes from the named place is seen: the radius is 2")
	}
	for _, at := range []int64{rows[0].until, rows[0].until + 5000, rows[0].from - 1} {
		if province.AnyEyeSees(f.eyesFor(f.player, at), near, "plains") {
			t.Fatalf("the eye still sees at milli %d (window %d..%d)", at, rows[0].from, rows[0].until)
		}
	}
	_ = fc
}

func TestBirdOracle_ShowsAForeignArmyWithItsStrengthLikeAnyLiveEye(t *testing.T) {
	// /foreign-units reads the same eyes (province.LoadLiveEyes) and shows type, size and stance in full for
	// anything AnyEyeSees lets through, so the oracle needs no strength code of its own: the eye is the test.
	f := newAlphaFixture(t)
	fc, pos := f.oracleCity(t, "Argos", 1, 1)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'infantry','land',40,0,'positioned',$3,$4)`,
		f.world, fc.owner, pos.Q+2, pos.R); err != nil {
		t.Fatal(err)
	}
	if code, body := f.castOracle("Argos"); code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	row := f.sights(t)[0]
	if !province.AnyEyeSees(f.eyesFor(f.player, row.from+1), province.MapPosition{Q: pos.Q + 2, R: pos.R}, "plains") {
		t.Fatal("the army is not within the oracle's sight")
	}
}

func TestBirdOracle_TheWatchedHearNothing(t *testing.T) {
	f := newAlphaFixture(t)
	hub := &wallNoticeRecorder{}
	f.h.Hub = hub
	fc, _ := f.oracleCity(t, "Tiryns", 2, 2)
	if code, body := f.castOracle("Tiryns"); code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	for i, to := range hub.to {
		if to == fc.owner {
			t.Fatalf("the watched Wanax was sent %q", hub.kinds[i])
		}
	}
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE world_id=$1 AND stream_id=$2`, f.world, fc.id).Scan(&n)
	if n != 0 {
		t.Fatalf("%d events on the watched city's stream", n)
	}
}

func TestBirdOracle_TheEyeIsTheCastersAlone(t *testing.T) {
	f := newAlphaFixture(t)
	fc, _ := f.oracleCity(t, "Pylos", 1, 1)
	if code, body := f.castOracle("Pylos"); code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	at := f.sights(t)[0].from + 1
	if !hasOracleEye(f.eyesFor(f.player, at)) {
		t.Fatal("the caster has no oracle eye")
	}
	if hasOracleEye(f.eyesFor(fc.owner, at)) {
		t.Fatal("another Wanax was given the oracle's eye")
	}
}

func TestBirdOracle_TheCastWritesGreyMemoryAndRefreshesTheCityMemory(t *testing.T) {
	f := newAlphaFixture(t)
	fc, pos := f.oracleCity(t, "Thebes", 3, 1) // really walled 3, remembered 1
	if code, body := f.castOracle("Thebes"); code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	scouted := func(p province.MapPosition) bool {
		var ok bool
		_ = f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=$3 AND r=$4)`, f.world, f.player, p.Q, p.R).Scan(&ok)
		return ok
	}
	for _, h := range hexgrid.Disk(hexgrid.Coord{Q: pos.Q, R: pos.R}, 2) {
		if !scouted(province.MapPosition{Q: h.Q, R: h.R}) {
			t.Fatalf("hex %v within the oracle's sight did not become memory", h)
		}
	}
	if scouted(province.MapPosition{Q: pos.Q + 3, R: pos.R}) {
		t.Fatal("a hex 3 away became memory")
	}
	var wall int
	_ = f.pool.QueryRow(context.Background(), `SELECT wall_level FROM player_seen_settlements WHERE player_id=$1 AND settlement_id=$2`, f.player, fc.id).Scan(&wall)
	if wall != 3 {
		t.Fatalf("the remembered wall is %d, want the seen 3", wall)
	}
}

func TestBirdOracle_TheNextGameDayWritesMemoryAgainEvenIfTheCasterNeverLogsIn(t *testing.T) {
	f := newAlphaFixture(t)
	fc, _ := f.oracleCity(t, "Sparta", 1, 1)
	if code, body := f.castOracle("Sparta"); code != 200 {
		t.Fatalf("cast %d %s", code, body)
	}
	var due int
	var raw []byte
	if err := f.pool.QueryRow(context.Background(), `SELECT due_tick, payload FROM scheduled_events WHERE world_id=$1 AND event_type='BirdOracleClose'`, f.world).Scan(&due, &raw); err != nil {
		t.Fatalf("no BirdOracleClose scheduled: %v", err)
	}
	if due != 101 {
		t.Fatalf("close due at tick %d, want 101", due)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE settlements SET wall_level=2 WHERE id=$1`, fc.id); err != nil {
		t.Fatal(err)
	}
	h := combat.NewBirdOracleCloseHandler(f.pool)
	for i := 0; i < 2; i++ { // idempotent
		if err := h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	var wall int
	_ = f.pool.QueryRow(context.Background(), `SELECT wall_level FROM player_seen_settlements WHERE player_id=$1 AND settlement_id=$2`, f.player, fc.id).Scan(&wall)
	if wall != 2 {
		t.Fatalf("the remembered wall is %d after the closing sweep, want 2", wall)
	}
}

func TestBirdOracle_RefusesWhatIsNotANamedPlaceOfYourMapWithoutTakingTheGift(t *testing.T) {
	f := newAlphaFixture(t)
	fc, pos := f.oracleCity(t, "Unknown City", 1, 1)
	_, _ = f.pool.Exec(context.Background(), `DELETE FROM player_seen_settlements WHERE settlement_id=$1`, fc.id)
	// A named place whose anchor the caster does not know, and one it does.
	f.plainsAround(t, province.MapPosition{Q: 60, R: 60}, 3)
	f.plainsAround(t, province.MapPosition{Q: 70, R: 70}, 3)
	for name, q := range map[string]int{"Hidden Cape": 60, "Known Isle": 70} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,name,named_tick) VALUES($1,'island',$2,$2,$3,1)`, f.world, q, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,70,70)`, f.world, f.player); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"unknown city": "Unknown City", "nameless hex": fmt.Sprintf("%d,%d", pos.Q, pos.R), "place not on your map": "Hidden Cape",
		"nothing": "", "no such": "Nowhere"}
	for name, target := range cases {
		si := f.stock(t, f.cities[0], "silver")
		code, body := f.castOracle(target)
		if code < 400 || code >= 500 {
			t.Fatalf("%s: status %d %s, want 4xx", name, code, body)
		}
		if f.stock(t, f.cities[0], "silver") != si {
			t.Fatalf("%s: the gift was taken", name)
		}
	}
	if n := len(f.sights(t)); n != 0 {
		t.Fatalf("%d eyes opened by refused prayers", n)
	}
	// A named place whose anchor you know is fine.
	code, body := f.castOracle("known isle")
	if code != 200 {
		t.Fatalf("known place: %d %s", code, body)
	}
	rows := f.sights(t)
	if len(rows) != 1 || rows[0].q != 70 || rows[0].kind != "island" || rows[0].pName != "Known Isle" {
		t.Fatalf("sights %+v", rows)
	}
}

func TestBirdOracle_YourOwnCityIsAKnownPlaceToo(t *testing.T) {
	f := newAlphaFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE settlements SET name='Home Harbour' WHERE id=$1`, f.cities[1]); err != nil {
		t.Fatal(err)
	}
	if code, body := f.castOracle("Home Harbour"); code != 200 {
		t.Fatalf("own city: %d %s", code, body)
	}
	if rows := f.sights(t); len(rows) != 1 || rows[0].q != 1 || rows[0].r != 0 {
		t.Fatalf("sights %+v", rows)
	}
}

func TestBirdOracle_CatalogueListsKnownCitiesAndNamedPlaces(t *testing.T) {
	f := newAlphaFixture(t)
	f.oracleCity(t, "Mycenae", 1, 1)
	f.plainsAround(t, province.MapPosition{Q: 70, R: 70}, 2)
	_, _ = f.pool.Exec(context.Background(), `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,name,named_tick) VALUES($1,'island',70,70,'Known Isle',1),($1,'cape',80,80,'Far Cape',1)`, f.world)
	_, _ = f.pool.Exec(context.Background(), `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,70,70)`, f.world, f.player)
	row, ok := f.catalogue(t, f.cities[0])[religion.PrayerAsterionBirdOracle]
	if !ok || row.Targets == nil || row.Targets.Kind != "place" {
		t.Fatalf("catalogue row %+v", row)
	}
	var cities, places []string
	for _, c := range row.Targets.Cities {
		cities = append(cities, c.Name)
	}
	for _, p := range row.Targets.Places {
		places = append(places, p.Name)
	}
	if !strings.Contains(strings.Join(cities, "|"), "Mycenae") || !strings.Contains(strings.Join(cities, "|"), "Alpha City") {
		t.Fatalf("cities %v, want Mycenae and the caster's own", cities)
	}
	if len(places) != 1 || places[0] != "Known Isle" {
		t.Fatalf("places %v, want only the known one", places)
	}
	b, _ := json.Marshal(row.Targets)
	if strings.Contains(string(b), "Far Cape") {
		t.Fatal("an unknown place is listed")
	}
}
