package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// resolveSettlement looks up a settlement by name or UUID, returning its ID.
func resolveSettlement(c *Client, worldID, nameOrID string) (string, error) {
	if len(nameOrID) == 36 && strings.Count(nameOrID, "-") == 4 {
		return nameOrID, nil
	}
	data, err := c.get("/api/v1/worlds/" + worldID + "/provinces")
	if err != nil {
		return "", err
	}
	var markers []map[string]any
	if err := json.Unmarshal(data, &markers); err != nil {
		return "", err
	}
	needle := strings.ToLower(nameOrID)
	for _, m := range markers {
		n, _ := m["name"].(string)
		if strings.ToLower(n) == needle {
			sid, _ := m["settlement_id"].(string)
			if sid == "" {
				return "", fmt.Errorf("province %q has no settlement", n)
			}
			return sid, nil
		}
	}
	return "", fmt.Errorf("no visible settlement named %q", nameOrID)
}
