package main

import "testing"

func answerSett(net any, days any) map[string]any {
	return map[string]any{"forecast": map[string]any{"food": map[string]any{"net": net, "days": days}}}
}

// One answer, the same words as the web (food_line.js): net after everything that eats,
// and the days left only when the food is falling.
func TestFoodAnswerLine(t *testing.T) {
	for _, c := range []struct {
		net, days any
		want      string
	}{
		{0.42, nil, "Food +0.4 per game day"},
		{-2.04, 5.9, "Food −2 per game day · lasts 5.9 game days"},
		{-1.0, 1.0, "Food −1 per game day · lasts 1 game day"},
		{0.0, nil, "Food about even"},
		{-0.01, 120000.0, "Food about even"},
		{-0.15, 8000.0, "Food −0.2 per game day · lasts over 1,000 game days"},
		{3.0, 8000.0, "Food +3 per game day"},
	} {
		if got := foodAnswerLine(answerSett(c.net, c.days)); got != c.want {
			t.Errorf("net %v days %v: %q, want %q", c.net, c.days, got, c.want)
		}
	}
	// the other server figures are different quantities and never the answer
	other := map[string]any{"food_net_per_tick_after_upkeep": 1.0, "food": map[string]any{"food_net_per_tick": 2.0, "coverage_ticks": 240.0}}
	if got := foodAnswerLine(other); got != "" {
		t.Errorf("without a forecast there is no answer, got %q", got)
	}
}

func TestCityFoodLineKeepsStocksBesideTheAnswer(t *testing.T) {
	sett := answerSett(0.4, nil)
	sett["food"] = map[string]any{"goods": map[string]any{"grain": 1200.0, "fish": 0.0}}
	if got, want := cityFoodLine(sett), "Food +0.4 per game day (stocks: 0 fish, 1200 grain)"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
