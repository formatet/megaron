package main

import (
	"encoding/json"
	"fmt"
)

type recallAllResult struct {
	UnitID  string          `json:"unit_id"`
	Name    string          `json:"name"`
	Status  string          `json:"status"`
	Reason  string          `json:"reason,omitempty"`
	Receipt json.RawMessage `json:"receipt,omitempty"`
}

// Both --unit and --all use the existing single-unit recall endpoint.
func requestRecall(c *Client, worldID, unitID string) ([]byte, error) {
	return c.post(fmt.Sprintf("/api/v1/worlds/%s/units/%s/recall", worldID, unitID), map[string]any{})
}

func recallAllUnits(c *Client, worldID string) error {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", worldID))
	if err != nil {
		return err
	}
	var list struct {
		Units []unitRow `json:"units"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("could not read units: %w", err)
	}
	results := make([]recallAllResult, 0)
	called := 0
	for _, u := range list.Units {
		if u.Status != "marching" {
			continue
		}
		name := u.DisplayName
		if name == "" && u.Name != nil {
			name = *u.Name
		}
		if name == "" {
			name = u.ID
		}
		result := recallAllResult{UnitID: u.ID, Name: name, Status: "recall_sent"}
		receipt, err := requestRecall(c, worldID, u.ID)
		if err != nil {
			result.Status = "rejected"
			result.Reason = err.Error()
		} else {
			called++
			result.Receipt = receipt
		}
		results = append(results, result)
	}
	if jsonMode {
		printJSON(map[string]any{"called_home": called, "results": results})
		return nil
	}
	for _, r := range results {
		if r.Status == "recall_sent" {
			fmt.Printf("%s: recall sent.\n", r.Name)
		} else {
			fmt.Printf("%s: %s\n", r.Name, r.Reason)
		}
	}
	fmt.Println(recallAllSummary(called))
	return nil
}

func recallAllSummary(count int) string {
	if count == 0 {
		return "No units are being called home."
	}
	small := []string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
	amount := fmt.Sprint(count)
	if count < len(small) {
		amount = small[count]
	}
	if count == 1 {
		return amount + " unit is being called home."
	}
	return amount + " units are being called home."
}
