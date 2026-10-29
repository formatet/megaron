package province

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCodexTerrainTravel(t *testing.T) {
	for _, row := range []struct{ article, terrain string }{{"plains", "plains"},
		{"river-valley", "river_valley"},
		{"river-delta", "river_delta"},
		{"river-ford", "river_ford"},
		{"olive-grove", "forest_olive_grove"},
		{"maquis", "scrub_maquis"},
		{"hills", "hills"},
		{"cedar-forest", "forest_cedar"},
		{"semi-desert", "semi_desert"}} {
		raw, err := os.ReadFile("../../../web/static/codex/" + row.article + ".md")
		if err != nil {
			t.Fatal(err)
		}
		for _, force := range []struct{ label, category string }{{"Foot soldiers", "land"}, {"Chariots", CategoryWarChariot}} {
			want := fmt.Sprintf("| %s | %g |", force.label, TerrainMoveTicksFor(row.terrain, force.category))
			if !strings.Contains(regexp.MustCompile(`\[\[[^|\]]+\|([^\]]+)\]\]`).ReplaceAllString(string(raw), "$1"), want) {
				t.Errorf("%s missing %s", row.article, want)
			}
		}
	}
}
