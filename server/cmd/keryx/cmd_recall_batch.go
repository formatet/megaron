package main

import (
	"encoding/json"
	"fmt"
)

type recallBatchResponse struct {
	Results []struct {
		UnitID string `json:"unit_id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"results"`
	Groups []struct {
		UnitIDs       []string `json:"unit_ids"`
		Q             int      `json:"q"`
		R             int      `json:"r"`
		DueTick       int      `json:"courier_due_tick"`
		PassageStatus string   `json:"passage_status"`
	} `json:"groups"`
}

func printRecallBatch(c *Client, data []byte) error {
	var report recallBatchResponse
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	if len(report.Results) == 0 {
		fmt.Println("No units match the recall selection.")
		return nil
	}
	for _, g := range report.Groups {
		label := "Group"
		if len(g.UnitIDs) == 1 {
			label = "Single"
		}
		fmt.Printf("%s: Runner for %d unit(s) at (%d,%d)", label, len(g.UnitIDs), g.Q, g.R)
		if g.PassageStatus != "" {
			fmt.Printf(" — %s", g.PassageStatus)
		} else {
			fmt.Printf(" — delivery on game day %d", g.DueTick)
		}
		fmt.Println("; they turn home on delivery.")
	}
	for _, r := range report.Results {
		name := r.Name
		if name == "" {
			name = r.UnitID
		}
		switch r.Status {
		case "order_dispatched":
			fmt.Printf("%s: recall sent by Runner.\n", name)
		case "order_applied":
			fmt.Printf("%s: recall given in person.\n", name)
		default:
			fmt.Printf("%s: rejected — %s\n", name, r.Reason)
		}
	}
	return nil
}
