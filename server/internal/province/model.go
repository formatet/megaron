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
	CultureMinoan   Culture = "minoan"
	CultureHatti    Culture = "hatti"
)

// MVPCulture — MVP is minoan (EK1 = B, Timothy 2026-08-05). The other five
// cultures are DEACTIVATED, not removed: their data, prayers and name pools
// stay in place and they are reactivated post-MVP.
const MVPCulture = CultureMinoan

// NormaliseCulture forces every culture to the MVP culture. This is silent
// coercion, not a client error: an empty, unknown or deactivated culture is
// not a mistake a player made, and refusing the request would break existing
// clients days before the test week (Timothy 2026-08-05).
func NormaliseCulture(c string) string {
	return string(MVPCulture)
}

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

// ResourceLedger holds the settlement column resource: silver (the currency shekel).
// Kharis lives on player_world_records (one pool per Wanax per world).
// All other producible goods live in settlement_goods.
type ResourceLedger struct {
	Silver ResourceState
}

// Snapshot returns the current silver value at the given time.
func (rl ResourceLedger) Snapshot(at time.Time) map[string]float64 {
	return map[string]float64{
		"silver": rl.Silver.Current(at),
	}
}

// ResourceDetail is a full resource snapshot including rate and cap.
type ResourceDetail struct {
	Amount float64 `json:"amount"`
	Rate   float64 `json:"rate"`
	Cap    float64 `json:"cap"`
}

// SnapshotFull returns amount, rate, and cap for silver.
func (rl ResourceLedger) SnapshotFull(at time.Time) map[string]ResourceDetail {
	return map[string]ResourceDetail{
		"silver": {Amount: rl.Silver.Current(at), Rate: rl.Silver.RatePerMinute, Cap: rl.Silver.Cap},
	}
}

// ArmyComposition describes the military units in a settlement or marching army.
// Ship = galley (standardskepp, byggd med timber). Display: "Galley".
// DB-kolumnen heter `ship` för bakåtkompatibilitet med ~87 befintliga Go-refs.
type ArmyComposition struct {
	Spearman      int
	WarChariot    int
	Ship          int // galley — DB-kolumn: ship
	EliteInfantry int
	WarGalley     int // krigsgalär, kräver foundry + bronze
	Merchantman   int // handelsskepp, svag strid
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
	BuildingHarbour     BuildingType = "harbour"
	BuildingShipyard    BuildingType = "shipyard"
	BuildingFoundry     BuildingType = "foundry"
	BuildingStable      BuildingType = "stable"
	BuildingTemple      BuildingType = "temple"
	BuildingOlivePress  BuildingType = "olive_press"
	BuildingWinery      BuildingType = "winery"
)

// HexBoundBuildings is the ONE canonical list of building types placed on a
// specific catchment hex — their effect (production rate AND worker cap)
// applies ONLY to that hex, and a settlement may build several of the same
// type, one per hex (megaron_plan_byggnad_pa_hex.md §A, Timothy 2026-09-28).
// Every other building stays city-wide (one per settlement), exactly as
// before this slice.
//
// economy may not import province (G1), so it carries its OWN mirror of this
// exact set (hexBoundBuildingTypes, recompute.go) rather than importing this
// map — kept in sync by a test in a package allowed to import both
// (api/handlers/hex_bound_mirror_test.go). Add a type here → add it there too.
var HexBoundBuildings = map[BuildingType]bool{
	BuildingFarm:        true,
	BuildingMine:        true,
	BuildingLumbermill:  true,
	BuildingStonequarry: true,
}

// Province is a hex tile — terrain and territory state only.
// Inhabited data (resources, army, name, owner) lives in the Settlement that
// references this province via settlement.province_id.
type Province struct {
	ID             uuid.UUID
	WorldID        uuid.UUID
	MapTile        MapPosition
	TerrainType    string
	Coastal        bool   // true = land tile adjacent to coastal_sea
	TerritoryState string // 'free' | 'controlled'
	ControllerID   *uuid.UUID
	CopperDeposit  bool
	TinDeposit     bool
	SilverDeposit  bool
	CedarDeposit   bool
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
