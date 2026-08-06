package economy

import "math/rand"

// Dice är den probabilistiska rollens seam, samma form som clock.Clock:
// produktionen kastar på riktigt, testet kastar det det behöver.
type Dice interface {
	Float64() float64 // [0,1) — math/rand.Float64:s kontrakt
	Intn(n int) int   // [0,n)  — math/rand.Intn:s kontrakt
}

// wallDice is the production Dice. It delegates to the global math/rand
// source, so production behaves bit-for-bit as before this seam existed.
type wallDice struct{}

func (wallDice) Float64() float64 { return rand.Float64() }
func (wallDice) Intn(n int) int   { return rand.Intn(n) }

// NewWallDice returns the production Dice. Exported so packages outside
// economy (e.g. combat, at KR3 battle initiation) can populate a Dice-typed
// field with the real production seam without reaching into an unexported
// type — the struct itself (wallDice) stays private and unchanged.
func NewWallDice() Dice { return wallDice{} }

// seededDice is a deterministic Dice backed by its own math/rand source,
// independent of the global one. KR3 (megaron_plan_kr3_stridssystem.md §3)
// uses this to make a battle round reproducible: given the same seed, the
// exact sequence of Float64()/Intn(n) calls it produces is bit-for-bit
// identical every time, in any process, forever (math/rand's algorithm is
// part of its documented compatibility contract for a given Go version).
type seededDice struct {
	r *rand.Rand
}

// NewSeededDice creates a Dice whose entire roll sequence is determined by
// seed. Two seededDice built from the same seed and asked for the same
// sequence of calls (same count of Float64()/Intn(n) calls, same n's) always
// produce the same sequence of results — this is what lets a KR3 battle round
// be replayed exactly from (battles.seed, tick_index, round_index) alone.
func NewSeededDice(seed int64) Dice {
	return &seededDice{r: rand.New(rand.NewSource(seed))}
}

func (d *seededDice) Float64() float64 { return d.r.Float64() }
func (d *seededDice) Intn(n int) int   { return d.r.Intn(n) }
