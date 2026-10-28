package province

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Codex "Roads" article is the player-facing truth; its stone table and
// factor table must say what the code charges and applies. A tuned number that
// forgets the article goes red here.
func TestCodexRoadsArticleMatchesCode(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/index.json")
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Articles []struct {
			ID      string   `json:"id"`
			Related []string `json:"related"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	ids := []string{"roads"}
	for _, a := range idx.Articles {
		if a.ID == "roads" {
			ids = append(ids, a.Related...)
		}
	}
	md := ""
	for _, id := range ids {
		raw, err := os.ReadFile("../../../web/static/codex/" + id + ".md")
		if err != nil {
			t.Fatal(err)
		}
		md += "\n" + string(raw)
	}
	// A labelled link reads as its label: "[[war-chariot|Chariots]]" is "Chariots" to the player.
	md = regexp.MustCompile(`\[\[[a-z0-9-]+\|([^\]]+)\]\]`).ReplaceAllString(md, "$1")
	rows := []struct{ label, terrain string }{
		{"Plains, river valley, delta", "plains"},
		{"Olive grove, maquis", "forest_olive_grove"},
		{"Hills, cedar forest", "hills"},
		{"Semi-desert", "semi_desert"},
	}
	for _, r := range rows {
		want := fmt.Sprintf("| %s | %g |", r.label, RoadStoneCost(r.terrain))
		if !strings.Contains(md, want) {
			t.Errorf("roads.md lacks %q (code charges %g stone)", want, RoadStoneCost(r.terrain))
		}
	}
	// Cedar forest and delta/valley cost the same as their row-mates.
	for _, same := range [][2]string{{"plains", "river_valley"}, {"plains", "river_delta"}, {"hills", "forest_cedar"}, {"forest_olive_grove", "scrub_maquis"}} {
		if RoadStoneCost(same[0]) != RoadStoneCost(same[1]) {
			t.Errorf("%s and %s are one row in the article but cost %g vs %g", same[0], same[1], RoadStoneCost(same[0]), RoadStoneCost(same[1]))
		}
	}
	factors := []struct{ label, category string }{
		{"Foot soldiers", "land"}, {"Runners and messengers", CategoryCourier},
		{"Caravans", CategoryCaravan}, {"Chariots", CategoryWarChariot},
	}
	for _, f := range factors {
		rt, _ := RoadTicks(f.category)
		want := fmt.Sprintf("| %s | %g game day", f.label, rt)
		if !strings.Contains(md, want) {
			t.Errorf("roads.md lacks %q (code applies %g ticks per hex)", want, rt)
		}
	}
	if roadTicks[CategoryCourier] != roadTicks[CategoryCourierLand] {
		t.Error("runners share one article row but have different factors")
	}
}
