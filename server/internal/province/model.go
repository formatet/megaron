// Package province contains the domain model for hex map tiles and shared types.
package province

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Culture identifies a player's cultural background.
type Culture string

const (
	CultureAkhaier  Culture = "akhaier"
	CultureKhemetiu Culture = "khemetiu"
	CultureKnaani   Culture = "knaani"
	CultureThrakes  Culture = "thrakes"
	CulturePelasger Culture = "pelasger"
	CultureHatti    Culture = "hatti"
)

// MapPosition is an axial hex coordinate.
type MapPosition struct {
	Q int
	R int
}

// ResourceState holds the lazy-evaluated state of a single resource.
type ResourceState struct {
	Amount        float64
	RatePerMinute float64
	LastCalcAt    time.Time
	Cap           float64
}

// Current returns the resource amount at the given time, capped at Cap.
func (rs ResourceState) Current(at time.Time) float64 {
	elapsed := at.Sub(rs.LastCalcAt).Minutes()
	v := rs.Amount + elapsed*rs.RatePerMinute
	return math.Min(math.Max(v, 0), rs.Cap)
}

// ResourceLedger holds all resource states for a settlement.
type ResourceLedger struct {
	Gold   ResourceState
	Food   ResourceState
	Lumber ResourceState
	Stone  ResourceState
	Iron   ResourceState
	Kharis ResourceState
}

// Snapshot returns current values for all resources at the given time.
func (rl ResourceLedger) Snapshot(at time.Time) map[string]float64 {
	return map[string]float64{
		"gold":   rl.Gold.Current(at),
		"food":   rl.Food.Current(at),
		"lumber": rl.Lumber.Current(at),
		"stone":  rl.Stone.Current(at),
		"iron":   rl.Iron.Current(at),
		"kharis": rl.Kharis.Current(at),
	}
}

// ArmyComposition describes the military units in a settlement or marching army.
type ArmyComposition struct {
	Infantry      int
	Cavalry       int
	Catapult      int
	Priest        int
	Ship          int
	EliteInfantry int
}

// Building represents a constructed improvement in a settlement.
type Building struct {
	ID           uuid.UUID
	SettlementID uuid.UUID
	Type         BuildingType
	Level        int
	BuiltAt      time.Time
}

// BuildingType is the kind of building.
type BuildingType string

const (
	BuildingFarm        BuildingType = "farm"
	BuildingBarracks    BuildingType = "barracks"
	BuildingMine        BuildingType = "mine"
	BuildingLumbermill  BuildingType = "lumbermill"
	BuildingStonequarry BuildingType = "stonequarry"
	BuildingMarket      BuildingType = "market"
	BuildingWall        BuildingType = "wall"
	BuildingTower       BuildingType = "tower"
	BuildingHarbour     BuildingType = "harbour"
	BuildingFoundry     BuildingType = "foundry"
	BuildingStable      BuildingType = "stable"
	BuildingBronzeWall  BuildingType = "bronze_wall"
)

// Province is a hex tile — terrain and territory state only.
// Inhabited data (resources, army, name, owner) lives in the Settlement that
// references this province via settlement.province_id.
type Province struct {
	ID             uuid.UUID
	WorldID        uuid.UUID
	MapTile        MapPosition
	TerrainType    string
	TerritoryState string // 'free' | 'controlled'
	ControllerID   *uuid.UUID
	CopperDeposit  bool
	TinDeposit     bool
}

// MarchIntent identifies why an army is moving.
type MarchIntent string

const (
	MarchAttack    MarchIntent = "attack"
	MarchSupport   MarchIntent = "support"
	MarchReinforce MarchIntent = "reinforce"
)

// MarchingArmy is an army in transit between provinces.
type MarchingArmy struct {
	ID            uuid.UUID
	WorldID       uuid.UUID
	OriginID      uuid.UUID
	TargetID      uuid.UUID
	Army          ArmyComposition
	DepartsAt     time.Time
	ArrivesAt     time.Time
	Intent        MarchIntent
	SupportTarget *uuid.UUID
	Resolved      bool
}
