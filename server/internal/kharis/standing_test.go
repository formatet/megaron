package kharis

import (
	"strings"
	"testing"
)

func TestStandingCurrentConsequences(t *testing.T) {
	for _, c := range []struct {
		amount        float64
		temple        bool
		shield        int
		band          string
		punish, bless float64
		text          string
	}{
		{4, false, 0, "Wrathful", .03, 0, "3%"},
		{5, true, 0, "Suspicious", .03, 0, "3%"},
		{14.9, false, 1, "Suspicious", 0, 0, "protected"},
		{15, true, 0, "Suspicious", 0, 0, "no spontaneous"},
		{30, true, 0, "Indifferent", 0, 0, "no spontaneous"},
		{59.9, true, 0, "Indifferent", 0, 0, "no spontaneous"},
		{60, true, 0, "Favorable", 0, .15, "15%"},
		{90, false, 0, "Favorable", 0, 0, "standing temple is required"},
		{90, true, 7, "Favorable", 0, .15, "15%"},
	} {
		s := DescribeStanding(c.amount, c.temple, c.shield)
		if s.Band != c.band || s.PunishmentProbability != c.punish || s.BlessingProbability != c.bless || !strings.Contains(s.Summary, c.text) {
			t.Fatalf("amount=%g temple=%v shield=%d: %+v", c.amount, c.temple, c.shield, s)
		}
	}
}
