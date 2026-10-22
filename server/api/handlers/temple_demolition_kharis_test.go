package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/kharis"
	"github.com/google/uuid"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func seedDemolitionTemple(t *testing.T, f *mineGateFixture, level int, standing float64) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_cap,kharis_calc_tick) SELECT owner_id,world_id,id,$2,0,100,current_world_tick() FROM settlements WHERE id=$1`, f.settlementID, standing); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',$2,'potnia') RETURNING id`, f.settlementID, level).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestTempleDemolitionDebtFloorReplay(t *testing.T) {
	for _, tc := range []struct {
		level       int
		start, want float64
	}{{1, 80, 70}, {2, 80, 60}, {3, 80, 50}, {3, 12, 1}} {
		f := buildHexFixture(t)
		id := seedDemolitionTemple(t, f, tc.level, tc.start)
		code, body := f.do(t, http.MethodDelete, demolishPath(f, id), nil)
		if code != 200 {
			t.Fatalf("delete %d %v", code, body)
		}
		var got float64
		if err := f.pool.QueryRow(context.Background(), `SELECT kharis_amount FROM player_world_records WHERE settlement_id=$1`, f.settlementID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("L%d standing%g want%g", tc.level, got, tc.want)
		}
		var notices int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='TempleDemolitionWrath'`, f.worldID).Scan(&notices); err != nil {
			t.Fatal(err)
		}
		if notices != 1 {
			t.Errorf("wrath notices%d want1", notices)
		}
		code, _ = f.do(t, http.MethodDelete, demolishPath(f, id), nil)
		if code != 404 {
			t.Fatal(code)
		}
		var after float64
		f.pool.QueryRow(context.Background(), `SELECT kharis_amount FROM player_world_records WHERE settlement_id=$1`, f.settlementID).Scan(&after)
		if after != got {
			t.Fatal("retry charged again")
		}
	}
}

func TestTempleDemolitionQuoteIncludesOtherCitiesAndStaysPrivate(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	id := seedDemolitionTemple(t, f, 3, 80)
	// A second temple in a different owned city determines the post-demolition ceiling.
	var colony uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,5,0,'plains') RETURNING id`, f.worldID).Scan(&colony); err != nil {
		t.Fatal(err)
	}
	var sid uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,population) SELECT world_id,$2,'Colony','achaean',owner_id,100 FROM settlements WHERE id=$1 RETURNING id`, f.settlementID, colony).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',2,'potnia')`, sid); err != nil {
		t.Fatal(err)
	}
	var owner uuid.UUID
	f.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&owner)
	quote, err := kharis.ProjectTempleDemolition(ctx, f.pool, owner, f.worldID, id)
	if err != nil {
		t.Fatal(err)
	}
	if quote.CapBefore != 100 || quote.CapAfter != 75 || quote.KharisAfter != 50 || quote.KharisLoss != 30 {
		t.Fatalf("quote %+v", quote)
	}
	ph := NewProvinceHandler(f.pool, nil, nil, nil, nil)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)
	fetch := func(token string) []map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/buildings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("list %d %s", w.Code, w.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if fetch(f.accessToken)[0]["demolition_kharis"] == nil {
		t.Fatal("owner quote missing")
	}
	tok, _, err := auth.NewService(f.pool, "test-secret").Register(ctx, "observer-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := fetch(tok)[0]["demolition_kharis"]; leaked {
		t.Fatal("foreign realm standing leaked")
	}
	code, body := f.do(t, http.MethodDelete, demolishPath(f, id), nil)
	if code != 200 {
		t.Fatal(code, body)
	}
	actual := body["demolition_kharis"].(map[string]any)
	if actual["cap_after"] != float64(75) || actual["kharis_after"] != float64(50) {
		t.Fatal(actual)
	}
}

func TestTempleDemolitionNotificationFailureRollsBackEverything(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	id := seedDemolitionTemple(t, f, 2, 80)
	if _, err := f.pool.Exec(ctx, `ALTER TABLE notifications ADD CONSTRAINT reject_demolition_test CHECK(kind<>'TempleDemolitionWrath') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	defer f.pool.Exec(ctx, `ALTER TABLE notifications DROP CONSTRAINT reject_demolition_test`)
	code, _ := f.do(t, http.MethodDelete, demolishPath(f, id), nil)
	if code != 500 {
		t.Fatal(code)
	}
	var amount float64
	f.pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE settlement_id=$1`, f.settlementID).Scan(&amount)
	if amount != 80 {
		t.Fatal("debt not rolled back", amount)
	}
	var n int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM buildings WHERE id=$1`, id).Scan(&n)
	if n != 1 {
		t.Fatal("temple deleted")
	}
	f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE world_id=$1 AND event_type IN ('TempleDemolitionWrath','BuildingDemolished')`, f.worldID).Scan(&n)
	if n != 0 {
		t.Fatal("event escaped rollback", n)
	}
}

func TestTempleDemolitionCancellationAndOrdinaryBuildingHaveNoDebt(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	seedDemolitionTemple(t, f, 1, 40)
	var queued uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO build_queue(settlement_id,world_id,building_type,god_id,complete_at) SELECT id,world_id,'temple','velchanos',now() FROM settlements WHERE id=$1 RETURNING id`, f.settlementID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if code, body := f.do(t, http.MethodDelete, demolishPath(f, queued), nil); code != 200 || body["cancelled"] != true {
		t.Fatal(code, body)
	}
	id := seedQuarryWithCrew(t, f)
	if code, body := f.do(t, http.MethodDelete, demolishPath(f, id), nil); code != 200 {
		t.Fatal(code, body)
	}
	var amount float64
	f.pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE settlement_id=$1`, f.settlementID).Scan(&amount)
	if amount != 40 {
		t.Fatal("non-temple/cancel charged", amount)
	}
	var n int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='TempleDemolitionWrath'`, f.worldID).Scan(&n)
	if n != 0 {
		t.Fatal("false wrath", n)
	}
}
