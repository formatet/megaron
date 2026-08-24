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

// resolveUnitID resolves a --unit/--ship value to a full unit UUID. An
// exact UUID-shaped string is trusted outright — the server checks
// existence and ownership itself, same as resolveSettlement above.
// Anything shorter is treated as a prefix and matched case-insensitively
// against the player's own units (GET .../units — the same list `keryx
// unit list` reads): zero matches names what to run for the full id, more
// than one lists every candidate instead of guessing at which one was meant.
//
// Rad H, megaron_plan_cli_sanning.md: every dispatch confirmation in
// cmd_unit.go echoes an 8-char unitID[:8] for brevity ("Unit 8afb6a29
// marching to..."), with no marker that it's only a fragment — pasting that
// straight back into --unit used to fail with an opaque "invalid unit ID"
// (HTTP 400). Applied to every --unit/--ship flag in cmd_unit.go: they are
// the identical defect (one flag pattern, one root), not eight different
// ones needing eight different fixes.
func resolveUnitID(c *Client, worldID, idOrPrefix string) (string, error) {
	if len(idOrPrefix) == 36 && strings.Count(idOrPrefix, "-") == 4 {
		return idOrPrefix, nil
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", worldID))
	if err != nil {
		return "", err
	}
	var resp struct {
		Units []unitRow `json:"units"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	needle := strings.ToLower(idOrPrefix)
	var matches []unitRow
	for _, u := range resp.Units {
		if strings.HasPrefix(strings.ToLower(u.ID), needle) {
			matches = append(matches, u)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no unit of yours starts with %q — run `keryx unit list` for the full id", idOrPrefix)
	case 1:
		return matches[0].ID, nil
	default:
		lines := make([]string, 0, len(matches))
		for _, u := range matches {
			name := u.DisplayName
			if name == "" {
				name = u.Type
			}
			lines = append(lines, fmt.Sprintf("  %s  %s", u.ID, name))
		}
		return "", fmt.Errorf("%q matches %d of your units — be more specific:\n%s", idOrPrefix, len(matches), strings.Join(lines, "\n"))
	}
}
