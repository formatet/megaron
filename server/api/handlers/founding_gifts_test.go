package handlers

import "testing"

// The founding forecast must name the gifts a metropolis is owed HERE — they are
// geography-gated exactly like the grain numbers, and a Wanax comparing two sites
// should not discover them only after the irreversible settle. foundingGifts is
// the pure half ColonizePreview calls; its conditions mirror createMetropolis
// (Demeter) and foundMetropolisFromNomadicHost (Poseidon).

func TestFoundingGifts_DemeterOnlyWhenAFarmWouldHelp(t *testing.T) {
	cases := []struct {
		name                     string
		baseGrain, withFarmGrain float64
		coastal                  bool
		want                     []string
	}{
		{"barren inland — no gift at all", 4.0, 4.0, false, nil},
		{"farmland inland — Demeter only", 4.0, 9.5, false, []string{"demeter_farm"}},
		{"barren coast — Poseidon only", 4.0, 4.0, true, []string{"poseidon_galley"}},
		{"farmland coast — both", 4.0, 9.5, true, []string{"demeter_farm", "poseidon_galley"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := foundingGifts(tc.baseGrain, tc.withFarmGrain, tc.coastal)
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

// Floating-point noise must not conjure a farm: the with-farm figure is computed
// from the same table as the base, so an identical catchment can differ in the
// last bits. Only a real improvement counts.
func TestFoundingGifts_IgnoresFloatNoise(t *testing.T) {
	if got := foundingGifts(7.2, 7.2+1e-12, false); len(got) != 0 {
		t.Fatalf("float noise granted a farm: %v", got)
	}
}

// Every gift must carry text a client can render as-is — an empty label would
// print a bare bullet in both keryx and the map drawer.
func TestFoundingGifts_CarryRenderableText(t *testing.T) {
	for _, g := range foundingGifts(4.0, 9.5, true) {
		if g["key"] == "" || g["label"] == "" || g["detail"] == "" {
			t.Errorf("gift missing text: %v", g)
		}
	}
}
