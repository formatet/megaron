package main

import (
	"encoding/json"
	"fmt"
)

type cultPrayer struct {
	riteAvailablePrayer
	SettlementID string `json:"settlement_id"`
	ProvinceID   string `json:"province_id"`
	CityName     string `json:"city_name"`
	HasTemple    bool   `json:"has_temple"`
}
type cultTemple struct {
	ID           string         `json:"id"`
	SettlementID string         `json:"settlement_id"`
	ProvinceID   string         `json:"province_id"`
	CityName     string         `json:"city_name"`
	GodID        string         `json:"god_id"`
	Name         string         `json:"name"`
	Level        int            `json:"level"`
	Staffed      int            `json:"staffed"`
	Capacity     int            `json:"capacity"`
	Offer        map[string]any `json:"offer"`
	Demolition   map[string]any `json:"demolition_kharis"`
}
type cultUnbuilt struct {
	SettlementID string `json:"settlement_id"`
	ProvinceID   string `json:"province_id"`
	CityName     string `json:"city_name"`
	GodID        string `json:"god_id"`
	Name         string `json:"name"`
}
type cultView struct {
	Kharis  map[string]any `json:"kharis"`
	Temples []cultTemple   `json:"temples"`
	Prayers []cultPrayer   `json:"prayers"`
	Unbuilt []cultUnbuilt  `json:"unbuilt_temples"`
}

func loadRealmCult(c *Client) (cultView, error) {
	var view cultView
	raw, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/cult", cfg.WorldID))
	if err != nil {
		return view, err
	}
	err = json.Unmarshal(raw, &view)
	if err == nil && view.Kharis == nil {
		err = fmt.Errorf("realm cult projection unavailable")
	}
	return view, err
}
func listRealmPrayers(c *Client, settlement string) error {
	view, err := loadRealmCult(c)
	if err != nil {
		return err
	}
	if settlement != "" {
		rows := []cultPrayer{}
		for _, p := range view.Prayers {
			if p.SettlementID == settlement {
				rows = append(rows, p)
			}
		}
		view.Prayers = rows
	}
	if jsonMode {
		raw, err := json.Marshal(view)
		if err != nil {
			return err
		}
		printRawJSON(raw)
		return nil
	}
	if standing, ok := view.Kharis["kharis_standing"].(map[string]any); ok {
		fmt.Println(standing["summary"])
	}
	printDivineValues(cultDivineValues(view.Kharis))
	fmt.Println("Prayers across your realm — ready prayers first")
	for _, p := range view.Prayers {
		ready, wait := prayerReadiness(p.riteAvailablePrayer)
		fmt.Printf("%s — %s · %s\n  %s · %s · %s\n", p.Name, p.God, p.CityName, p.ID, ready, wait)
		fmt.Printf("  Requires %.0f kharis · traditional offering %s\n  %s\n", p.MinKharis, formatOffering(p.Offering), p.Effect)
		if taste := favouredGoods(p.Favours); taste != "" {
			fmt.Printf("  %s favours %s\n", p.God, taste)
		}
		fmt.Printf("  Cast here: keryx rite --settlement %s --prayer %s\n", p.SettlementID, p.ID)
	}
	if len(view.Prayers) == 0 {
		fmt.Println("No prayers available here. Found a city and build a temple.")
	}
	fmt.Println("Kharis is a requirement, never spent. Spontaneous blessings are separate from prayers.")
	return nil
}
func cultDivineValues(m map[string]any) map[string]float64 {
	out := map[string]float64{}
	if values, ok := m["divine_values"].(map[string]any); ok {
		for key, value := range values {
			if n, ok := value.(float64); ok {
				out[key] = n
			}
		}
	}
	return out
}
func printRealmTemples(c *Client) bool {
	view, err := loadRealmCult(c)
	if err != nil {
		fmt.Println("Realm cult overview unavailable.")
		return false
	}
	fmt.Println("Temples across your realm")
	for _, t := range view.Temples {
		fmt.Printf("  %s — %s · level %d · workers %d/%d\n", t.Name, t.CityName, t.Level, t.Staffed, t.Capacity)
		fmt.Println(templeOfferingLine(t.Offer))
		fmt.Printf("  Manage: keryx staff temple +1 --province %s --god %s\n", t.ProvinceID, t.GodID)
		fmt.Printf("  Demolish: keryx demolish --type temple --god %s --province %s\n", t.GodID, t.ProvinceID)
	}
	if len(view.Temples) == 0 {
		fmt.Println("  No standing temples.")
	}
	if len(view.Unbuilt) > 0 {
		fmt.Println("Not built")
	}
	for _, t := range view.Unbuilt {
		fmt.Printf("  %s — %s · open this city to build\n", t.Name, t.CityName)
	}

	return true
}
