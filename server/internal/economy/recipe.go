package economy

// Recipe describes a craft transformation: ingredients → output.
type Recipe struct {
	ID           int
	OutputKey    string
	OutputQty    float64
	BuildingType string  // required building
	DurationMin  float64 // reserved for future craft queue
}

// RecipeIngredient is one consumed input for a recipe.
type RecipeIngredient struct {
	RecipeID int
	GoodKey  string
	Quantity float64
}
