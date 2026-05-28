// Package religion implements the divine intervention system.
// Gods are probabilistic and lynniga — priest actions shift probability, not guarantee outcomes.
package religion

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// PantheonID identifies a group of related deities.
type PantheonID string

const (
	PantheonOlympian PantheonID = "olympian"
	PantheonEgyptian PantheonID = "egyptian"
	PantheonBaal     PantheonID = "baal"
	PantheonChthonic PantheonID = "chthonic"
)

// InterventionType describes what the gods do when they act.
type InterventionType string

const (
	InterventionPestilence InterventionType = "pestilence" // reduces population
	InterventionStorm      InterventionType = "storm"      // destroys ships and coastal buildings
	InterventionOracle     InterventionType = "oracle"     // reveals hidden info to recipient
	InterventionBlessing   InterventionType = "blessing"   // temporary resource rate bonus
	InterventionSilence    InterventionType = "silence"    // kharis to zero, temples lose power
)

// Temple is a religious structure in a province.
type Temple struct {
	ID         uuid.UUID
	ProvinceID uuid.UUID
	PantheonID PantheonID
	Level      int    // 1-3
	LocalPower float64 // 0.0-1.0
	PriestID   *uuid.UUID
	BuiltAt    time.Time
}

// DivineIntervention is a pending or triggered act of the gods.
type DivineIntervention struct {
	ID          uuid.UUID
	WorldID     uuid.UUID
	PantheonID  PantheonID
	Type        InterventionType
	TargetID    uuid.UUID
	Probability float64    // shaped by priest actions; resolved probabilistically at trigger time
	TriggeredAt *time.Time
	CreatedAt   time.Time
}

// PantheonRegion defines where a pantheon has its strongest influence.
// Power decays with hex distance from the core region centre.
type PantheonRegion struct {
	PantheonID      PantheonID
	CoreQ           int
	CoreR           int
	PowerDecayRate  float64 // power reduction per hex of distance
}

// DefaultPantheonRegions returns approximate geographic centres for each pantheon.
// These are calibrated for a 40×30 map; adjust when map dimensions change.
func DefaultPantheonRegions() []PantheonRegion {
	return []PantheonRegion{
		{PantheonOlympian, 12, 8, 0.05},  // Aegean / Greek world
		{PantheonEgyptian, 28, 22, 0.04}, // Nile delta
		{PantheonBaal, 22, 14, 0.06},     // Levant / Canaan
		{PantheonChthonic, 8, 18, 0.07},  // Underworld — weaker everywhere, strongest in west
	}
}

// LocalPower computes the divine power of a pantheon at a given hex position.
// Returns a value in [0.0, 1.0].
func LocalPower(region PantheonRegion, q, r int) float64 {
	dist := hexDist(q, r, region.CoreQ, region.CoreR)
	p := 1.0 - float64(dist)*region.PowerDecayRate
	return math.Max(0.05, p) // minimum presence everywhere
}

func hexDist(aq, ar, bq, br int) int {
	dq := aq - bq
	dr := ar - br
	return (iAbs(dq) + iAbs(dq+dr) + iAbs(dr)) / 2
}

func iAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
