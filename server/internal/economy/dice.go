package economy

import "math/rand"

// Dice är den probabilistiska rollens seam, samma form som clock.Clock:
// produktionen kastar på riktigt, testet kastar det det behöver.
type Dice interface {
	Float64() float64 // [0,1) — math/rand.Float64:s kontrakt
	Intn(n int) int    // [0,n)  — math/rand.Intn:s kontrakt
}

// wallDice is the production Dice. It delegates to the global math/rand
// source, so production behaves bit-for-bit as before this seam existed.
type wallDice struct{}

func (wallDice) Float64() float64 { return rand.Float64() }
func (wallDice) Intn(n int) int   { return rand.Intn(n) }
