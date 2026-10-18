package economy

// AutomaticFood is food made without placed citizens. Remainder people work
// an invisible fraction of the best eligible food worker, taking no slot.
type AutomaticFood struct {
	NearjordGrain   float64 `json:"nearjord_grain_per_tick"`
	RemainderPeople int     `json:"remainder_people"`
	RemainderGrain  float64 `json:"remainder_grain_per_tick"`
	RemainderFish   float64 `json:"remainder_fish_per_tick"`
}

func (a AutomaticFood) GrainPerTick() float64 { return a.NearjordGrain + a.RemainderGrain }
func (a AutomaticFood) FoodPerTick() float64  { return a.GrainPerTick() + a.RemainderFish }

func AutomaticFoodForPopulation(options []HexOption, population int) AutomaticFood {
	a := AutomaticFood{NearjordGrain: NearjordGrainPerTick, RemainderPeople: population % 100}
	best := 0.0
	bestGood := ""
	for _, opt := range options {
		for _, good := range []string{GoodGrain, GoodFish} {
			if opt.PlaceCapPerGood[good] <= 0 {
				continue
			}
			yield := HexYieldPerWorker(opt.RatePerGood[good], opt.MultPerGood[good])
			// Grain wins equal yields deterministically. Actual occupancy is irrelevant:
			// this invisible work neither needs nor consumes a real worker's slot.
			if yield > best || yield == best && yield > 0 && good == GoodGrain {
				best = yield
				bestGood = good
			}
		}
	}
	amount := float64(a.RemainderPeople) / 100 * best
	if bestGood == GoodFish {
		a.RemainderFish = amount
	} else {
		a.RemainderGrain = amount
	}
	return a
}
