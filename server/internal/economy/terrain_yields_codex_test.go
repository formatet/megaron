package economy

import (
	"context"
	"flag"
	"fmt"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"os"
	"sort"
	"strings"
	"testing"
)

var updateTerrainArticle = flag.String("update-terrain-article", "", "Regenerate one Codex terrain's base production table from the city production owner")

func TestCodexTerrainProduction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, row := range []struct{ slug, terrain string }{{"plains", "plains"}, {"river-valley", "river_valley"}, {"river-delta", "river_delta"}, {"hills", "hills"}, {"olive-grove", "forest_olive_grove"}, {"cedar-forest", "forest_cedar"}} {
		t.Run(row.slug, func(t *testing.T) {
			path := "../../../web/static/codex/" + row.slug + ".md"
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			const start = "<!-- terrain-production -->"
			const end = "<!-- /terrain-production -->"
			if !strings.Contains(string(raw), start) && *updateTerrainArticle != row.slug {
				return
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			var world uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'archived') RETURNING id`, "codex-"+uuid.NewString()).Scan(&world); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,0,0,$2)`, world, row.terrain); err != nil {
				t.Fatal(err)
			}
			opts, err := loadHexOptionsForHexes(ctx, tx, world, []hexgrid.Coord{{Q: 0, R: 0}}, BuildingSet{})
			if err != nil {
				t.Fatal(err)
			}
			if len(opts) != 1 {
				t.Fatalf("options=%d", len(opts))
			}
			opt := opts[0]
			keys := []string{}
			for good, rate := range opt.RatePerGood {
				if rate > 0 {
					keys = append(keys, good)
				}
			}
			sort.Strings(keys)
			table := start + "\n| Good | Worker places | Per worker per game day |\n|---|---|---|\n"
			for _, good := range keys {
				table += fmt.Sprintf("| [[%s]] | %d | %.3g |\n", good, opt.PlaceCapPerGood[good], hexYield(opt.RatePerGood[good], opt.PlaceCapPerGood[good], opt.MultPerGood[good], 1))
			}
			table += end
			text := string(raw)
			if *updateTerrainArticle == row.slug {
				if a := strings.Index(text, start); a >= 0 {
					b := strings.Index(text[a:], end)
					if b < 0 {
						t.Fatal("missing end")
					}
					text = text[:a] + table + text[a+b+len(end):]
				} else {
					text += "\n## Base production\n\nPer hex, without buildings or mineral deposits. Each worker is one citizen group. The places shown are limits for each good; a worker does one job.\n\n" + table + "\n\nBuildings, deposits and refining can change these values. Check City production for this hex's actual output. See [[hex-production-buildings]].\n"
				}
				if err := os.WriteFile(path, []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			} else if !strings.Contains(text, table) {
				t.Fatalf("%s production table differs from city rules; regenerate with -update-terrain-article=%s", row.slug, row.slug)
			}
		})
	}
}
