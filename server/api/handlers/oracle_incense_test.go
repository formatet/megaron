package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/religion"
)

func TestPotniaVisionFindsUnknownIncenseAsMemory(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown_incense", true: "already_known_incense"}[known], func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			for _, p := range hexgrid.Disk(hexgrid.Coord{Q: 40, R: 0}, 2) {
				if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,incense_deposit) VALUES($1,$2,$3,'scrub_maquis',$4)`, f.world, p.Q, p.R, p.Q == 40 && p.R == 0); err != nil {
					t.Fatal(err)
				}
			}
			if known {
				if _, err := f.pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,40,0)`, f.world, f.player); err != nil {
					t.Fatal(err)
				}
			}
			rec := f.cast(f.cities[0], religion.PrayerPotniaVision)
			if rec.Code != 200 {
				t.Fatalf("rite=%d %s", rec.Code, rec.Body.String())
			}
			var out struct {
				Effect struct {
					Ore *struct {
						Q, R int
						Good string `json:"ore"`
					} `json:"revealed_ore"`
					Tiles []any  `json:"revealed_tiles"`
					Tier  string `json:"tier"`
				}
				Message string
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if known {
				if out.Effect.Ore != nil || len(out.Effect.Tiles) != 0 {
					t.Fatalf("known incense was selected: %s", rec.Body.String())
				}
			} else {
				if out.Effect.Ore == nil || out.Effect.Ore.Q != 40 || out.Effect.Ore.Good != "incense" || len(out.Effect.Tiles) != 19 || out.Effect.Tier != "remembered" {
					t.Fatalf("unknown incense not revealed as radius2 memory: %s", rec.Body.String())
				}
				if !strings.Contains(out.Message, "source of incense") {
					t.Fatalf("incense described as ore: %s", out.Message)
				}
			}
			if !strings.Contains(religion.PrayerSpecs[religion.PrayerPotniaVision].Description, "incense") {
				t.Fatal("current prayer description hides incense eligibility")
			}
		})
	}
}
