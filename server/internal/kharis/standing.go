package kharis

import (
	"fmt"

	"formatet/megaron/server/internal/religion"
)

// BlessingProbability is shared by the tick and owner-private standing displays.
// A maintained tick means at least one standing temple, independent of offerings.
func BlessingProbability(amount float64, maintained bool) float64 {
	if maintained && amount >= blessThreshold {
		return blessProbability
	}
	return 0
}

type Standing struct {
	Band                  string  `json:"band"`
	PunishmentProbability float64 `json:"punishment_probability"`
	BlessingProbability   float64 `json:"blessing_probability"`
	// The gods' hand at sea (religion.StormTilt): chance a storm spares a ship,
	// chance it takes one more hull.
	StormSpareProbability float64 `json:"storm_spare_probability"`
	StormWrathProbability float64 `json:"storm_wrath_probability"`
	Summary               string  `json:"summary"`
}

// DescribeStanding describes today's rules, not tomorrow's projected kharis.
// Persist this snapshot with new notices; never reprice historical notices.
func DescribeStanding(amount float64, maintained bool, protectionTicks int) Standing {
	s := Standing{Band: kharisBand(amount), BlessingProbability: BlessingProbability(amount, maintained)}
	if protectionTicks <= 0 {
		s.PunishmentProbability = PunishmentProbability(amount, maintained)
	}
	effect := "no spontaneous blessing or punishment at this standing"
	if protectionTicks > 0 && PunishmentProbability(amount, maintained) > 0 {
		effect = "protected from divine punishment for now"
	}
	if s.PunishmentProbability > 0 {
		effect = fmt.Sprintf("about %.0f%% chance of divine punishment each game day", s.PunishmentProbability*100)
	}
	if s.BlessingProbability > 0 {
		effect = fmt.Sprintf("about %.0f%% chance of a spontaneous blessing each game day", s.BlessingProbability*100)
	}
	if amount >= blessThreshold && !maintained {
		effect = "a standing temple is required for spontaneous blessings"
	}
	s.Summary = fmt.Sprintf("Kharis %.1f (%s) · %s", amount, s.Band, effect)
	s.StormSpareProbability, s.StormWrathProbability = religion.StormTilt(amount)
	if s.StormSpareProbability > 0 {
		s.Summary += fmt.Sprintf(" · at sea, about %.0f%% chance Poseidon holds back a storm's blow", s.StormSpareProbability*100)
	}
	if s.StormWrathProbability > 0 {
		s.Summary += fmt.Sprintf(" · at sea, about %.0f%% chance a storm takes %d more hull", s.StormWrathProbability*100, religion.StormWrathExtraHull)
	}
	return s
}
