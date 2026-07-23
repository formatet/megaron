// mapgen-debug renders generated world maps to PNG + sidecar JSON without a
// DB or web client — a hundred maps in seconds. Per seed it writes
// map_<W>x<H>_seed<EFFECTIVE_SEED>.png (terrain + deposits), a _overlay.png
// (land components, sea channels, hemispheres, spawn candidates, deposit
// catchments) and a .json with the metrics contract (P4 calibration data).
//
//	go run ./cmd/mapgen-debug -w 56 -h 40 -seed 42 -n 5 -out /tmp/maps
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"formatet/megaron/server/internal/world"
)

// debugID satisfies GenerateMap's worldID parameter (used only in log lines).
type debugID struct{}

func (debugID) String() string { return "mapgen-debug" }

func main() {
	w := flag.Int("w", 56, "map width in columns")
	h := flag.Int("h", 40, "map height in rows")
	seed := flag.Int64("seed", 42, "first requested seed")
	n := flag.Int("n", 1, "number of seeds to generate, starting at -seed")
	out := flag.String("out", ".", "output directory (created if missing)")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mapgen-debug: %v\n", err)
		os.Exit(1)
	}

	failed := false
	for i := 0; i < *n; i++ {
		if err := runSeed(*seed+int64(i), *w, *h, *out); err != nil {
			fmt.Fprintf(os.Stderr, "mapgen-debug: seed %d: %v\n", *seed+int64(i), err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// runSeed generates one map and writes its three artifacts. GenerateMap
// panics when validateMap rejects all maxMapAttempts candidates (untested
// sizes may hit this) — that is a finding, not a crash: recover, report the
// summary line as FAILED and let the remaining seeds run.
func runSeed(requested int64, w, h int, outDir string) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Printf("FAILED w=%d h=%d requested_seed=%d: %v\n", w, h, requested, rec)
		}
	}()

	tiles, eff := world.GenerateMap(debugID{}, requested, w, h)

	m := world.ComputeMapMetrics(tiles, w, h)
	m.RequestedSeed = requested
	m.EffectiveSeed = eff
	// GenerateMap tries seed, seed+1, … and returns the first that validates,
	// so the attempt count is recoverable without touching its signature.
	m.Attempts = eff - requested + 1

	base := filepath.Join(outDir, fmt.Sprintf("map_%dx%d_seed%d", w, h, eff))
	if err := world.ExportDebugPNG(tiles, w, h, base+".png"); err != nil {
		return err
	}
	if err := world.ExportDebugOverlayPNG(tiles, w, h, base+"_overlay.png"); err != nil {
		return err
	}
	if err := m.WriteJSON(base + ".json"); err != nil {
		return err
	}

	// One stable summary line per map — grep-friendly key=value pairs.
	fmt.Printf("map w=%d h=%d requested_seed=%d effective_seed=%d attempts=%d "+
		"land_fraction=%.3f components=%d largest_component_fraction=%.3f spawn_valid=%d "+
		"copper=%d tin=%d silver=%d cedar=%d straits=%d delta=%d river_valley=%d "+
		"target_players=%d player_capacity=%d copper_sources=%d tin_sources=%d silver_sources=%d\n",
		w, h, requested, eff, m.Attempts,
		m.LandFraction, m.LandComponents, m.LargestComponentFraction, m.SpawnValidTiles,
		m.CopperDeposits, m.TinDeposits, m.SilverDeposits, m.CedarDeposits, m.Straits, m.DeltaTiles,
		m.RiverValleyTiles,
		m.TargetPlayers, m.PlayerCapacity, m.CopperSources, m.TinSources, m.SilverSources)
	return nil
}
