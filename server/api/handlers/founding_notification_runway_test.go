package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFoundingNotificationRunway_Metropolis(t *testing.T) {
	for _, tc := range []struct {
		name        string
		grain, fish float64
	}{{"dust", 5 - 1e-15, 0}, {"fish", 0, 5}, {"real deficit", 4, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := citiesTestPool(t)
			ctx := context.Background()
			wid := seedJoinableWorld(t, pool)
			svc := auth.NewService(pool, "runway-test")
			pid, token := registerViewer(t, ctx, svc, "runway-"+uuid.NewString())
			hub := notify.New()
			hub.SetPool(pool)
			h := NewJoinHandler(pool, events.NewStore(pool), clock.NewTestClock(time.Now()), hub)
			r := chi.NewRouter()
			r.Use(auth.Middleware(svc))
			r.Post("/worlds/{worldID}/join", h.Join)
			r.Post("/worlds/{worldID}/founding/settle", h.Settle)
			send := func(path, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, "/worlds/"+wid.String()+path, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				return rec
			}
			if rec := send("/join", `{}`); rec.Code != 201 {
				t.Fatal(rec.Body.String())
			}
			installFoundingRateFixture(t, pool, wid, tc.grain, tc.fish)
			if rec := send("/founding/settle", `{"name":"Runwaytown"}`); rec.Code != 201 {
				t.Fatal(rec.Body.String())
			}
			var raw []byte
			if err := pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='MetropolisFounded'`, wid, pid).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var b map[string]any
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			net, ok := b["food_net_per_tick"].(float64)
			expected := tc.grain + tc.fish - 5
			if !ok || math.Abs(net-expected) > 1e-14 {
				t.Fatalf("full food net absent/wrong: %+v", b)
			}
			ticks, present := b["grain_ticks"]
			alias, aliasPresent := b["grain_days"]
			if expected < -1e-9 {
				amount := b["grain_amount"].(float64)
				if !present || !aliasPresent || ticks.(float64) != amount/-net || alias != ticks {
					t.Fatalf("real deficit lost runway: %+v", b)
				}
			} else if present || aliasPresent {
				t.Fatalf("dust/fish wrote false runway: %+v", b)
			}

		})
	}
}

func installFoundingRateFixture(t *testing.T, pool *pgxpool.Pool, wid uuid.UUID, grain, fish float64) {
	t.Helper()
	ctx := context.Background()
	name := "founding_rate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ddl := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $fn$
 BEGIN
 IF EXISTS(SELECT 1 FROM settlements s WHERE s.id=NEW.settlement_id AND s.world_id='%s') THEN
 IF NEW.good_key='grain' THEN NEW.rate:=%s; ELSIF NEW.good_key='fish' THEN NEW.rate:=%s; END IF;
 END IF;RETURN NEW;END $fn$;
 CREATE TRIGGER %s BEFORE INSERT OR UPDATE ON settlement_goods FOR EACH ROW EXECUTE FUNCTION %s();`, name, wid, strings.TrimSpace(strconv.FormatFloat(grain, 'g', -1, 64)), strings.TrimSpace(strconv.FormatFloat(fish, 'g', -1, 64)), name, name)
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON settlement_goods; DROP FUNCTION %s();`, name, name)); err != nil {
			t.Error(err)
		}
	})
}
