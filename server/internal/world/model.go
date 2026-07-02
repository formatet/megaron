// Package world contains the domain model for worlds, their state, and collapse logic.
package world

import (
	"time"

	"github.com/google/uuid"
)

// State is the lifecycle state of a world.
type State string

const (
	StateForming    State = "forming"
	StateActive     State = "active"
	StateCollapsing State = "collapsing"
	StateEnded      State = "ended"
)

// World is a single game instance. Each world is fully self-contained.
type World struct {
	ID             uuid.UUID
	Name           string
	State          State
	Prestige       int
	EraNumber      int
	EraStartedAt   time.Time // kept for DB scan compatibility; not used in collapse logic
	MaxProvinces   int
	MinEraWeeks    int // legacy field; kept for DB scan compatibility
	MaxEraWeeks    int // legacy field; kept for DB scan compatibility
	KingdomMinSize int
	KingdomMaxSize int
	MapSeed        int64
	MapWidth       int
	MapHeight      int
	CurrentTick    int
	CreatedAt      time.Time
}

// Terrain describes the type of a hex tile.
type Terrain string

const (
	TerrainPlains            Terrain = "plains"
	TerrainForestOliveGrove  Terrain = "forest_olive_grove"
	TerrainHills             Terrain = "hills"
	TerrainMountainLimestone Terrain = "mountain_limestone"
	TerrainMountainRed       Terrain = "mountain_red"
	TerrainCoastalSea        Terrain = "coastal_sea"
	TerrainDeepSea           Terrain = "deep_sea"
	TerrainRiverValley       Terrain = "river_valley"
	TerrainRiverDelta        Terrain = "river_delta"
	TerrainScrubMaquis       Terrain = "scrub_maquis"
	TerrainSemiDesert        Terrain = "semi_desert"
)

// MapTile is a single hex in the world map.
type MapTile struct {
	WorldID       uuid.UUID
	Q             int
	R             int
	Terrain       Terrain
	Coastal       bool // true = land tile adjacent to coastal_sea (coastal property, not terrain type)
	Fertility     float64
	Mineral       float64
	CopperDeposit bool
	TinDeposit    bool
	SilverDeposit bool
	CedarDeposit  bool
}
