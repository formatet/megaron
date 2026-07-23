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

// resolveProvince looks up a province by province-UUID, settlement-UUID, name, or
// "q,r" coordinate, returning the province ID. Province-id and settlement-id are both
// bare UUIDs — shape alone can't tell them apart, so (unlike resolveSettlement, which
// trusts any UUID-shaped string) this always checks the marker list to resolve a
// settlement-UUID to its owning province.
func resolveProvince(c *Client, worldID, nameOrID string) (string, error) {
	data, err := c.get("/api/v1/worlds/" + worldID + "/provinces")
	if err != nil {
		return "", err
	}
	var markers []map[string]any
	if err := json.Unmarshal(data, &markers); err != nil {
		return "", err
	}

	// 1. province-id (exact match).
	for _, m := range markers {
		if pid, _ := m["id"].(string); pid == nameOrID {
			return pid, nil
		}
	}
	// 2. settlement-id → resolve to its owning province.
	for _, m := range markers {
		if sid, _ := m["settlement_id"].(string); sid != "" && sid == nameOrID {
			pid, _ := m["id"].(string)
			return pid, nil
		}
	}
	// 3. name (case-insensitive).
	needle := strings.ToLower(nameOrID)
	for _, m := range markers {
		if n, _ := m["name"].(string); strings.ToLower(n) == needle {
			pid, _ := m["id"].(string)
			return pid, nil
		}
	}
	// 4. "q,r" coordinate. Reuses parseQR (cmd_unit.go) — it errors on a shape
	// mismatch, which just means this input isn't a coordinate; fall through.
	if q, r, qrErr := parseQR(nameOrID); qrErr == nil {
		for _, m := range markers {
			mq, _ := m["q"].(float64)
			mr, _ := m["r"].(float64)
			if int(mq) == q && int(mr) == r {
				pid, _ := m["id"].(string)
				return pid, nil
			}
		}
		return "", fmt.Errorf("no province at %s in view", nameOrID)
	}

	if len(nameOrID) == 36 && strings.Count(nameOrID, "-") == 4 {
		return "", fmt.Errorf("no province/settlement with id %q you can see — run `keryx settlements`", nameOrID)
	}
	return "", fmt.Errorf("no visible province named %q", nameOrID)
}
