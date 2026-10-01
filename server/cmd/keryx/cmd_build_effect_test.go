package main

import "testing"

func TestBuildEffectLine_NewVsUpgrade(t *testing.T) {
	resp := &placementOptionsResp{
		Hexes: []placementHex{
			{HexQ: 1, HexR: 0, Building: &placementHexBuilding{Type: "farm", Level: 1, UpgradeEffect: "grain production ×1.7 → ×2.4"}},
			{HexQ: 2, HexR: 0},
		},
		ValidHexesForBuilding: map[string][]placementValidHex{
			"farm": {{Q: 2, R: 0, Effect: "grain production ×1.7 · space for 4 more workers"}},
			"mine": {{Q: 1, R: 0, Effect: "silver can be mined · space for 5 workers"}},
		},
	}
	if got := buildEffectLine(resp, "farm", 2, 0); got != "grain production ×1.7 · space for 4 more workers" {
		t.Errorf("new build: %q", got)
	}
	if got := buildEffectLine(resp, "farm", 1, 0); got != "grain production ×1.7 → ×2.4" {
		t.Errorf("upgrade: %q", got)
	}
	if got := buildEffectLine(resp, "mine", 1, 0); got != "silver can be mined · space for 5 workers" {
		t.Errorf("other type on same hex: %q", got)
	}
	if got := buildEffectLine(resp, "farm", 9, 9); got != "" {
		t.Errorf("unknown hex: %q", got)
	}
}
