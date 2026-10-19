package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func notificationTick(n notificationItem) *int {
	if n.WorldTick != nil {
		return n.WorldTick
	}
	var b struct {
		WorldTick *int `json:"world_tick"`
	}
	if json.Unmarshal(n.Body, &b) == nil {
		return b.WorldTick
	}
	return nil
}
func notificationDay(n notificationItem) string {
	if tick := notificationTick(n); tick != nil && *tick >= 0 {
		return fmt.Sprintf("Day #%d", *tick)
	}
	return "Day unknown"
}

// A recovered city's historical start must not remain urgent forever.
func urgentNotificationIndices(notifs []notificationItem) map[int]bool {
	indices := make([]int, len(notifs))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := notifs[indices[i]], notifs[indices[j]]
		at, bt := notificationTick(a), notificationTick(b)
		if at != nil && bt != nil && *at != *bt {
			return *at > *bt
		}
		aTime, _ := time.Parse(time.RFC3339Nano, a.CreatedAt)
		bTime, _ := time.Parse(time.RFC3339Nano, b.CreatedAt)
		return aTime.After(bTime)
	})
	urgent := map[int]bool{}
	seen := map[string]bool{}
	for _, i := range indices {
		n := notifs[i]
		if subsistenceTier(n) == "critical" {
			urgent[i] = true
		}
		family := ""
		switch n.Kind {
		case "GarrisonStarvationStarted", "GarrisonStarvationEnded":
			family = "garrison:"
		case "CivilianStarvationStarted", "CivilianStarvationEnded":
			family = "civilian:"
		default:
			continue
		}
		var b struct {
			SettlementID string `json:"settlement_id"`
			ProvinceID   string `json:"province_id"`
		}
		if json.Unmarshal(n.Body, &b) != nil {
			continue
		}
		key := b.SettlementID
		if key == "" {
			key = b.ProvinceID
		}
		if key == "" {
			continue
		}
		key = family + key
		if seen[key] {
			continue
		}
		seen[key] = true
		if n.Kind == "GarrisonStarvationStarted" || n.Kind == "CivilianStarvationStarted" {
			urgent[i] = true
		}
	}
	return urgent
}
