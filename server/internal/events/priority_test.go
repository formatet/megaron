package events

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestEveryScheduledTypeHasAPriority is the guard that keeps the day's order a
// decision rather than an accident. Adding a ScheduledEventType without placing
// it in the ladder is easy to do and invisible at runtime — the type simply
// lands on DefaultTickPriority and races whatever else shares that number. The
// test reads the const block itself, so it sees a new type the moment it is
// declared, without anyone having to remember to register it anywhere.
func TestEveryScheduledTypeHasAPriority(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "scheduler.go", nil, 0)
	if err != nil {
		t.Fatalf("parse scheduler.go: %v", err)
	}

	var declared []string
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != "ScheduledEventType" {
			return true
		}
		for _, name := range vs.Names {
			declared = append(declared, name.Name)
		}
		return true
	})

	if len(declared) < 20 {
		t.Fatalf("only found %d ScheduledEventType constants — the parser stopped seeing them, "+
			"so this test would pass vacuously; fix the parse before trusting it", len(declared))
	}

	// Map Go identifier -> the string value, via the same const block.
	values := map[string]ScheduledEventType{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != "ScheduledEventType" {
			return true
		}
		for i, name := range vs.Names {
			if i < len(vs.Values) {
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					values[name.Name] = ScheduledEventType(strings.Trim(lit.Value, `"`))
				}
			}
		}
		return true
	})

	for _, name := range declared {
		v, ok := values[name]
		if !ok {
			t.Errorf("%s has no string value — cannot check its priority", name)
			continue
		}
		if _, placed := tickPriorities[v]; !placed {
			t.Errorf("%s (%q) has no entry in tickPriorities — decide where in the game day it "+
				"runs and add it to priority.go, then record the choice in megaron_tickordning.md. "+
				"Falling back to DefaultTickPriority means it races whatever else sits on %d.",
				name, v, DefaultTickPriority)
		}
	}
}

// TestObligationsRunBeforeGrowth pins the one ordering the acceptance world
// caught red-handed on 2026-08-24: KharisTick's grain-funded growth spends the
// settlement's whole grain stock, so if it runs before UpkeepTick the garrison
// starves in a city with a healthy surplus. Both capitals lost 10 men to
// grain_shortage at tick 1 under the old coin flip.
func TestObligationsRunBeforeGrowth(t *testing.T) {
	reserve := TickPriority(ScheduledSitosTick)
	upkeep := TickPriority(ScheduledUpkeepTick)
	food := TickPriority(ScheduledFoodTick)
	growth := TickPriority(ScheduledKharisTick)

	if !(reserve < upkeep && upkeep < growth) {
		t.Fatalf("the day's economic order must be reserve → obligation → growth, got "+
			"Sitos=%d Upkeep=%d Kharis=%d", reserve, upkeep, growth)
	}
	// Utfodringsordningen (megaron_plan_utfodringsordningen.md, 2026-08-26):
	// the population eats AFTER the army's own upkeep (Timothy 2026-08-25 —
	// "ALLT SOM STADEN FÖRSÖRJER ÄTER FÖRE BEFOLKNINGEN") and BEFORE growth,
	// which must only ever see what both obligations left behind.
	if !(upkeep < food && food < growth) {
		t.Fatalf("the day's food order must be plikt → föda → tillväxt, got "+
			"Upkeep=%d Food=%d Kharis=%d", upkeep, food, growth)
	}
	if TickPriority(ScheduledUnitArrival) >= TickPriority(ScheduledMarchSightingScan) {
		t.Errorf("the eyes must read the map after the marches have moved: UnitArrival=%d MarchSightingScan=%d",
			TickPriority(ScheduledUnitArrival), TickPriority(ScheduledMarchSightingScan))
	}
	if TickPriority(ScheduledMarchSightingScan) >= TickPriority(ScheduledBattleTick) {
		t.Errorf("sighting must precede battle resolution: MarchSightingScan=%d BattleTick=%d",
			TickPriority(ScheduledMarchSightingScan), TickPriority(ScheduledBattleTick))
	}
}
