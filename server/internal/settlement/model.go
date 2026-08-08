// Package settlement contains domain types for inhabited fortress cities.
package settlement

import (
	"time"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// ControlType describes how a settlement controls its province.
type ControlType string

const (
	ControlCapital  ControlType = "capital"
	ControlColony   ControlType = "colony"
	ControlOccupied ControlType = "occupied"
)

// State is the current lifecycle state of a settlement.
type State string

const (
	StateActive State = "active"
	// StateBesieged is UNUSED — superseded by the Settlement.Besieged bool
	// (migration 122). state='active' is read as a general "functioning
	// normally" gate across capabilities/loyalty/religion/succession/
	// march_start; reusing this value for belägring would have silently
	// dropped a besieged settlement out of all of them (Timothy 2026-08-08,
	// decided when megaron_plan_belagring.md's S1+S2 build hit the
	// collision). Left in place only so State's enum isn't renumbered.
	StateBesieged  State = "besieged"
	StateRevolting State = "revolting"
	StateSunk      State = "sunk"
)

// LoyaltyTrend indicates the direction of loyalty change.
type LoyaltyTrend string

const (
	LoyaltyTrendRising  LoyaltyTrend = "rising"
	LoyaltyTrendStable  LoyaltyTrend = "stable"
	LoyaltyTrendFalling LoyaltyTrend = "falling"
)

// Settlement is an inhabited fortress city anchored to a province hex tile.
// Resources, army, loyalty and culture all belong here — not on the terrain tile.
type Settlement struct {
	ID           uuid.UUID
	WorldID      uuid.UUID
	ProvinceID   uuid.UUID // the hex tile this settlement sits in
	Name         string
	CultureID    province.Culture
	OwnerID      *uuid.UUID
	KingdomID    *uuid.UUID
	ControlType  ControlType
	FoundedFrom  *uuid.UUID // parent settlement, for colonies
	GovernorID   *uuid.UUID
	GovernorIsAI bool
	Loyalty      int // 1-4: disgruntled | loyal | devoted | fervent
	LoyaltyTrend LoyaltyTrend
	WallLevel    int
	IsCapital    bool
	State        State
	Besieged     bool // belägring S1+S2 — catchment access denied by an enemy chokepoint (megaron_plan_belagring.md). Orthogonal to State — see migration 122.
	Population   int
	Resources    province.ResourceLedger
	Army         province.ArmyComposition
	UpdatedAt    time.Time
}

// LoyaltyEvent is a single loyalty change record for a settlement.
type LoyaltyEvent struct {
	ID           int64
	SettlementID uuid.UUID
	WorldID      uuid.UUID
	EventType    string
	LoyaltyDelta int
	Reason       string
	CreatedAt    time.Time
}
