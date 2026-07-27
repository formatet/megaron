package settlement

import "testing"

// Tröskeln är kalibrering och får justeras. FORMEN får inte gå sönder: ledet
// ska vara monotont i befolkningen och båda leden ska nås inom spannet mellan
// grundningsgolvet (101) och det mjuka befolkningstaket (30 000). Kartan ritar
// en annan siluett per led, så ett hål eller ett hopp bakåt syns direkt som en
// stad som krymper när den växer.
func TestSizeTier_MonotonicOverTheWholeRange(t *testing.T) {
	prev := SizeTier(0)
	seen := map[int]bool{prev: true}
	for pop := 1; pop <= 30000; pop++ {
		got := SizeTier(pop)
		if got < prev {
			t.Fatalf("ledet föll bakåt vid pop=%d: %d → %d", pop, prev, got)
		}
		if got > prev+1 {
			t.Fatalf("ledet hoppade över ett steg vid pop=%d: %d → %d", pop, prev, got)
		}
		prev, seen[got] = got, true
	}
	for tier := SizeTierHamlet; tier <= SizeTierTown; tier++ {
		if !seen[tier] {
			t.Errorf("led %d nås aldrig i spannet 0–30000 — en siluett ritas aldrig", tier)
		}
	}
}

func TestSizeTier_FoundingFloorIsTheSmallestSilhouette(t *testing.T) {
	// 101 är golvet en nygrundad metropolis landar på (kharis/tick.go). Den ska
	// ritas som ett par hus på en gård, inte som en stad.
	if got := SizeTier(101); got != SizeTierHamlet {
		t.Errorf("nygrundad stad (pop 101) fick led %d, ville ha %d", got, SizeTierHamlet)
	}
	// 30 000 är det mjuka taket. Knossos ska bära stadssiluetten.
	if got := SizeTier(30000); got != SizeTierTown {
		t.Errorf("pop 30000 fick led %d, ville ha %d", got, SizeTierTown)
	}
	// Negativ/noll befolkning får aldrig ge något annat än det minsta ledet —
	// en kollapsad stad ritas av en egen gren i renderaren, men koden här får
	// inte kunna svara med en stad.
	if got := SizeTier(-5); got != SizeTierHamlet {
		t.Errorf("pop -5 fick led %d, ville ha %d", got, SizeTierHamlet)
	}
}

// Tröskeln själv, från båda hållen. 800 är Timothys tal (2026-07-27) och det är
// den enda siffran i filen som en spelare kan MÄRKA — en stad som passerar den
// byter utseende på kartan.
func TestSizeTier_ThresholdIsExact(t *testing.T) {
	if got := SizeTier(SizeTierThreshold - 1); got != SizeTierHamlet {
		t.Errorf("pop %d fick led %d, ville ha hamlet", SizeTierThreshold-1, got)
	}
	if got := SizeTier(SizeTierThreshold); got != SizeTierTown {
		t.Errorf("pop %d fick led %d, ville ha town", SizeTierThreshold, got)
	}
}
