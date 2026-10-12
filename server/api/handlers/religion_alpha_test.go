package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type alphaDice struct{ roll int }

func (d alphaDice) Intn(n int) int   { return d.roll % n }
func (d alphaDice) Float64() float64 { return float64(d.roll) / 100 }

type alphaFixture struct {
	pool          *pgxpool.Pool
	world, player uuid.UUID
	cities        []uuid.UUID
	token         string
	h             *SettlementHandler
	router        http.Handler
}

func newAlphaFixture(t *testing.T) alphaFixture {
	t.Helper()
	ctx := context.Background()
	f := alphaFixture{pool: riteTestPool(t)}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`)
	must(err)
	must(f.pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',100) RETURNING id`, "alpha-"+uuid.NewString()).Scan(&f.world))
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, f.world) })
	svc := auth.NewService(f.pool, "alpha-secret")
	f.token, _, err = svc.Register(ctx, "alpha-"+uuid.NewString(), "pw")
	must(err)
	claims, err := svc.ValidateAccessToken(f.token)
	must(err)
	f.player = claims.PlayerID
	_, err = f.pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,90,0,100)`, f.player, f.world)
	must(err)
	for q := 0; q < 2; q++ {
		var province, city uuid.UUID
		must(f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,$2,0,'plains') RETURNING id`, f.world, q).Scan(&province))
		must(f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type,is_capital) VALUES($1,$2,$3,'minoan',$4,'capital',$5) RETURNING id`, f.world, province, "Alpha City", f.player, q == 0).Scan(&city))
		f.cities = append(f.cities, city)
		_, err = f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'temple',1)`, city)
		must(err)
		_, err = f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT $1,key,100,0,1000,100 FROM goods`, city)
		must(err)
		_, err = f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains')`, f.world, q)
		must(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	f.h = NewSettlementHandler(f.pool, events.NewStore(f.pool), events.NewScheduler(f.pool, clk), clk, economy.SitosConfig{})
	f.h.RiteDice = alphaDice{0}
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/rite", f.h.Rite)
	f.router = r
	return f
}
func (f alphaFixture) cast(city uuid.UUID, prayer string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]string{"prayer": prayer})
	req := httptest.NewRequest("POST", "/worlds/"+f.world.String()+"/settlements/"+city.String()+"/rite", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}
func (f alphaFixture) stock(t *testing.T, city uuid.UUID, good string) float64 {
	t.Helper()
	var amount float64
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, city, good).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	return amount
}

