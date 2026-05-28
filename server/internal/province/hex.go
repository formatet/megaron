package province

// HexDistance returns the distance between two axial hex coordinates.
func HexDistance(a, b MapPosition) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	return (abs(dq) + abs(dq+dr) + abs(dr)) / 2
}

// VisibleFrom returns true if target is within radius hexes of any province in origins.
func VisibleFrom(target MapPosition, origins []MapPosition, radius int) bool {
	for _, o := range origins {
		if HexDistance(o, target) <= radius {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
