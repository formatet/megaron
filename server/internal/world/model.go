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

// CollapsePhase describes the current collapse risk level visible to players.
type CollapsePhase string

const (
	CollapseStable     CollapsePhase = "stable"
	CollapseWarning    CollapsePhase = "warning"
	CollapseCollapsing CollapsePhase = "collapsing"
	CollapseEnded      CollapsePhase = "ended"
)

// WorldLifeTicks is the total number of ticks a world lives (720 ticks ≈ 1 month at 1 tick/hour).
const WorldLifeTicks = 720

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

// CollapseState is the computed collapse risk for a world.
type CollapseState struct {
	WorldID      uuid.UUID
	Tick         int // current world tick (replaces legacy EraWeek)
	CollapseRisk float64
	Phase        CollapsePhase
}

// ComputeCollapse calculates the collapse state for a world at a given tick.
// Progress = currentTick / WorldLifeTicks; risk ramps in the last quarter (ticks 540–720).
// A world ends when currentTick >= WorldLifeTicks.
// High prestige and active wars accelerate collapse risk.
func ComputeCollapse(w *World, activeWarCount int, currentTick int) CollapseState {
	var risk float64
	if currentTick >= WorldLifeTicks {
		risk = 1.0
	} else if currentTick >= 540 {
		risk = float64(currentTick-540) / float64(WorldLifeTicks-540)
	}

	// Prestige and wars add a modest modifier (capped to avoid runaway values).
	prestigeMod := float64(w.Prestige) / 10000.0
	warMod := float64(activeWarCount) * 0.02
	risk += prestigeMod + warMod
	if risk > 1.0 {
		risk = 1.0
	}

	var phase CollapsePhase
	switch {
	case w.State == StateEnded || currentTick >= WorldLifeTicks:
		phase = CollapseEnded
	case risk >= 0.7:
		phase = CollapseCollapsing
	case risk >= 0.3:
		phase = CollapseWarning
	default:
		phase = CollapseStable
	}

	return CollapseState{
		WorldID:      w.ID,
		Tick:         currentTick,
		CollapseRisk: risk,
		Phase:        phase,
	}
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
