package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/religion"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type waitReasonPrayer struct {
	CanPrepare bool   `json:"can_prepare_offering"`
	ID         string `json:"id"`
	Reason     string `json:"unavailable_reason"`
	Wait       int    `json:"cooldown_remaining_game_days"`
	Active     int    `json:"active_remaining_game_days"`
	Pending    bool   `json:"departure_pending"`
	Affordable bool   `json:"affordable"`
}

func readWaitReasonPrayer(t *testing.T, f alphaFixture, id string) waitReasonPrayer {
	t.Helper()
	var prov uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT province_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(f.pool, nil, f.h.clk, nil, nil)
	r := chi.NewRouter()
	r.With(auth.Middleware(auth.NewService(f.pool, "alpha-secret"))).Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.world.String()+"/provinces/"+prov.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Settlement struct {
			Prayers []waitReasonPrayer `json:"available_prayers"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, p := range body.Settlement.Prayers {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("prayer missing: %s", id)
	return waitReasonPrayer{}
}

func TestPrayerStatusKeepsStructuredWaitAndActiveState(t *testing.T) {
	for _, id := range []string{religion.PrayerPotniaVision, religion.PrayerGreenShoots, religion.PrayerAsterionThread} {
		t.Run(id, func(t *testing.T) {
			f := newAlphaFixture(t)
			rec := f.cast(f.cities[0], id)
			if rec.Code != 200 {
				t.Fatalf("cast=%d %s", rec.Code, rec.Body.String())
			}
			p := readWaitReasonPrayer(t, f, id)
			if p.Reason != "" {
				t.Fatalf("generic reason hides structured waiting/active state: %+v", p)
			}
			if p.Affordable || p.Wait != religion.PrayerSpecs[id].CooldownTicks {
				t.Fatalf("waiting prayer says it can be cast or loses wait: %+v", p)
			}
			if id == religion.PrayerGreenShoots && p.Active <= 0 {
				t.Fatalf("active gift missing: %+v", p)
			}
			if id == religion.PrayerAsterionThread && (!p.Pending || p.Active != 0) {
				t.Fatalf("pending departure treated as timed active gift: %+v", p)
			}
		})
	}
}

func TestPrayerStatusKeepsConcreteBlockersDuringWait(t *testing.T) {
	for _, kind := range []string{"temple", "kharis", "offering"} {
		t.Run(kind, func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			id := religion.PrayerPotniaVision
			want := "insufficient offering"
			if kind != "offering" {
				if r := f.cast(f.cities[0], id); r.Code != 200 {
					t.Fatal(r.Body.String())
				}
			}
			switch kind {
			case "temple":
				want = "no Temple of Potnia"
				if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND god_id='potnia'`, f.cities[0]); err != nil {
					t.Fatal(err)
				}
			case "kharis":
				want = "insufficient kharis"
				if _, err := f.pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=1,kharis_rate=0,kharis_calc_tick=100 WHERE player_id=$1 AND world_id=$2`, f.player, f.world); err != nil {
					t.Fatal(err)
				}
			case "offering":
				if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_goods WHERE settlement_id=$1`, f.cities[0]); err != nil {
					t.Fatal(err)
				}
			}
			p := readWaitReasonPrayer(t, f, id)
			if p.Reason != want || p.Affordable {
				t.Fatalf("concrete blocker lost or ambiguous: got%+v want%q", p, want)
			}
		})
	}
}
