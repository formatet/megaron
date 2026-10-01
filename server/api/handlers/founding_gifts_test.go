package handlers

import "testing"

// The founding forecast must name the gifts a metropolis is owed HERE — they are
// geography-gated exactly like the grain numbers, and a Wanax comparing two sites
// should not discover them only after the irreversible settle. foundingGifts is
// the pure half ColonizePreview calls; its conditions mirror createMetropolis
// (Demeter) and foundMetropolisFromNomadicHost (Poseidon).

func TestFoundingGifts_DemeterOnlyWhenAFarmHexExists(t *testing.T) {
	cases := []struct {
		name     string
		farmGift bool
		coastal  bool
		want     []string
	}{
		{"barren inland — no gift at all", false, false, nil},
		{"farmland inland — Demeter only", true, false, []string{"demeter_farm"}},
		{"barren coast — Poseidon only", false, true, []string{"poseidon_galley"}},
		// EITHER farm OR galley (Timothy 2026-09-28, slice C): the coast is fed
		// by the sea, so farmland there earns no farm.
		{"farmland coast — Poseidon only", true, true, []string{"poseidon_galley"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := foundingGifts(tc.farmGift, tc.coastal)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d gifts, want %d (%v)", len(got), len(tc.want), got)
			}
			for i, key := range tc.want {
				if got[i]["key"] != key {
					t.Errorf("gift %d: got %q, want %q", i, got[i]["key"], key)
				}
			}
		})
	}
}

// Every gift must carry text a client can render as-is — an empty label would
// print a bare bullet in both keryx and the map drawer.
func TestFoundingGifts_CarryRenderableText(t *testing.T) {
	gifts := append(foundingGifts(true, false), foundingGifts(false, true)...)
	for _, g := range gifts {
		if g["key"] == "" || g["label"] == "" || g["detail"] == "" {
			t.Errorf("gift missing text: %v", g)
		}
	}
}
