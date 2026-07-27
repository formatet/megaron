package settlement

import "testing"

// Trösklarna är kalibrering och får justeras. FORMEN får inte gå sönder: ledet
// ska vara monotont i befolkningen och täcka hela spannet mellan
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
	for tier := SizeTierHamlet; tier <= SizeTierPalace; tier++ {
		if !seen[tier] {
			t.Errorf("led %d nås aldrig i spannet 0–30000 — en siluett ritas aldrig", tier)
		}
	}
}

func TestSizeTier_FoundingFloorIsTheSmallestSilhouette(t *testing.T) {
	// 101 är golvet en nygrundad metropolis landar på (kharis/tick.go). Den ska
	// ritas som ett stenhus, inte som ett palats.
	if got := SizeTier(101); got != SizeTierHamlet {
		t.Errorf("nygrundad stad (pop 101) fick led %d, ville ha %d", got, SizeTierHamlet)
	}
	// 30 000 är det mjuka taket. Knossos ska bära anaktoron.
	if got := SizeTier(30000); got != SizeTierPalace {
		t.Errorf("pop 30000 fick led %d, ville ha %d", got, SizeTierPalace)
	}
	// Negativ/noll befolkning får aldrig ge något annat än det minsta ledet —
	// en kollapsad stad ritas av en egen gren i renderaren, men koden här får
	// inte kunna svara med ett palats.
	if got := SizeTier(-5); got != SizeTierHamlet {
		t.Errorf("pop -5 fick led %d, ville ha %d", got, SizeTierHamlet)
	}
}
