package province

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// waterClasses pins every terrain's water class. A new terrain (lake, slice 2)
// gets a row here and in the three owners: Go (water.go, hex.go), SQL
// (terrain_is_sea / terrain_is_navigable) and JS (web/static/js/megaron/terrain.js).
var waterClasses = []struct {
	terrain                  string
	sea, navigable, landWall bool
}{
	{"coastal_sea", true, true, true},
	{"deep_sea", true, true, true},
	{"river", false, true, true},
	{"river_ford", false, true, false},
	{"plains", false, false, false},
	{"hills", false, false, false},
	{"forest_olive_grove", false, false, false},
	{"forest_cedar", false, false, false},
	{"scrub_maquis", false, false, false},
	{"semi_desert", false, false, false},
	{"mountain_limestone", false, false, false},
	{"mountain_red", false, false, false},
	{"river_valley", false, false, false},
	{"river_delta", false, false, false},
	{"", false, false, false},
}

func TestWaterClassesPinned(t *testing.T) {
	for _, c := range waterClasses {
		if got := IsSea(c.terrain); got != c.sea {
			t.Errorf("IsSea(%q) = %v, want %v", c.terrain, got, c.sea)
		}
		if got := IsNavigable(c.terrain); got != c.navigable {
			t.Errorf("IsNavigable(%q) = %v, want %v", c.terrain, got, c.navigable)
		}
		if got := IsWaterBarrier(c.terrain); got != c.landWall {
			t.Errorf("IsWaterBarrier(%q) = %v, want %v", c.terrain, got, c.landWall)
		}
	}
}

// TestWaterSQLMatchesGo: the SQL owner answers exactly as the Go owner, and a
// NULL terrain stays NULL (the NOT IN lists it replaced excluded NULL rows).
func TestWaterSQLMatchesGo(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, c := range waterClasses {
		var sea, nav bool
		if err := pool.QueryRow(ctx, `SELECT terrain_is_sea($1), terrain_is_navigable($1)`, c.terrain).Scan(&sea, &nav); err != nil {
			t.Fatal(err)
		}
		if sea != IsSea(c.terrain) || nav != IsNavigable(c.terrain) {
			t.Errorf("%q: SQL sea=%v navigable=%v, Go sea=%v navigable=%v", c.terrain, sea, nav, IsSea(c.terrain), IsNavigable(c.terrain))
		}
	}
	var notNav *bool
	if err := pool.QueryRow(ctx, `SELECT NOT terrain_is_navigable(NULL)`).Scan(&notNav); err != nil {
		t.Fatal(err)
	}
	if notNav != nil {
		t.Errorf("NOT terrain_is_navigable(NULL) = %v, want NULL", *notNav)
	}
}

// waterListLine matches a line naming a sea terrain AND another water terrain
// as quoted literals — a hand-written water list. (River-only checks such as
// `t == "river" || t == "river_ford"` ask "is this a river?", not "is this water?".)
var (
	seaLiteral   = regexp.MustCompile(`["'](coastal_sea|deep_sea)["']`)
	waterLiteral = regexp.MustCompile(`["'](coastal_sea|deep_sea|river|river_ford)["']`)
)

// waterListAllowed: the owners, and per-terrain tables that are not a
// classification (keyed by path, value = text the allowed line contains).
var waterListAllowed = map[string]string{
	"server/internal/province/hex.go":         "func IsSea",
	"web/static/js/megaron/terrain.js":        "export function isSea",
	"server/api/handlers/opening_dispatch.go": "TerrainMoveTicksFor(\"deep_sea\"",
	"server/internal/economy/hex_rules.go":    "\"scrub_maquis\"",
	"server/cmd/keryx/river_flow.go":          "\"coastal water\"",
}

// TestNoHardcodedWaterLists fails on a new hand-written water list outside the
// owners, in Go (SQL strings included) and in the web client. internal/world
// (mapgen) is a zero-dependency leaf with its own typed predicates.
func TestNoHardcodedWaterLists(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	for _, dir := range []string{"server", filepath.Join("web", "static", "js")} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if rel == "server/internal/world" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".test.mjs") ||
				!(strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".js")) {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(src), "\n") {
				if !seaLiteral.MatchString(line) {
					continue
				}
				if names := waterLiteral.FindAllString(line, -1); len(names) < 2 {
					continue
				}
				if allow, ok := waterListAllowed[rel]; ok && strings.Contains(line, allow) {
					continue
				}
				t.Errorf("%s:%d hardcodes a water list — use province.IsSea/IsNavigable/IsWaterBarrier, "+
					"terrain_is_sea/terrain_is_navigable or terrain.js: %s", rel, i+1, strings.TrimSpace(line))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
