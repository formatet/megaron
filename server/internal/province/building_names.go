package province

// BuildingDisplayName is the player-facing name shared by dispatches, queues and CLI.
var buildingDisplayNames = map[string]string{"farm": "Farm", "barracks": "Barracks", "mine": "Mine", "lumbermill": "Lumbermill", "stonequarry": "Stone Quarry", "market": "Market", "wall": "Wall", "tower": "Tower", "harbour": "Harbour", "shipyard": "Shipyard", "foundry": "Foundry", "stable": "Stable", "bronze_wall": "Bronze Wall", "olive_press": "Olive Press", "winery": "Winery", "temple": "Temple", "pen": "Pen", "silo": "Granary"}

func BuildingDisplayName(key string) string {
	if name, ok := buildingDisplayNames[key]; ok {
		return name
	}
	return "Building"
}
