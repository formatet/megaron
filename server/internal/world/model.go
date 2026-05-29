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
	CollapseStable    CollapsePhase = "stable"
	CollapseWarning   CollapsePhase = "warning"
	CollapseCollapsing CollapsePhase = "collapsing"
	CollapseEnded     CollapsePhase = "ended"
)

// World is a single game instance. Each world is fully self-contained.
type World struct {
	ID             uuid.UUID
	Name           string
	State          State
	Prestige       int
	EraNumber      int
	EraStartedAt   time.Time
	MaxProvinces   int
	MinEraWeeks    int
	MaxEraWeeks    int
	KingdomMinSize int
	KingdomMaxSize int
	MapSeed        int64
	MapWidth       int
	MapHeight      int
	CreatedAt      time.Time
}

// CollapseState is the computed collapse risk for a world.
type CollapseState struct {
	WorldID      uuid.UUID
	EraWeek      int
	CollapseRisk float64
	Phase        CollapsePhase
}

// ComputeCollapse calculates the collapse state for a world at a given time.
// The formula is hidden from players. Risk reaches 1.0 at week 25 (configurable).
// High prestige and active wars accelerate collapse risk.
func ComputeCollapse(w *World, activeWarCount int, at time.Time) CollapseState {
	eraWeek := int(at.Sub(w.EraStartedAt).Hours() / (7 * 24))

	var risk float64
	if eraWeek >= w.MinEraWeeks {
		risk = float64(eraWeek-w.MinEraWeeks) / float64(w.MaxEraWeeks-w.MinEraWeeks)
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
	case w.State == StateEnded:
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
		EraWeek:      eraWeek,
		CollapseRisk: risk,
		Phase:        phase,
	}
}

// Terrain describes the type of a hex tile.
type Terrain string

const (
	TerrainPlains   Terrain = "plains"
	TerrainForest   Terrain = "forest"
	TerrainHills    Terrain = "hills"
	TerrainMountain Terrain = "mountain"
	TerrainCoast    Terrain = "coast"
	TerrainSea      Terrain = "sea"
)

// MapTile is a single hex in the world map.
type MapTile struct {
	WorldID       uuid.UUID
	Q             int
	R             int
	Terrain       Terrain
	Fertility     float64
	Mineral       float64
	CopperDeposit bool
	TinDeposit    bool
}
