// material-measure dumps fresh mapgen geometry for offline catchment analysis.
// It never opens a database or reads the process game configuration.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"formatet/megaron/server/internal/world"
)

type debugID struct{}

func (debugID) String() string { return "material-measure" }
func main() {
	seed := flag.Int64("seed", 42, "requested map seed")
	w := flag.Int("w", 64, "map width")
	h := flag.Int("h", 64, "map height")
	flag.Parse()
	generated, effective := world.GenerateMap(debugID{}, *seed, *w, *h)
	type tile struct {
		Q       int    `json:"q"`
		R       int    `json:"r"`
		Terrain string `json:"terrain"`
		Coastal bool   `json:"coastal"`
	}
	tiles := make([]tile, 0, len(generated))
	for _, t := range generated {
		tiles = append(tiles, tile{t.Q, t.R, string(t.Terrain), t.Coastal})
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Requested int64  `json:"requested_seed"`
		Effective int64  `json:"effective_seed"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Tiles     []tile `json:"tiles"`
	}{*seed, effective, *w, *h, tiles}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
