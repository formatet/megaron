package main

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
)

func resolveTransferDestination(c *Client, world, source, name string) (string, error) {
	if _, err := uuid.Parse(name); err == nil {
		return name, nil
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/trade/destinations", world, source))
	if err != nil {
		return "", err
	}
	var rows []struct {
		ID   string `json:"settlement_id"`
		Name string `json:"name"`
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		return "", err
	}
	found := ""
	for _, r := range rows {
		if strings.EqualFold(r.Name, name) {
			if found != "" {
				return "", fmt.Errorf("more than one city named %q; use its settlement ID", name)
			}
			found = r.ID
		}
	}
	if found == "" {
		return "", fmt.Errorf("no own or contacted city named %q", name)
	}
	return found, nil
}

func printGiftOutcome(n notificationItem) {
	var b struct {
		OriginName      string  `json:"origin_name"`
		DestinationName string  `json:"destination_name"`
		Good            string  `json:"good_key"`
		Credited        float64 `json:"credited_quantity"`
		Returned        float64 `json:"returned_quantity"`
		Lost            float64 `json:"lost_quantity"`
		Reason          string  `json:"reason"`
		Changed         bool    `json:"owner_changed"`
		Actual          string  `json:"actual_recipient_name"`
		Intended        string  `json:"recipient_name"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	fmt.Printf("    Gift: %s → %s · %g %s received; %g lost", b.OriginName, b.DestinationName, b.Credited, b.Good, b.Lost)
	if b.Returned > 0 {
		fmt.Printf(" · %g sent home aboard the damaged ship (return can still be intercepted)", b.Returned)
	}
	if b.Reason != "" {
		fmt.Printf(" · %s", strings.ReplaceAll(b.Reason, "_", " "))
	}
	if b.Changed {
		fmt.Printf(" · now held by %s, intended for %s", b.Actual, b.Intended)
	}
	fmt.Println()
}
