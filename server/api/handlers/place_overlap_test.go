package handlers

import (
	"context"
	"strings"
	"testing"

	"formatet/megaron/server/internal/world"
)

// The new kinds pass the API's kind check and the mig 236 CHECK (a one-hex delta is
// the smallest place a fixture can make).
func TestPlaceDeltaCanBeNamed(t *testing.T) {
	n := newPlaceRig(t)
	ctx := context.Background()
	if _, err := n.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,40,0,'river_delta')`, n.world); err != nil {
		t.Fatal(err)
	}
	if _, err := n.pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,40,0)`, n.world, n.viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := n.pool.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick) VALUES($1,'delta',40,0,$2,7)`, n.world, n.viewer); err != nil {
		t.Fatal(err)
	}
	if got := n.do("POST", "delta", "40,0", `{"name":"Nilos Mouth"}`, n.token, ""); got.Code != 200 {
		t.Fatalf("delta claimant cannot name it: HTTP%d %s", got.Code, got.Body)
	}
	got := n.do("GET", "", "", "", n.token, "")
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"kind":"delta","id":"40,0","name":"Nilos Mouth"`) {
		t.Fatalf("named delta missing from catalogue: HTTP%d %s", got.Code, got.Body)
	}
}

// A hex shows the smallest place it belongs to (platstyper P1): a massif on an
// island shows as the massif, the island's shore as the island, and on a tie the
// earlier place in world.Places order stays.
func TestSmallestPlaceByHex(t *testing.T) {
	massif := world.Place{Kind: "massif", Anchor: [2]int{1, 1}, Hexes: [][2]int{{1, 1}}}
	island := world.Place{Kind: "island", Anchor: [2]int{0, 0}, Hexes: [][2]int{{0, 0}, {1, 1}, {2, 2}}}
	rival := world.Place{Kind: "hills", Anchor: [2]int{2, 2}, Hexes: [][2]int{{2, 2}, {5, 5}, {6, 6}}}
	got := smallestPlaceByHex([]world.Place{massif, island, rival})
	if got[[2]int{1, 1}].Kind != "massif" {
		t.Fatalf("massif hex shows %s, want the smaller massif", got[[2]int{1, 1}].Kind)
	}
	if got[[2]int{0, 0}].Kind != "island" {
		t.Fatalf("shore hex shows %s, want island", got[[2]int{0, 0}].Kind)
	}
	if got[[2]int{2, 2}].Kind != "island" {
		t.Fatalf("equal-size tie shows %s, want the earlier place (island)", got[[2]int{2, 2}].Kind)
	}
}
