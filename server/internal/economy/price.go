package economy

const (
	priceFloor     = 0.5
	priceCeiling   = 3.0
	referenceRatio = 0.3 // comfortable stock = cap × referenceRatio
	epsilon        = 0.001
)

// LocalPrice returns the local market price for one unit of a good.
// price = baseValue × clamp(reference / max(stock, ε), 0.5, 3.0)
// Shortage triples price; surplus halves it.
func LocalPrice(baseValue, stock, cap float64) float64 {
	reference := cap * referenceRatio
	s := stock
	if s < epsilon {
		s = epsilon
	}
	ratio := reference / s
	if ratio < priceFloor {
		ratio = priceFloor
	}
	if ratio > priceCeiling {
		ratio = priceCeiling
	}
	return baseValue * ratio
}
