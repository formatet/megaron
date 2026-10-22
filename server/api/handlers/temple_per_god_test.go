package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// ORDER 15 (Timothy 2026-10-22): one temple per god and city; building the
// same god again raises that temple, another god is a new temple.
func TestTemplePerGod_BuildQueuesEachGodAndRefusesNoGod(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	setStock(t, f.pool, f.settlementID, "stone", 500)
	setStock(t, f.pool, f.settlementID, "timber", 500)

	code, body := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "temple"})
	if code != http.StatusBadRequest || !strings.Contains(body["error"].(string), "potnia") {
		t.Fatalf("temple without a god: %d %v", code, body)
	}
	for _, god := range []string{"potnia", "velchanos"} {
		if code, body := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "temple", "god": god}); code != http.StatusCreated {
			t.Fatalf("build %s: %d %v", god, code, body)
		}
	}
	if code, body := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "temple", "god": "potnia"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("same god queued twice: %d %v", code, body)
	}
	var gods string
	if err := f.pool.QueryRow(ctx, `SELECT string_agg(god_id, ',' ORDER BY god_id) FROM build_queue WHERE settlement_id=$1 AND building_type='temple'`, f.settlementID).Scan(&gods); err != nil || gods != "potnia,velchanos" {
		t.Fatalf("queued gods %q %v", gods, err)
	}
}

// Each temple employs its own workers; one god's room never covers another's,
// and demolishing one temple lifts only the workers standing at it.
func TestTemplePerGod_StaffingAndDemolitionStayWithTheirTemple(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=1000 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	ids := map[string]uuid.UUID{}
	for _, god := range []string{"potnia", "velchanos"} {
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,$2) RETURNING id`, f.settlementID, god).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[god] = id
	}
	staff := func(god string) (int, map[string]any) {
		body := map[string]any{"target_kind": "building", "building_type": "temple", "good_key": "cult"}
		if god != "" {
			body["god"] = god
		}
		return f.do(t, http.MethodPost, f.placementsPath(), body)
	}
	if code, body := staff(""); code != http.StatusUnprocessableEntity {
		t.Fatalf("two temples, no god chosen: %d %v", code, body)
	}
	for i := 0; i < 2; i++ {
		if code, body := staff("potnia"); code != http.StatusCreated {
			t.Fatalf("potnia %d: %d %v", i, code, body)
		}
	}
	if code, body := staff("potnia"); code != http.StatusConflict {
		t.Fatalf("potnia over its own capacity: %d %v", code, body)
	}
	if code, body := staff("velchanos"); code != http.StatusCreated {
		t.Fatalf("velchanos: %d %v", code, body)
	}
	count := func(id uuid.UUID) int {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE building_id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count(ids["potnia"]) != 2 || count(ids["velchanos"]) != 1 {
		t.Fatalf("workers potnia %d velchanos %d", count(ids["potnia"]), count(ids["velchanos"]))
	}
	code, body := f.do(t, http.MethodDelete, demolishPath(f, ids["velchanos"]), nil)
	if code != http.StatusOK || body["citizens_lifted"] != float64(1) {
		t.Fatalf("demolish velchanos: %d %v", code, body)
	}
	if count(ids["potnia"]) != 2 {
		t.Fatalf("demolishing Velchanos lifted Potnia's workers: %d left", count(ids["potnia"]))
	}
}

// A prayer is said at its own god's temple: a Temple of Potnia does not open
// Velchanos's Green Shoots, and the status names the missing temple.
func TestTemplePerGod_PrayerNeedsItsGodsTemple(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET culture_id='minoan' WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	_, prov := f.do(t, http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String(), nil)
	sett, _ := prov["settlement"].(map[string]any)
	prayers, _ := sett["available_prayers"].([]any)
	seen := 0
	for _, it := range prayers {
		p := it.(map[string]any)
		reason, _ := p["unavailable_reason"].(string)
		switch p["id"] {
		case religion.PrayerGreenShoots:
			seen++
			if reason != "no Temple of Velchanos" || p["temple"] != "Temple of Velchanos" {
				t.Fatalf("green shoots: %v", p)
			}
		case religion.PrayerPotniaVision:
			seen++
			if strings.HasPrefix(reason, "no Temple") {
				t.Fatalf("potnia's vision blocked by temple at a Temple of Potnia: %v", p)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("prayers not listed: %v", prayers)
	}
	temples, _ := sett["temples"].([]any)
	if len(temples) != 1 || temples[0].(map[string]any)["name"] != "Temple of Potnia" {
		t.Fatalf("temples %v", sett["temples"])
	}
}

// The rite itself is refused at the wrong god's temple and names the temple
// it needs; nothing is debited.
func TestTemplePerGod_RiteRefusedWithoutItsGodsTemple(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	city := f.cities[0]
	if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='temple' AND god_id<>'potnia'`, city); err != nil {
		t.Fatal(err)
	}
	r := f.cast(city, religion.PrayerGreenShoots)
	if r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "Temple of Velchanos") {
		t.Fatalf("green shoots at a Temple of Potnia: %d %s", r.Code, r.Body.String())
	}
	if got := f.stock(t, city, "grain"); got != 100 {
		t.Fatalf("refused rite debited grain %v", got)
	}
	if r := f.cast(city, religion.PrayerPotniaVision); r.Code != 200 {
		t.Fatalf("potnia's vision at the Temple of Potnia: %d %s", r.Code, r.Body.String())
	}
}