func TestAlphaPrayerSharedGodCooldownAndSilentAnswer(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	f.h.RiteDice = alphaDice{99}
	if r := f.cast(f.cities[0], religion.PrayerPotniaWalls); r.Code != 200 || !strings.Contains(r.Body.String(), `"success":false`) {
		t.Fatalf("silent response %d %s", r.Code, r.Body.String())
	}
	var count int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM prayer_cooldowns WHERE world_id=$1`, f.world).Scan(&count)
	if count != 0 {
		t.Fatal("silent answer imposed a god cooldown")
	}
	if got := f.stock(t, f.cities[0], "grain"); got != 97 {
		t.Fatalf("silent offering not consumed: %v", got)
	}
	f.h.RiteDice = alphaDice{0}
	if r := f.cast(f.cities[0], religion.PrayerPotniaWalls); r.Code != 200 {
		t.Fatalf("cast walls %d %s", r.Code, r.Body.String())
	}
	var until int
	var owner uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT garrison_defense_until_tick,garrison_defense_owner_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&until, &owner); err != nil {
		t.Fatal(err)
	}
	if until != 106 || owner != f.player {
		t.Fatalf("Walls outcome flag %d/%v", until, owner)
	}
	if r := f.cast(f.cities[1], religion.PrayerPotniaVision); r.Code != 409 {
		t.Fatalf("Potnia prayers did not share realm cooldown: %d %s", r.Code, r.Body.String())
	}
	if got := f.stock(t, f.cities[1], "grain"); got != 100 {
		t.Fatalf("rejected cooldown debited grain %v", got)
	}
	if r := f.cast(f.cities[1], religion.PrayerGreenShoots); r.Code != 200 {
		t.Fatalf("other god rejected %d %s", r.Code, r.Body.String())
	}
}

func TestAlphaGreenShootsCannotRenewActiveHarvest(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	if r := f.cast(f.cities[0], religion.PrayerGreenShoots); r.Code != 200 {
		t.Fatalf("green %d %s", r.Code, r.Body.String())
	}
	var until int
	_ = f.pool.QueryRow(ctx, `SELECT harvest_blessing_until_tick FROM settlements WHERE id=$1`, f.cities[0]).Scan(&until)
	if until != 125 {
		t.Fatalf("Green Shoots span %d, want125", until)
	}
	_, _ = f.pool.Exec(ctx, `DELETE FROM prayer_cooldowns WHERE world_id=$1`, f.world)
	before := f.stock(t, f.cities[0], "grain")
	if r := f.cast(f.cities[0], religion.PrayerGreenShoots); r.Code != 409 {
		t.Fatalf("active harvest renewed: %d %s", r.Code, r.Body.String())
	}
	if after := f.stock(t, f.cities[0], "grain"); after != before {
		t.Fatal("active-effect refusal consumed an offering")
	}
}

func TestAlphaPrayerConcurrentCitiesAndOutcomeRollback(t *testing.T) {
	t.Run("same god across cities", func(t *testing.T) {
		f := newAlphaFixture(t)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for _, city := range f.cities {
			wg.Add(1)
			go func(city uuid.UUID) { defer wg.Done(); codes <- f.cast(city, religion.PrayerPotniaWalls).Code }(city)
		}
		wg.Wait()
		close(codes)
		success, conflict := 0, 0
		for c := range codes {
			if c == 200 {
				success++
			} else if c == 409 {
				conflict++
			} else {
				t.Fatalf("unexpected cast status%d", c)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("concurrent god casts success%d conflict%d", success, conflict)
		}
	})
	t.Run("event write rollback", func(t *testing.T) {
		f := newAlphaFixture(t)
		ctx := context.Background()
		name := "reject_alpha_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		sql := `CREATE FUNCTION ` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.world_id='` + f.world.String() + `'::uuid AND NEW.event_type='RiteCast' THEN RAISE EXCEPTION 'alpha outcome write refused'; END IF; RETURN NEW; END $$; CREATE TRIGGER ` + name + ` BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION ` + name + `() `
		if _, err := f.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = f.pool.Exec(ctx, `DROP TRIGGER IF EXISTS `+name+` ON events; DROP FUNCTION IF EXISTS `+name+`()`)
		})
		if r := f.cast(f.cities[0], religion.PrayerPotniaWalls); r.Code != 500 {
			t.Fatalf("event failure did not fail cast %d %s", r.Code, r.Body.String())
		}
		if f.stock(t, f.cities[0], "grain") != 100 {
			t.Fatal("outcome failure did not roll back offering")
		}
		var count int
		_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM prayer_cooldowns WHERE world_id=$1`, f.world).Scan(&count)
		if count != 0 {
			t.Fatal("outcome failure left cooldown")
		}
		var until *int
		_ = f.pool.QueryRow(ctx, `SELECT garrison_defense_until_tick FROM settlements WHERE id=$1`, f.cities[0]).Scan(&until)
		if until != nil {
			t.Fatal("outcome failure left defense flag")
		}
	})
}

func TestAlphaVisionOnlyNewMemoryRadiusTwo(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	for _, p := range hexgrid.Disk(hexgrid.Coord{Q: 40, R: 0}, 2) {
		_, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,copper_deposit) VALUES($1,$2,$3,'hills',$4)`, f.world, p.Q, p.R, p.Q == 40 && p.R == 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A live ore hex cannot win; a remembered one from the province union cannot win.
	_, _ = f.pool.Exec(ctx, `UPDATE map_tiles SET tin_deposit=true WHERE world_id=$1 AND q=1 AND r=0`, f.world)
	var prov uuid.UUID
	_ = f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,30,0,'hills') RETURNING id`, f.world).Scan(&prov)
	_, err := f.pool.Exec(ctx, `INSERT INTO player_scouted_provinces(world_id,player_id,province_id) VALUES($1,$2,$3)`, f.world, f.player, prov)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,silver_deposit) VALUES($1,30,0,'hills',true)`, f.world)
	_, _ = f.pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,38,0)`, f.world, f.player)
	r := f.cast(f.cities[0], religion.PrayerPotniaVision)
	if r.Code != 200 {
		t.Fatalf("vision %d %s", r.Code, r.Body.String())
	}
	var response struct {
		Effect struct {
			Ore   struct{ Q, R int } `json:"revealed_ore"`
			Tiles []any              `json:"revealed_tiles"`
			Tier  string             `json:"tier"`
		}
	}
	if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Effect.Ore.Q != 40 || len(response.Effect.Tiles) != 18 || response.Effect.Tier != "remembered" {
		t.Fatalf("Vision did not reveal only new radius2 memory: %s", r.Body.String())
	}
	var count int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2`, f.world, f.player).Scan(&count)
	if count != 19 {
		t.Fatalf("memory rows %d want19", count)
	}
}

func TestAlphaVisionMemoryAPIHidesActivity(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	for _, p := range hexgrid.Disk(hexgrid.Coord{Q: 40}, 2) {
		_, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,copper_deposit) VALUES($1,$2,$3,'plains',$4)`, f.world, p.Q, p.R, p.Q == 40 && p.R == 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	var other, prov, city uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "foreign-"+uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,40,0,'plains') RETURNING id`, f.world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,owner_id,name,culture_id,control_type,besieged) VALUES($1,$2,$3,'Hidden City','minoan','capital',true) RETURNING id`, f.world, prov, other).Scan(&city); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,type,category,size,status) VALUES($1,$2,$3,'spearman','land',100,'garrison')`, f.world, other, city); err != nil {
		t.Fatal(err)
	}
	if r := f.cast(f.cities[0], religion.PrayerPotniaVision); r.Code != 200 {
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	wh := &WorldHandler{pool: f.pool, clk: f.h.clk}
	router := f.router.(chi.Router)
	router.Get("/worlds/{worldID}/map", wh.Map)
	router.Get("/worlds/{worldID}/provinces", wh.Provinces)
	read := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/worlds/"+f.world.String()+path, nil)
		req.Header.Set("Authorization", "Bearer "+f.token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("read %s %d %s", path, rec.Code, rec.Body.String())
		}
		return rec
	}
	var tiles []struct {
		Q, R int
		Tier string
	}
	if err := json.Unmarshal(read("/map").Body.Bytes(), &tiles); err != nil {
		t.Fatal(err)
	}
	memory := 0
	for _, tile := range tiles {
		if tile.Q >= 38 {
			if tile.Tier != "remembered" {
				t.Fatalf("oracle promoted tile to live: %+v", tile)
			}
			memory++
		}
	}
	if memory != 19 {
		t.Fatalf("memory patch rows%d want19", memory)
	}
	var markers []struct {
		SettlementID uuid.UUID `json:"settlement_id"`
		Besieged     bool
		Army         int `json:"army_total"`
	}
	if err := json.Unmarshal(read("/provinces").Body.Bytes(), &markers); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range markers {
		if m.SettlementID == city {
			found = true
			if m.Besieged || m.Army != 0 {
				t.Fatalf("remembered city leaked activity: %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("memory city marker missing")
	}
}
