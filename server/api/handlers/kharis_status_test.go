package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/kharis"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// API parity: every city's status exposes the same realm projection as the
// maintenance owner, including a negative net when the realm has no temples.
func TestProvinceRealmKharisProjection(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	wid := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "kharis-projection-status")
	pid, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "realm-projection-"+uuid.NewString())
	h := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), nil, nil)
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/provinces/{provinceID}", h.Get)
	for i := 0; i < 5; i++ {
		sid := settlementsOverviewSeedSettlement(t, pool, ctx, wid, pid, fmt.Sprintf("Realm city %d", i), i == 0, i*6, 0)
		if i != 0 && i != 4 {
			continue
		}
		var provinceID uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, sid).Scan(&provinceID); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/provinces/"+provinceID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("GET %d %s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Settlement map[string]any `json:"settlement"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		d, err := kharis.ProjectDecay(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		net, hasTemples, _, _, err := kharis.ProjectDailyNet(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		if d.CityCount != i+1 || hasTemples {
			t.Fatalf("bad fixture count=%d temples=%v", d.CityCount, hasTemples)
		}
		for key, want := range map[string]float64{"kharis_city_count": float64(d.CityCount), "kharis_base_decay": d.Base, "kharis_city_decay": d.Cities, "kharis_daily_decay": d.Total, "kharis_net_per_tick": net, "kharis_population": float64(d.Population), "kharis_population_decay": d.Residents, "kharis_production_per_tick": net + d.Total} {
			got, ok := payload.Settlement[key].(float64)
			if !ok || math.Abs(got-want) > 1e-9 {
				t.Errorf("%s=%v want %g", key, payload.Settlement[key], want)
			}
		}
		if known, _ := payload.Settlement["kharis_net_known"].(bool); !known {
			t.Fatal("templeless realm hides its known negative net")
		}
	}
}

func TestKharisPlayerRulesSourceParity(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/kharis.md")
	if err != nil {
		t.Fatal(err)
	}
	article := string(raw)
	for _, want := range []string{
		fmt.Sprintf("**%g Kharis**", kharis.StartingKharis),
		fmt.Sprintf("no temple **%g**", kharis.EffectiveKharisCap(100, 0)),
		fmt.Sprintf("level 1 **%g**", kharis.EffectiveKharisCap(100, 1)),
		fmt.Sprintf("level 2 **%g**", kharis.EffectiveKharisCap(100, 2)),
		fmt.Sprintf("level 3 **%g**", kharis.EffectiveKharisCap(100, 3)),
		fmt.Sprintf("a %.0f%% chance", 100*kharis.PunishmentProbability(kharis.WrathThreshold-1, false)),
		"including your capital", "whether or not they have temples", "random eligible city",
		"with or without a temple",
	} {
		if !strings.Contains(article, want) {
			t.Errorf("Codex missing active rule %q", want)
		}
	}
	h := &JoinHandler{startWanaxes: 3}
	for _, spec := range []hostSpec{newcomerHost, hostSpec{refugee: true}} {
		text := strings.Join(h.openingDispatch(uuid.New(), spec, false).Paragraphs, " ")
		if strings.Contains(text, "holds for") || strings.Contains(text, "144") || strings.Contains(text, "no punishment falls") {
			t.Fatal(text)
		}
		if spec.refugee && (strings.Contains(text, "starts at") || !strings.Contains(text, "standing is kept")) {
			t.Fatal(text)
		}
	}
}

func TestProvinceCultUpkeepDoesNotRevealForeignRealm(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	svc := auth.NewService(f.pool, "alpha-secret")
	token, _, err := svc.Register(ctx, "cult-visitor-"+uuid.NewString(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	var prov uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	h := NewProvinceHandler(f.pool, nil, f.h.clk, nil, nil)
	r := chi.NewRouter()
	r.With(auth.Middleware(svc)).Get("/worlds/{worldID}/provinces/{provinceID}", h.Get)
	req := httptest.NewRequest("GET", "/worlds/"+f.world.String()+"/provinces/"+prov.String(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("visitor %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settlement map[string]any `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kharis_wrath", "kharis_population", "kharis_population_decay", "kharis_daily_decay", "kharis_net_per_tick", "kharis_production_per_tick"} {
		if _, exists := body.Settlement[key]; exists {
			t.Errorf("foreign realm field %s leaked", key)
		}
	}
}

func TestOwnerWrathExpiryUsesWorldAnchorAndStaysPrivate(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick,context) VALUES($1,$2,$2,'divine_punishment','protected',100,'{"until_tick":107}')`, f.world, f.player); err != nil {
		t.Fatal(err)
	}
	status, err := kharis.LoadWrathStatus(ctx, f.pool, f.player, f.world)
	if err != nil {
		t.Fatal(err)
	}
	if status.ProtectionRemainingDays != 7 || status.ProtectionUntil == nil || status.Threshold != 15 || status.DailyProbability != .03 {
		t.Fatalf("bad owner shield %+v", status)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=107 WHERE id=$1`, f.world); err != nil {
		t.Fatal(err)
	}
	status, err = kharis.LoadWrathStatus(ctx, f.pool, f.player, f.world)
	if err != nil {
		t.Fatal(err)
	}
	if status.ProtectionRemainingDays != 0 || status.ProtectionUntil != nil {
		t.Fatal("expiry boundary still protected")
	}
}
