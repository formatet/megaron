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
	"formatet/megaron/server/internal/religion"
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
		count, base, burden, err := kharis.ProjectDecay(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		net, hasTemples, _, _, err := kharis.ProjectDailyNet(ctx, pool, pid, wid)
		if err != nil {
			t.Fatal(err)
		}
		if count != i+1 || hasTemples {
			t.Fatalf("bad fixture count=%d temples=%v", count, hasTemples)
		}
		for key, want := range map[string]float64{"kharis_city_count": float64(count), "kharis_base_decay": base, "kharis_city_decay": burden, "kharis_daily_decay": base + burden, "kharis_net_per_tick": net} {
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
		fmt.Sprintf("a %.0f%% chance", 100*kharis.PunishmentProbability(religion.MoodIndifferent-1, false)),
		"including your capital", "whether or not they have temples", "random eligible city",
		"A standing temple protects you from punishment",
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
