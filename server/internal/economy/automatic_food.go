package economy

// AutomaticFood explains grain produced without placed citizens. The remainder
// rule is preserved exactly; changing it is a separate balance decision.
type AutomaticFood struct {
	NearjordGrain   float64 `json:"nearjord_grain_per_tick"`
	RemainderPeople int     `json:"remainder_people"`
	RemainderGrain  float64 `json:"remainder_grain_per_tick"`
}

func (a AutomaticFood) GrainPerTick() float64 { return a.NearjordGrain + a.RemainderGrain }
func AutomaticFoodForPopulation(options []HexOption, population int) AutomaticFood {
	potential := 0.0
	for _, opt := range options {
		potential += opt.RatePerGood[GoodGrain] * opt.MultPerGood[GoodGrain]
	}
	people := population % 100
	return AutomaticFood{NearjordGrain: NearjordGrainPerTick, RemainderPeople: people, RemainderGrain: (potential / REF_LABOR) * float64(people)}
}
