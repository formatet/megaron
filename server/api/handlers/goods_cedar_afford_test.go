package handlers

import "testing"

// Tylissos 2026-10-25: 0 timber, 749 cedar — the galley (1.5 timber × crew 20)
// showed "insufficient" although Recruit's deductGoods lets cedar pay it.
func TestGoodsAffordableWithCedar(t *testing.T) {
	galley := map[string]float64{"timber": 1.5}
	if !goodsAffordableWithCedar(map[string]float64{"cedar": 749}, galley, 20) {
		t.Error("749 cedar must cover a galley's 30 timber")
	}
	if goodsAffordableWithCedar(map[string]float64{"timber": 10}, galley, 20) {
		t.Error("10 timber must not afford 30 (whole crew, not one man)")
	}
	if goodsAffordableWithCedar(map[string]float64{"timber": 10, "cedar": 19}, galley, 20) {
		t.Error("10 timber + 19 cedar is short of 30")
	}
	// Cedar already owed to the recipe is not spare.
	if goodsAffordableWithCedar(map[string]float64{"cedar": 20}, map[string]float64{"timber": 10, "cedar": 15}, 1) {
		t.Error("only 5 spare cedar; 10 timber not covered")
	}
}
