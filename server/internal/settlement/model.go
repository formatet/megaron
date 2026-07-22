// Package settlement contains domain types for inhabited fortress cities.
package settlement

import (
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/province"
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
	StateActive    State = "active"
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
