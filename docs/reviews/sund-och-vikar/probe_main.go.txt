// Kopiera till server/cmd/zz_straitprobe/main.go, kör, ta bort. Skriver kartor som JSON för sund*.py.
//   go run ./cmd/zz_straitprobe -w 64 -h 95 -n 12 -out $DIR/maps
// live.json: select json_build_object('w',64,'h',95,'seed',0,'tiles',json_agg(json_build_array(q,r,terrain))) from map_tiles where world_id=...
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"formatet/megaron/server/internal/world"
)

type id struct{}

func (id) String() string { return "probe" }

func main() {
	w := flag.Int("w", 64, "")
	h := flag.Int("h", 95, "")
	seed := flag.Int64("seed", 1, "")
	n := flag.Int("n", 10, "")
	out := flag.String("out", ".", "")
	flag.Parse()
	os.MkdirAll(*out, 0o755)
	for i := 0; i < *n; i++ {
		tiles, eff := world.GenerateMap(id{}, *seed+int64(i)*7919, *w, *h)
		rows := make([][3]interface{}, 0, len(tiles))
		for _, t := range tiles {
			rows = append(rows, [3]interface{}{t.Q, t.R, string(t.Terrain)})
		}
		pl := world.Places(tiles)
		ns, nb, sb := 0, 0, 0
		for _, p := range pl {
			switch p.Kind {
			case "strait":
				ns++
			case "bay":
				nb++
			}
		}
		terr := map[[2]int]world.Terrain{}
		for _, t := range tiles {
			terr[[2]int{t.Q, t.R}] = t.Terrain
		}
		bayRule := 0
		for _, t := range tiles {
			if t.Terrain != world.TerrainCoastalSea {
				continue
			}
			land := 0
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
				if tt, ok := terr[[2]int{t.Q + d[0], t.R + d[1]}]; ok && tt != world.TerrainCoastalSea && tt != world.TerrainDeepSea && tt != world.TerrainLake {
					land++
				}
			}
			if land >= 4 {
				bayRule++
			}
		}
		for _, p := range pl {
			if p.Kind == "strait" {
				for _, h := range p.Hexes {
					t := terr[[2]int{h[0], h[1]}]
					land := 0
					for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
						if tt, ok := terr[[2]int{h[0] + d[0], h[1] + d[1]}]; ok && tt != world.TerrainCoastalSea && tt != world.TerrainDeepSea && tt != world.TerrainLake {
							land++
						}
					}
					if t == world.TerrainCoastalSea && land >= 4 {
						sb++
					}
				}
			}
		}
		fmt.Printf("GO %s straits=%d bays=%d bayrule_hexes=%d of_which_in_strait=%d\n", fmt.Sprintf("m_%dx%d_%d.json", *w, *h, eff), ns, nb, bayRule, sb)
		b, _ := json.Marshal(map[string]interface{}{"w": *w, "h": *h, "seed": eff, "tiles": rows})
		os.WriteFile(filepath.Join(*out, fmt.Sprintf("m_%dx%d_%d.json", *w, *h, eff)), b, 0o644)
	}
}
