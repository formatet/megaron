package combat

import "formatet/megaron/server/internal/unit"

func rationCrewForTest(kind string) int { return unit.CrewFor(unit.Type(kind)) }
