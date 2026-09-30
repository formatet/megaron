package main

import "testing"

func TestBuildEffectLine_NewVsUpgrade(t *testing.T) {
	resp := &placementOptionsResp{
		Hexes: []placementHex{
			{HexQ: 1, HexR: 0, Building: &placementHexBuilding{Type: "farm", Level: 1, UpgradeEffect: "1.7 → 2.4 grain per worker"}},
			{HexQ: 2, HexR: 0},
		},
		ValidHexesForBuilding: map[string][]placementValidHex{
			"farm": {{Q: 2, R: 0, Effect: "grain 4 × 1.0 → 8 × 1.7"}},
			"mine": {{Q: 1, R: 0, Effect: "silver — → 5 × 2.3"}},
		},
	}
	if got := buildEffectLine(resp, "farm", 2, 0); got != "grain 4 × 1.0 → 8 × 1.7" {
		t.Errorf("new build: %q", got)
	}
	if got := buildEffectLine(resp, "farm", 1, 0); got != "1.7 → 2.4 grain per worker" {
		t.Errorf("upgrade: %q", got)
	}
	if got := buildEffectLine(resp, "mine", 1, 0); got != "silver — → 5 × 2.3" {
		t.Errorf("other type on same hex: %q", got)
	}
	if got := buildEffectLine(resp, "farm", 9, 9); got != "" {
		t.Errorf("unknown hex: %q", got)
	}
}
